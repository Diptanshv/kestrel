-- name: UpsertSalt :one
-- The no-op DO UPDATE is deliberate: DO NOTHING returns no row on conflict, so
-- two instances racing on the first request of a day would leave one of them
-- empty-handed. This way the winner's salt always comes back.
INSERT INTO salts (day, salt)
VALUES ($1, $2)
ON CONFLICT (day) DO UPDATE SET day = EXCLUDED.day
RETURNING salt;

-- name: DeleteSaltsBefore :exec
DELETE FROM salts WHERE day < $1;