-- Ответы блока А (docs/tasks.md). Витрины строятся только по core; деньги — только из
-- core.revenue_items / core.revenue_orders, где выручка определена один раз (model/build.sql).

SET client_min_messages = warning;

CREATE SCHEMA IF NOT EXISTS mart;

-- А1. Выручка и число заказов по месяцам, 2016-09 … 2018-08.
-- Уровень: заказ, месяц — по дате оформления (purchased_at).
-- Решения: месяцы берутся из generate_series, поэтому месяц без заказов (2016-11) остаётся
-- строкой с нулями, а не пропадает. Заказы — те же, что дают выручку (delivered).
CREATE OR REPLACE VIEW mart.a1_monthly_revenue AS
WITH months AS (
    SELECT generate_series(date '2016-09-01', date '2018-08-01', interval '1 month')::date AS month
),
monthly AS (
    SELECT
        date_trunc('month', purchased_at)::date AS month,
        count(*)                                AS orders,
        sum(revenue)                            AS revenue
    FROM core.revenue_orders
    GROUP BY 1
)
SELECT
    m.month,
    coalesce(r.orders, 0)  AS orders,
    coalesce(r.revenue, 0) AS revenue
FROM months AS m
LEFT JOIN monthly AS r ON r.month = m.month
ORDER BY m.month;

-- А2. Топ-10 продавцов по выручке, с городом и штатом.
-- Уровень: позиция заказа — выручка продавца только по его товарам, не по всему заказу.
-- Решения: orders — число разных заказов с его товарами, units — единиц товара (строк позиций).
-- При равной выручке порядок по seller_id, чтобы топ был детерминированным.
CREATE OR REPLACE VIEW mart.a2_top_sellers AS
SELECT
    s.seller_id,
    s.city,
    s.state,
    count(DISTINCT r.order_id) AS orders,
    count(*)                   AS units,
    sum(r.revenue)             AS revenue
FROM core.revenue_items AS r
JOIN core.sellers AS s ON s.seller_id = r.seller_id
GROUP BY s.seller_id, s.city, s.state
ORDER BY revenue DESC, s.seller_id
LIMIT 10;

-- А3. Средний чек заказа.
-- Уровень: заказ. Чек = цена + доставка всех позиций заказа (core.revenue_orders.order_check).
-- Решения: avg_lines — разные товары продавцов в заказе, avg_units — единицы товара; оба
-- больше 1, значит счёт идёт по заказам, а не по позициям.
CREATE OR REPLACE VIEW mart.a3_avg_check AS
SELECT
    round(avg(order_check), 2) AS avg_check,
    round(avg(lines), 4)       AS avg_lines,
    round(avg(units), 4)       AS avg_units
FROM core.revenue_orders;

-- А4. Покупатели и повторные покупки.
-- Уровень: покупатель-человек (customer_unique_id; customer_id источника новый на каждый
-- заказ и дал бы 99 441).
-- Решения: покупатель — кто оформил хотя бы один заказ в любом статусе; вернулся — оформил
-- два и более разных заказа.
CREATE OR REPLACE VIEW mart.a4_customers AS
WITH per_customer AS (
    SELECT
        customer_unique_id,
        count(DISTINCT order_id) AS orders
    FROM core.orders
    WHERE customer_unique_id IS NOT NULL
    GROUP BY customer_unique_id
)
SELECT
    count(*)                                                       AS customers,
    count(*) FILTER (WHERE orders > 1)                             AS returned,
    round(100.0 * count(*) FILTER (WHERE orders > 1) / count(*), 2) AS returned_pct
FROM per_customer;

-- А5. Выручка по категориям с долей в общей выручке.
-- Уровень: позиция заказа (категория — свойство товара позиции).
-- Решения: товары без категории — отдельной строкой 'unknown', а не теряются, поэтому сумма
-- по категориям равна общей выручке. LEFT JOIN к товарам — позиция без карточки тоже не теряется.
CREATE OR REPLACE VIEW mart.a5_category_revenue AS
WITH by_category AS (
    SELECT
        coalesce(p.category_name, 'unknown') AS category,
        sum(r.revenue)                       AS revenue
    FROM core.revenue_items AS r
    LEFT JOIN core.products AS p ON p.product_id = r.product_id
    GROUP BY 1
)
SELECT
    category,
    revenue,
    round(100 * revenue / sum(revenue) OVER (), 2) AS share_pct
FROM by_category
ORDER BY revenue DESC, category;
