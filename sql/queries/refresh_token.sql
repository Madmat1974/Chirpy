-- name: RefreshToken :one
INSERT INTO refresh_tokens (token, created_at, updated_at, user_id, expires_at, revoked_at)
VALUES (
    $1,
    NOW(),
    NOW(),
    $2,
    NOW() + INTERVAL '60 days',
    NULL  
)
RETURNING *;

-- name: GetUserFromRefreshToken :one
SELECT users.id, users.email, users.created_at, users.updated_at
FROM refresh_tokens 
JOIN users ON refresh_tokens.user_id = users.id
WHERE refresh_tokens.token = $1
    AND refresh_tokens.revoked_at IS NULL
    AND refresh_tokens.expires_at > now()
LIMIT 1;

-- name: RevokeRefreshToken :execrows
UPDATE refresh_tokens
SET revoked_at = NOW(), updated_at = NOW()
WHERE token = $1 AND revoked_at IS NULL;