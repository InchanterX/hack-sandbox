-- Сборка модели core из raw. Запускается на чистой базе и повторно: схема core пересоздаётся
-- целиком, raw только читается. Витрины mart зависят от core и пересоздаются после
-- (mart/answers_*.sql).
--
-- Откуда взяты ключевые идеи. Решений участников в brief/assets/solutions/ нет (там только
-- README), поэтому вердикт Fusion T01 не смог назвать автора ни одной идеи и дал чек-лист
-- member-2 «только как чек-лист, сверить с handout и sql.md». Ниже у каждой идеи указан
-- источник: [вердикт/member-2] — пункт чек-листа из вердикта, [sql.md] — правило
-- brief/04_stack/sql.md, [tasks.md]/[handout] — подсказка организаторов, [своё] — решение
-- исполнителя, подтверждённое контрольными цифрами.
--
-- Слои raw -> core -> mart, mart читает только core              [вердикт/member-2, sql.md]
-- Зерно: заказ — одна строка, позиция — одна единица товара,
--        деньги считаем на позиции и только агрегируем вверх      [вердикт/member-2]
-- Статусы и выручка: справочник статусов с флагом
--        counts_as_revenue вместо CASE в каждой витрине           [вердикт/member-2]
--        выручка = цена позиций доставленных заказов,
--        чек = цена + доставка — проверено контрольными цифрами   [своё, tasks.md]
-- Покупатель — customer_unique_id, а не customer_id               [handout, tasks.md, sql.md]
-- Пустая категория остаётся NULL в core, в витринах — отдельной
--        строкой «unknown»                                        [вердикт/member-2, tasks.md]
-- Дубли: точные дубли схлопываются DISTINCT, конфликт по ключу
--        роняет сборку на PRIMARY KEY (громко, а не молча)        [вердикт/member-2 + своё]
-- Типизация: timestamp, numeric(12,2), integer; пустое и мусор —
--        явно в NULL через core.try_* (без отдельной таблицы
--        rejects: sql.md требует NULL, а мусора в данных нет)     [sql.md вместо вердикта]

SET client_min_messages = warning;

DROP SCHEMA IF EXISTS core CASCADE;
CREATE SCHEMA core;

-- Безопасное приведение типов: пустая строка и мусор -> NULL, сборка не падает.

CREATE FUNCTION core.try_timestamp(x text) RETURNS timestamp
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE
        WHEN trim(x) ~ '^\d{4}-\d{2}-\d{2}( \d{2}:\d{2}(:\d{2})?)?$' THEN trim(x)::timestamp
    END
$$;

CREATE FUNCTION core.try_numeric(x text) RETURNS numeric
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN trim(x) ~ '^-?\d+(\.\d+)?$' THEN trim(x)::numeric END
$$;

CREATE FUNCTION core.try_integer(x text) RETURNS integer
LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN trim(x) ~ '^-?\d{1,9}(\.0+)?$' THEN trim(x)::numeric::integer END
$$;

-- ---------------------------------------------------------------------------------------------
-- Статусы заказа. Здесь и только здесь решается, какой заказ считается выручкой.

CREATE TABLE core.order_statuses (
    order_status      text PRIMARY KEY,
    counts_as_revenue boolean NOT NULL,
    description       text NOT NULL
);

COMMENT ON TABLE core.order_statuses IS
    'одна строка — статус заказа. Единственное место, где решается, какие заказы — выручка';
COMMENT ON COLUMN core.order_statuses.counts_as_revenue IS
    'почему так: выручка — только delivered, товар дошёл до покупателя. shipped/invoiced/'
    'processing/approved/created ещё могут отмениться, canceled/unavailable не состоялись. '
    'Сверено с контролем блока А: 13 221 498.11';

INSERT INTO core.order_statuses (order_status, counts_as_revenue, description) VALUES
    ('delivered',   true,  'вручён покупателю'),
    ('shipped',     false, 'передан в доставку, ещё не вручён'),
    ('invoiced',    false, 'выставлен счёт'),
    ('processing',  false, 'собирается'),
    ('approved',    false, 'оплата подтверждена'),
    ('created',     false, 'оформлен'),
    ('canceled',    false, 'отменён'),
    ('unavailable', false, 'товар недоступен');

