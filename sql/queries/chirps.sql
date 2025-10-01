-- name: CreateChirp :one
INSERT INTO chirps(id, created_at, updated_at, body, user_id)
VALUES (
    $1,
    now(),
    now(),
    $2,
    $3
)
RETURNING *;
