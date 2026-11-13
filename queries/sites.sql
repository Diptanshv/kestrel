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

-- name: GetSiteByDomain :one 
SELECT * FROM sites WHERE domain = $1;

-- name: SetSitePublicSlug :one
-- Separate from UpdateSite because that one cannot set a column to NULL.
UPDATE sites SET public_slug = sqlc.narg(public_slug) WHERE id = sqlc.arg(id)
RETURNING *;

-- name: GetSiteByPublicSlug :one
SELECT * FROM sites WHERE public_slug = $1;
