-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (
    gen_random_uuid(),
    NOW(),
    NOW(),
    $1,
    $2
    
)
RETURNING id, created_at, updated_at, email, hashed_password, is_chirpy_red;

-- name: DeleteUsers :exec
DELETE FROM users;

-- name: GetUserByEmail :one
SELECT id, created_at, updated_at, email, hashed_password, is_chirpy_red
FROM users
WHERE email = $1;

-- name: UpdateUserByID :one
UPDATE users
SET
    email = $2,
    hashed_password = $3,
    updated_at = NOW()
WHERE id = $1
RETURNING id, created_at, updated_at, email;

-- name: UpgradeRedByID :one
UPDATE users
SET
    is_chirpy_red = TRUE
WHERE id = $1
RETURNING id, is_chirpy_red;