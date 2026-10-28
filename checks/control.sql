-- Контрольные цифры блока А из docs/tasks.md. Проверяет витрины mart.a1..a5 (их формат —
-- в brief/04_stack/sql.md) и падает со списком всех расхождений сразу.
DO $$
DECLARE
  errors text[] := '{}';
  n bigint;
  x numeric;
  total constant numeric := 13221498.11;

BEGIN
  -- 1. выручка и заказы по месяцам
  BEGIN
    SELECT count(*), round(sum(revenue)::numeric, 2) INTO n, x FROM mart.a1_monthly_revenue;
    IF n <> 24 THEN errors := errors || format('a1: %s строк, ожидалось 24', n); END IF;
    IF x IS DISTINCT FROM total THEN errors := errors || format('a1: итого %s, ожидалось %s', x, total); END IF;
    SELECT count(*) INTO n FROM mart.a1_monthly_revenue WHERE coalesce(revenue, 0) = 0;
    IF n <> 1 THEN errors := errors || format('a1: месяцев с нулевой выручкой %s, ожидался 1', n); END IF;
  EXCEPTION WHEN OTHERS THEN errors := errors || ('a1: ' || SQLERRM);
  END;

  -- 2. топ-10 продавцов
  BEGIN
    SELECT count(*), round(max(revenue)::numeric, 2) INTO n, x FROM mart.a2_top_sellers;
    IF n <> 10 THEN errors := errors || format('a2: %s строк, ожидалось 10', n); END IF;
    IF x IS DISTINCT FROM 226987.93 THEN errors := errors || format('a2: у первого продавца %s, ожидалось 226987.93', x); END IF;
    PERFORM seller_id, city, state, orders, units FROM mart.a2_top_sellers LIMIT 1;
  EXCEPTION WHEN OTHERS THEN errors := errors || ('a2: ' || SQLERRM);
  END;

  -- 3. средний чек
  BEGIN
    SELECT round(avg_check::numeric, 2) INTO x FROM mart.a3_avg_check;
    IF x IS DISTINCT FROM 159.88 THEN errors := errors || format('a3: средний чек %s, ожидалось 159.88', x); END IF;
    PERFORM avg_lines, avg_units FROM mart.a3_avg_check;
  EXCEPTION WHEN OTHERS THEN errors := errors || ('a3: ' || SQLERRM);
  END;

  -- 4. покупатели и повторные покупки
  BEGIN
    SELECT customers, round(returned_pct::numeric, 2) INTO n, x FROM mart.a4_customers;
    IF n <> 96096 THEN errors := errors || format('a4: покупателей %s, ожидалось 96096', n); END IF;
    IF x IS DISTINCT FROM 3.12 THEN errors := errors || format('a4: вернулись %s%%, ожидалось 3.12%%', x); END IF;
    PERFORM returned FROM mart.a4_customers;
  EXCEPTION WHEN OTHERS THEN errors := errors || ('a4: ' || SQLERRM);
  END;

  -- 5. выручка по категориям
  BEGIN
    SELECT count(*), round(max(revenue)::numeric, 2) INTO n, x FROM mart.a5_category_revenue;
    IF n <> 74 THEN errors := errors || format('a5: %s категорий, ожидалось 74', n); END IF;
    IF x IS DISTINCT FROM 1233131.72 THEN errors := errors || format('a5: у крупнейшей %s, ожидалось 1233131.72', x); END IF;
    SELECT round(sum(revenue)::numeric, 2) INTO x FROM mart.a5_category_revenue;
    IF x IS DISTINCT FROM total THEN errors := errors || format('a5: сумма по категориям %s, ожидалось %s', x, total); END IF;
    PERFORM category, share_pct FROM mart.a5_category_revenue LIMIT 1;
  EXCEPTION WHEN OTHERS THEN errors := errors || ('a5: ' || SQLERRM);
  END;

  IF cardinality(errors) > 0 THEN
    RAISE EXCEPTION E'контрольные цифры не сошлись:\n  %', array_to_string(errors, E'\n  ');
  END IF;
  RAISE NOTICE 'контрольные цифры блока А сошлись (5 из 5)';
END
$$;
