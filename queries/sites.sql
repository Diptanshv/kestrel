-- name: CreateSite :one
INSERT INTO sites (user_id, domain, public_slug)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListSitesByUserID :many
SELECT * FROM sites WHERE user_id = $1 ORDER BY id ;

-- name: GetSiteByID :one
SELECT * FROM sites WHERE id = $1;

-- name: UpdateSite :one
UPDATE sites 
SET
    domain = COALESCE(sqlc.narg('domain'), domain),
    public_slug = COALESCE(sqlc.narg('public_slug'), public_slug)
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteSite :exec
DELETE FROM sites WHERE id=$1;