-- Новый статус в raw не теряется: попадает сюда с counts_as_revenue = false и виден в отчётах.
INSERT INTO core.order_statuses (order_status, counts_as_revenue, description)
SELECT DISTINCT trim(order_status), false, 'новый статус из источника: решить, выручка ли это'
FROM raw.orders
WHERE nullif(trim(order_status), '') IS NOT NULL
ON CONFLICT (order_status) DO NOTHING;

-- ---------------------------------------------------------------------------------------------
-- Категории товаров. В справочнике перевода 71 строка, в товарах встречается 73 категории:
-- две без перевода тоже попадают сюда (english = NULL), чтобы FK из products не терял товары.

CREATE TABLE core.categories (
    category_name         text PRIMARY KEY,
    category_name_english text
);

COMMENT ON TABLE core.categories IS
    'одна строка — категория товара (португальское название из источника). '
    'Объединение справочника перевода и категорий из товаров; без перевода — english NULL';

INSERT INTO core.categories (category_name, category_name_english)
SELECT DISTINCT ON (category_name) category_name, category_name_english
FROM (
    SELECT nullif(trim(product_category_name), '') AS category_name,
           nullif(trim(product_category_name_english), '') AS category_name_english
    FROM raw.product_category_translation
    UNION ALL
    SELECT nullif(trim(product_category_name), ''), NULL
    FROM raw.products
) AS c
WHERE category_name IS NOT NULL
ORDER BY category_name, category_name_english NULLS LAST;

-- ---------------------------------------------------------------------------------------------
-- Продавцы.

CREATE TABLE core.sellers (
    seller_id       text PRIMARY KEY,
    zip_code_prefix text,
    city            text,
    state           text
);

COMMENT ON TABLE core.sellers IS 'одна строка — продавец (seller_id из источника)';

INSERT INTO core.sellers (seller_id, zip_code_prefix, city, state)
SELECT DISTINCT
    trim(seller_id),
    nullif(trim(seller_zip_code_prefix), ''),
    nullif(trim(seller_city), ''),
    nullif(trim(seller_state), '')
FROM raw.sellers;

-- ---------------------------------------------------------------------------------------------
-- Товары. Опечатку источника (lenght) исправляем в именах колонок core.

CREATE TABLE core.products (
    product_id                 text PRIMARY KEY,
    category_name              text REFERENCES core.categories,
    product_name_length        integer,
    product_description_length integer,
    product_photos_qty         integer,
    product_weight_g           integer,
    product_length_cm          integer,
    product_height_cm          integer,
    product_width_cm           integer
);

COMMENT ON TABLE core.products IS 'одна строка — карточка товара (product_id из источника)';
COMMENT ON COLUMN core.products.category_name IS
    'NULL — категория не заполнена в источнике (610 товаров); в витринах это строка unknown';

INSERT INTO core.products
SELECT DISTINCT
    trim(product_id),
    nullif(trim(product_category_name), ''),
    core.try_integer(product_name_lenght),
    core.try_integer(product_description_lenght),
    core.try_integer(product_photos_qty),
    core.try_integer(product_weight_g),
    core.try_integer(product_length_cm),
    core.try_integer(product_height_cm),
    core.try_integer(product_width_cm)
FROM raw.products;

CREATE INDEX ON core.products (category_name);

-- ---------------------------------------------------------------------------------------------
-- Покупатели. В raw.customers customer_id новый на каждый заказ (99 441 = числу заказов),
-- человек — customer_unique_id (96 096). Поэтому покупатель в core — человек, а адрес
-- доставки и customer_id источника — свойства заказа (core.orders).

CREATE TABLE core.customers (
    customer_unique_id text PRIMARY KEY,
    zip_code_prefix    text,
    city               text,
    state              text
);

COMMENT ON TABLE core.customers IS
    'одна строка — покупатель-человек (customer_unique_id). Адрес — из его последнего заказа; '
    'адрес на момент каждого заказа лежит в core.orders';

