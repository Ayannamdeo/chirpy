-- name: CreateChirp :one
INSERT INTO chirps (
    body,      -- will be $1
    user_id    -- will be $2
) VALUES (
    $1,        -- this is the body text
    $2         -- this is the user_id
)
RETURNING *;

-- name: GetAllChirps :many
SELECT * FROM chirps order by created_at asc;

-- name: GetChirpsById :one
SELECT * FROM chirps where id = $1;

-- name: DeleteAllChirps :exec
DELETE FROM chirps;
