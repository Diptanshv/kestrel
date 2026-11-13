-- name: GetRollupState :one
SELECT hourly_through, daily_through FROM rollup_state WHERE id = 1;

-- name: SetRollupState :exec
INSERT INTO rollup_state (id, hourly_through, daily_through)
VALUES (1, sqlc.arg(hourly_through), sqlc.arg(daily_through))
ON CONFLICT (id) DO UPDATE
SET hourly_through = EXCLUDED.hourly_through,
    daily_through  = EXCLUDED.daily_through;

-- name: ListEventsForRollup :many
-- Everything the aggregator needs, oldest first. visitor_id stays in the
-- database process boundary only as long as the rollup run.
SELECT site_id, ts, visitor_id, session_id, name, pathname,
       coalesce(referrer, '')::text AS referrer,
       coalesce(country, '')::text  AS country,
       coalesce(browser, '')::text  AS browser,
       coalesce(os, '')::text       AS os,
       coalesce(device, '')::text   AS device
FROM events_raw
WHERE ts >= sqlc.arg(from_ts) AND ts < sqlc.arg(to_ts)
ORDER BY ts, id;

-- name: DeleteRollupHourlyRange :exec
-- Re-rolling replaces whole buckets: a value that has since been folded into
-- (other) must not survive from the previous run.
DELETE FROM rollup_hourly WHERE bucket >= sqlc.arg(from_ts) AND bucket < sqlc.arg(to_ts);

-- name: DeleteRollupDailyRange :exec
DELETE FROM rollup_daily WHERE day >= sqlc.arg(from_day) AND day < sqlc.arg(to_day);

-- name: UpsertRollupHourly :exec
-- Bulk upsert from parallel arrays; multiple unnest() calls in one select
-- list zip element-wise when the arrays are the same length.
INSERT INTO rollup_hourly (site_id, bucket, dimension, value, pageviews, visitors, visits, bounces, duration_sum)
SELECT
  unnest(sqlc.arg(site_ids)::bigint[]),
  unnest(sqlc.arg(buckets)::timestamptz[]),
  unnest(sqlc.arg(dimensions)::text[]),
  unnest(sqlc.arg(vals)::text[]),
  unnest(sqlc.arg(pageviews)::int[]),
  unnest(sqlc.arg(visitors)::int[]),
  unnest(sqlc.arg(visits)::int[]),
  unnest(sqlc.arg(bounces)::int[]),
  unnest(sqlc.arg(duration_sums)::bigint[])
ON CONFLICT (site_id, bucket, dimension, value) DO UPDATE
SET pageviews    = EXCLUDED.pageviews,
    visitors     = EXCLUDED.visitors,
    visits       = EXCLUDED.visits,
    bounces      = EXCLUDED.bounces,
    duration_sum = EXCLUDED.duration_sum;

-- name: UpsertRollupDaily :exec
INSERT INTO rollup_daily (site_id, day, dimension, value, pageviews, visitors, visits, bounces, duration_sum)
SELECT
  unnest(sqlc.arg(site_ids)::bigint[]),
  unnest(sqlc.arg(days)::date[]),
  unnest(sqlc.arg(dimensions)::text[]),
  unnest(sqlc.arg(vals)::text[]),
  unnest(sqlc.arg(pageviews)::int[]),
  unnest(sqlc.arg(visitors)::int[]),
  unnest(sqlc.arg(visits)::int[]),
  unnest(sqlc.arg(bounces)::int[]),
  unnest(sqlc.arg(duration_sums)::bigint[])
ON CONFLICT (site_id, day, dimension, value) DO UPDATE
SET pageviews    = EXCLUDED.pageviews,
    visitors     = EXCLUDED.visitors,
    visits       = EXCLUDED.visits,
    bounces      = EXCLUDED.bounces,
    duration_sum = EXCLUDED.duration_sum;

-- name: DeleteEventsRawBefore :execrows
DELETE FROM events_raw WHERE ts < $1;

-- name: DeleteRollupHourlyBefore :execrows
DELETE FROM rollup_hourly WHERE bucket < $1;

-- name: RollupDailyTotals :one
SELECT
  coalesce(sum(pageviews), 0)::bigint    AS pageviews,
  coalesce(sum(visitors), 0)::bigint     AS visitors,
  coalesce(sum(visits), 0)::bigint       AS visits,
  coalesce(sum(bounces), 0)::bigint      AS bounces,
  coalesce(sum(duration_sum), 0)::bigint AS duration_sum
FROM rollup_daily
WHERE site_id = sqlc.arg(site_id)
  AND dimension = '_total'
  AND day >= sqlc.arg(from_day) AND day < sqlc.arg(to_day);

-- name: RollupDailySeries :many
SELECT day, pageviews::bigint AS pageviews, visitors::bigint AS visitors
FROM rollup_daily
WHERE site_id = sqlc.arg(site_id)
  AND dimension = '_total'
  AND day >= sqlc.arg(from_day) AND day < sqlc.arg(to_day)
ORDER BY day;

-- name: RollupHourlySeries :many
SELECT bucket, pageviews::bigint AS pageviews, visitors::bigint AS visitors
FROM rollup_hourly
WHERE site_id = sqlc.arg(site_id)
  AND dimension = '_total'
  AND bucket >= sqlc.arg(from_ts) AND bucket < sqlc.arg(to_ts)
ORDER BY bucket;

-- name: RollupDailyBreakdown :many
SELECT value,
       sum(pageviews)::bigint AS pageviews,
       sum(visitors)::bigint  AS visitors
FROM rollup_daily
WHERE site_id = sqlc.arg(site_id)
  AND dimension = sqlc.arg(dimension)
  AND day >= sqlc.arg(from_day) AND day < sqlc.arg(to_day)
GROUP BY value
ORDER BY pageviews DESC, value ASC
LIMIT sqlc.arg(row_limit);

-- name: MinEventTs :one
-- Where a first-ever rollup run starts; fallback when there are no events.
SELECT coalesce(min(ts), sqlc.arg(fallback)::timestamptz)::timestamptz AS min_ts FROM events_raw;
