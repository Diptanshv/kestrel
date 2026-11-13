-- name: StatsSummary :one
SELECT
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name = 'pageview';

-- name: StatsSessions :one
-- A visit is one session_id. Bounce: exactly one pageview and no custom event.
-- Duration: last event minus first, so a bounce contributes 0 seconds.
-- session_id 0 marks rows written before sessions existed; they are skipped.
WITH s AS (
  SELECT
    count(*) FILTER (WHERE name = 'pageview')  AS pageviews,
    count(*) FILTER (WHERE name <> 'pageview') AS custom,
    extract(epoch FROM max(ts) - min(ts))      AS duration
  FROM events_raw
  WHERE site_id = sqlc.arg(site_id)
    AND ts >= sqlc.arg(from_ts)
    AND ts <  sqlc.arg(to_ts)
    AND session_id <> 0
  GROUP BY session_id
)
SELECT
  count(*)::bigint                                              AS visits,
  count(*) FILTER (WHERE pageviews = 1 AND custom = 0)::bigint AS bounces,
  coalesce(sum(duration), 0)::bigint                            AS duration_sum
FROM s;

-- name: StatsTimeseriesHourly :many
SELECT
  b.bucket::timestamptz                  AS bucket,
  count(e.id)::bigint                    AS pageviews,
  count(DISTINCT e.visitor_id)::bigint   AS visitors
FROM generate_series(
  date_trunc('hour', sqlc.arg(from_ts)::timestamptz, sqlc.arg(tz)),
  date_trunc('hour', sqlc.arg(to_ts)::timestamptz - interval '1 microsecond', sqlc.arg(tz)),
  interval '1 hour'
) AS b(bucket)
LEFT JOIN events_raw e
  ON e.site_id = sqlc.arg(site_id)
 AND e.name = 'pageview'
 AND e.ts >= b.bucket
 AND e.ts <  b.bucket + interval '1 hour'
GROUP BY b.bucket
ORDER BY b.bucket;

-- name: StatsTimeseriesDaily :many
SELECT
  b.bucket::timestamptz                  AS bucket,
  count(e.id)::bigint                    AS pageviews,
  count(DISTINCT e.visitor_id)::bigint   AS visitors
FROM generate_series(
  date_trunc('day', sqlc.arg(from_ts)::timestamptz, sqlc.arg(tz)),
  date_trunc('day', sqlc.arg(to_ts)::timestamptz - interval '1 microsecond', sqlc.arg(tz)),
  interval '1 day'
) AS b(bucket)
LEFT JOIN events_raw e
  ON e.site_id = sqlc.arg(site_id)
 AND e.name = 'pageview'
 AND e.ts >= b.bucket
 AND e.ts <  b.bucket + interval '1 day'
GROUP BY b.bucket
ORDER BY b.bucket;

-- name: StatsBreakdownPages :many
SELECT
  pathname                               AS value,
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name = 'pageview'
GROUP BY pathname
ORDER BY pageviews DESC, value ASC
LIMIT sqlc.arg(row_limit);

-- name: StatsBreakdownReferrers :many
SELECT
  coalesce(referrer, '')::text           AS value,
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name = 'pageview'
  AND referrer IS NOT NULL
GROUP BY referrer
ORDER BY pageviews DESC, value ASC
LIMIT sqlc.arg(row_limit);
-- name: StatsBreakdownEvents :many
SELECT
  name                                   AS value,
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name <> 'pageview'
GROUP BY name
ORDER BY pageviews DESC, value ASC
LIMIT sqlc.arg(row_limit);

-- name: StatsBreakdownCountries :many
SELECT
  coalesce(country, '')::text            AS value,
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name = 'pageview'
  AND country IS NOT NULL
GROUP BY country
ORDER BY pageviews DESC, value ASC
LIMIT sqlc.arg(row_limit);

-- name: StatsBreakdownBrowsers :many
SELECT
  coalesce(browser, '')::text            AS value,
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name = 'pageview'
  AND browser IS NOT NULL AND browser <> ''
GROUP BY browser
ORDER BY pageviews DESC, value ASC
LIMIT sqlc.arg(row_limit);

-- name: StatsBreakdownOS :many
SELECT
  coalesce(os, '')::text                 AS value,
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name = 'pageview'
  AND os IS NOT NULL AND os <> ''
GROUP BY os
ORDER BY pageviews DESC, value ASC
LIMIT sqlc.arg(row_limit);

-- name: StatsBreakdownDevices :many
SELECT
  coalesce(device, '')::text             AS value,
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name = 'pageview'
  AND device IS NOT NULL AND device <> ''
GROUP BY device
ORDER BY pageviews DESC, value ASC
LIMIT sqlc.arg(row_limit);