INSERT INTO core.customers (customer_unique_id, zip_code_prefix, city, state)
SELECT DISTINCT ON (trim(c.customer_unique_id))
    trim(c.customer_unique_id),
    nullif(trim(c.customer_zip_code_prefix), ''),
    nullif(trim(c.customer_city), ''),
    nullif(trim(c.customer_state), '')
FROM raw.customers AS c
LEFT JOIN raw.orders AS o ON trim(o.customer_id) = trim(c.customer_id)
ORDER BY trim(c.customer_unique_id), core.try_timestamp(o.order_purchase_timestamp) DESC NULLS LAST;

-- ---------------------------------------------------------------------------------------------
-- Заказы. raw.customers соединяется LEFT JOIN: заказ без покупателя не теряется
-- (customer_unique_id = NULL; сейчас таких 0).

CREATE TABLE core.orders (
    order_id                 text PRIMARY KEY,
    customer_unique_id       text REFERENCES core.customers,
    source_customer_id       text NOT NULL UNIQUE,
    order_status             text NOT NULL REFERENCES core.order_statuses,
    purchased_at             timestamp NOT NULL,
    approved_at              timestamp,
    delivered_carrier_at     timestamp,
    delivered_customer_at    timestamp,
    estimated_delivery_date  timestamp,
    delivery_zip_code_prefix text,
    delivery_city            text,
    delivery_state           text
);

COMMENT ON TABLE core.orders IS
    'одна строка — заказ. Покупатель — человек (customer_unique_id), source_customer_id — '
    'одноразовый id покупателя из источника, адрес — куда доставлялся этот заказ';
COMMENT ON COLUMN core.orders.source_customer_id IS
    'customer_id источника: новый на каждый заказ, для подсчёта покупателей не годится';

INSERT INTO core.orders
SELECT DISTINCT
    trim(o.order_id),
    trim(c.customer_unique_id),
    trim(o.customer_id),
    trim(o.order_status),
    core.try_timestamp(o.order_purchase_timestamp),
    core.try_timestamp(o.order_approved_at),
    core.try_timestamp(o.order_delivered_carrier_date),
    core.try_timestamp(o.order_delivered_customer_date),
    core.try_timestamp(o.order_estimated_delivery_date),
    nullif(trim(c.customer_zip_code_prefix), ''),
    nullif(trim(c.customer_city), ''),
    nullif(trim(c.customer_state), '')
FROM raw.orders AS o
LEFT JOIN raw.customers AS c ON trim(c.customer_id) = trim(o.customer_id);

CREATE INDEX ON core.orders (customer_unique_id);
CREATE INDEX ON core.orders (order_status);
CREATE INDEX ON core.orders (purchased_at);

-- ---------------------------------------------------------------------------------------------
-- Позиции заказа. order_item_id — номер внутри заказа: одна строка источника — одна единица
-- товара (два одинаковых товара = две строки с одной ценой). Поэтому единиц = строк, а
-- «строк заказа» (разных товаров продавца) — count(DISTINCT (product_id, seller_id)).

CREATE TABLE core.order_items (
    order_id            text NOT NULL REFERENCES core.orders,
    order_item_id       integer NOT NULL,
    product_id          text NOT NULL REFERENCES core.products,
    seller_id           text NOT NULL REFERENCES core.sellers,
    shipping_limit_date timestamp,
    price               numeric(12,2) NOT NULL,
    freight_value       numeric(12,2) NOT NULL,
    PRIMARY KEY (order_id, order_item_id)
);

COMMENT ON TABLE core.order_items IS
    'одна строка — единица товара в заказе (order_id + порядковый order_item_id); '
    'цена товара и доставка этой единицы';

INSERT INTO core.order_items
SELECT DISTINCT
    trim(order_id),
    core.try_integer(order_item_id),
    trim(product_id),
    trim(seller_id),
    core.try_timestamp(shipping_limit_date),
    core.try_numeric(price),
    core.try_numeric(freight_value)
FROM raw.order_items;

CREATE INDEX ON core.order_items (product_id);
CREATE INDEX ON core.order_items (seller_id);

-- ---------------------------------------------------------------------------------------------
-- Платежи.

