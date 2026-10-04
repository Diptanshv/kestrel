-- name: StatsSummary :one
SELECT
  count(*)::bigint                       AS pageviews,
  count(DISTINCT visitor_id)::bigint     AS visitors
FROM events_raw
WHERE site_id = sqlc.arg(site_id)
  AND ts >= sqlc.arg(from_ts)
  AND ts <  sqlc.arg(to_ts)
  AND name = 'pageview';

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
