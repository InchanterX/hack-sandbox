-- Сверка переноса raw -> core. Исключения из поштучного переноса считаются явно:
-- повторные ключи сворачиваются, покупатели объединяются по customer_unique_id,
-- категории и статусы дополняются из других источников или справочника.
DO $$
DECLARE
    check_row record;
    raw_count bigint;
    expected_count bigint;
    actual_count bigint;
    exception_count bigint;
    raw_amount numeric;
    core_amount numeric;
BEGIN
    FOR check_row IN
        SELECT * FROM (VALUES
            ('sellers', 'sellers', 'trim(seller_id)'),
            ('products', 'products', 'trim(product_id)'),
            ('orders', 'orders', 'trim(order_id)'),
            ('order_items', 'order_items', 'ROW(trim(order_id), core.try_integer(order_item_id))'),
            ('order_payments', 'order_payments', 'ROW(trim(order_id), core.try_integer(payment_sequential))'),
            ('order_reviews', 'order_reviews', 'ROW(trim(review_id), trim(order_id))'),
            ('customers', 'customers', 'trim(customer_unique_id)')
        ) AS checks(raw_table, core_table, grain)
    LOOP
        EXECUTE format('SELECT count(*), count(DISTINCT %s) FROM raw.%I',
                       check_row.grain, check_row.raw_table)
            INTO raw_count, expected_count;
        EXECUTE format('SELECT count(*) FROM core.%I', check_row.core_table) INTO actual_count;
        exception_count := raw_count - expected_count;
        RAISE NOTICE 'raw.%: %, свернуто по зерну: %, ожидается core.%: %',
            check_row.raw_table, raw_count, exception_count, check_row.core_table, expected_count;
        IF actual_count <> expected_count THEN
            RAISE EXCEPTION 'core.%: ожидалось % строк из raw.% (исключено по зерну %), получено %',
                check_row.core_table, expected_count, check_row.raw_table, exception_count, actual_count;
        END IF;
    END LOOP;

    -- Категории: объединение непустых названий перевода и товаров.
    SELECT count(*) INTO raw_count FROM raw.product_category_translation;
    SELECT count(*) INTO expected_count FROM (
        SELECT nullif(trim(product_category_name), '') AS name FROM raw.product_category_translation
        UNION
        SELECT nullif(trim(product_category_name), '') FROM raw.products
    ) AS names WHERE name IS NOT NULL;
    SELECT count(*) INTO actual_count FROM core.categories;
    RAISE NOTICE 'raw.product_category_translation: %, объединение с категориями товаров: %', raw_count, expected_count;
    IF actual_count <> expected_count THEN
        RAISE EXCEPTION 'core.categories: ожидалось % категорий из перевода и товаров, получено %', expected_count, actual_count;
    END IF;
    -- Статусы: восемь фиксированных значений и непустые статусы заказов.
    SELECT count(*) INTO expected_count FROM (
        SELECT unnest(ARRAY['delivered', 'shipped', 'invoiced', 'processing',
                            'approved', 'created', 'canceled', 'unavailable']) AS status
        UNION
        SELECT nullif(trim(order_status), '') FROM raw.orders
    ) AS statuses WHERE status IS NOT NULL;
    SELECT count(*) INTO actual_count FROM core.order_statuses;
    IF actual_count <> expected_count THEN
        RAISE EXCEPTION 'core.order_statuses: ожидалось % статусов справочника и заказов, получено %', expected_count, actual_count;
    END IF;

    FOR check_row IN
        SELECT * FROM (VALUES
            ('order_items', 'price'),
            ('order_items', 'freight_value'),
            ('order_payments', 'payment_value')
        ) AS checks(table_name, column_name)
    LOOP
        EXECUTE format('SELECT sum(core.try_numeric(%I)) FROM raw.%I', check_row.column_name, check_row.table_name)
            INTO raw_amount;
        EXECUTE format('SELECT sum(%I) FROM core.%I', check_row.column_name, check_row.table_name)
            INTO core_amount;
        IF raw_amount IS DISTINCT FROM core_amount THEN
            RAISE EXCEPTION 'Денежная сумма %.%: raw = %, core = %',
                check_row.table_name, check_row.column_name, raw_amount, core_amount;
        END IF;
    END LOOP;

    -- LEFT JOIN при переносе заказов допускает NULL покупателя: считаем сирот явно.
    SELECT count(*) INTO actual_count
    FROM raw.orders AS o
    LEFT JOIN raw.customers AS c ON trim(c.customer_id) = trim(o.customer_id)
    WHERE c.customer_id IS NULL;
    IF actual_count <> 0 THEN
        RAISE EXCEPTION 'raw.orders: % заказов без raw.customers по customer_id', actual_count;
    END IF;
    SELECT count(*) INTO actual_count FROM core.orders WHERE customer_unique_id IS NULL;
    IF actual_count <> 0 THEN
        RAISE EXCEPTION 'core.orders: % заказов без покупателя', actual_count;
    END IF;

    -- Проверяем зерно независимо от ограничений и представления, собираемые JOIN-ами.
    FOR check_row IN
        SELECT * FROM (VALUES
            ('sellers', 'seller_id'), ('products', 'product_id'),
            ('customers', 'customer_unique_id'), ('orders', 'order_id'),
            ('order_items', 'ROW(order_id, order_item_id)'),
            ('order_payments', 'ROW(order_id, payment_sequential)'),
            ('order_reviews', 'ROW(review_id, order_id)'),
            ('categories', 'category_name'), ('order_statuses', 'order_status'),
            ('revenue_items', 'ROW(order_id, order_item_id)'), ('revenue_orders', 'order_id')
        ) AS checks(table_name, grain)
    LOOP
        EXECUTE format('SELECT count(*) - count(DISTINCT %s) FROM core.%I', check_row.grain, check_row.table_name)
            INTO actual_count;
        IF actual_count <> 0 THEN
            RAISE EXCEPTION 'core.%: % строк нарушают уникальность зерна %', check_row.table_name, actual_count, check_row.grain;
        END IF;
    END LOOP;
END
$$;
