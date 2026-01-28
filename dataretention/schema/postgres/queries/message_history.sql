-- name: CreateMessageHistory :one
INSERT INTO message_history (message_id, message_type, status, payload, error_message, processing_duration_ms)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetMessageHistory :many
SELECT * FROM message_history
WHERE message_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: GetMessageDetailsForHistory :one
SELECT message_type, payload FROM message_queue
WHERE id = $1;

