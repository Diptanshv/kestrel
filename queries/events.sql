-- name: CreateEventRaw :one
INSERT INTO events_raw (site_id, ts, visitor_id, session_id, name, pathname, referrer, utm_source, country, browser, os, device)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: ListEventsRawBySiteID :many
SELECT * FROM events_raw
WHERE site_id = $1
  AND ts >= $2
  AND ts < $3
ORDER BY ts DESC, id DESC
LIMIT $4;

-- name: GetEventRawByID :one
SELECT * FROM events_raw WHERE id = $1;

-- name: DeleteEventRaw :exec
DELETE FROM events_raw WHERE id = $1;