CREATE TABLE core.order_payments (
    order_id             text NOT NULL REFERENCES core.orders,
    payment_sequential   integer NOT NULL,
    payment_type         text NOT NULL,
    payment_installments integer,
    payment_value        numeric(12,2) NOT NULL,
    PRIMARY KEY (order_id, payment_sequential)
);

COMMENT ON TABLE core.order_payments IS
    'одна строка — платёж заказа (заказ бывает оплачен несколькими способами). '
    'Для выручки не используется: она считается по позициям, см. core.order_revenue';

INSERT INTO core.order_payments
SELECT DISTINCT
    trim(order_id),
    core.try_integer(payment_sequential),
    trim(payment_type),
    core.try_integer(payment_installments),
    core.try_numeric(payment_value)
FROM raw.order_payments;

-- ---------------------------------------------------------------------------------------------
-- Отзывы. review_id не уникален: один и тот же отзыв бывает привязан к нескольким заказам
-- (99 224 строки, 98 410 review_id), поэтому ключ — пара (review_id, order_id).

CREATE TABLE core.order_reviews (
    review_id       text NOT NULL,
    order_id        text NOT NULL REFERENCES core.orders,
    review_score    integer CHECK (review_score BETWEEN 1 AND 5),
    comment_title   text,
    comment_message text,
    created_at      timestamp,
    answered_at     timestamp,
    PRIMARY KEY (review_id, order_id)
);

COMMENT ON TABLE core.order_reviews IS
    'одна строка — отзыв о заказе (review_id + order_id: один отзыв бывает у нескольких заказов, '
    'у заказа бывает несколько отзывов)';

INSERT INTO core.order_reviews
SELECT DISTINCT
    trim(review_id),
    trim(order_id),
    core.try_integer(review_score),
    nullif(trim(review_comment_title), ''),
    nullif(trim(review_comment_message), ''),
    core.try_timestamp(review_creation_date),
    core.try_timestamp(review_answer_timestamp)
FROM raw.order_reviews;

CREATE INDEX ON core.order_reviews (order_id);

-- ---------------------------------------------------------------------------------------------
-- Определение выручки — одно на всю модель. Витрины берут деньги только отсюда.
--
--   выручка заказа = сумма цен его позиций (без доставки), если статус counts_as_revenue;
--   чек заказа     = цена + доставка позиций, то есть сколько заплатил покупатель за заказ.
--
-- Почему по позициям, а не по оплатам: позиции знают продавца и товар, поэтому выручка
-- продавца и категории складывается в ту же сумму, что и общая. Почему без доставки:
-- доставка — деньги перевозчика, а не продажи товара; сумма цен доставленных заказов
-- даёт контрольные 13 221 498.11, а цена + доставка на заказ — контрольный чек 159.88.

CREATE VIEW core.revenue_items AS
SELECT
    i.order_id,
    i.order_item_id,
    i.product_id,
    i.seller_id,
    o.customer_unique_id,
    o.purchased_at,
    i.price                   AS revenue,
    i.freight_value           AS freight,
    i.price + i.freight_value AS amount
FROM core.order_items AS i
JOIN core.orders AS o ON o.order_id = i.order_id
JOIN core.order_statuses AS s ON s.order_status = o.order_status
WHERE s.counts_as_revenue;

COMMENT ON VIEW core.revenue_items IS
    'одна строка — единица товара в заказе, который считается выручкой. '
    'revenue = цена товара, amount = цена + доставка. Единственное определение выручки';

CREATE VIEW core.revenue_orders AS
SELECT
    order_id,
    customer_unique_id,
    purchased_at,
    count(DISTINCT (product_id, seller_id)) AS lines,
    count(*)                                AS units,
    sum(revenue)                            AS revenue,
    sum(freight)                            AS freight,
    sum(amount)                             AS order_check
FROM core.revenue_items
GROUP BY order_id, customer_unique_id, purchased_at;

COMMENT ON VIEW core.revenue_orders IS
    'одна строка — заказ, который считается выручкой и в котором есть позиции. '
    'order_check = цена + доставка всех позиций (чек заказа)';
