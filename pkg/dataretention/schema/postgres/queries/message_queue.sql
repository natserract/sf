-- name: EnqueueMessage :one
INSERT INTO message_queue (message_type, payload, priority, status, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: DequeueMessages :many
UPDATE message_queue
SET status = $1, processed_at = $2
WHERE id IN (
    SELECT id FROM message_queue mq
    WHERE mq.status = $3
        AND (mq.next_retry_at IS NULL OR mq.next_retry_at <= $2)
    ORDER BY mq.priority ASC, mq.created_at ASC
    LIMIT $4
    FOR UPDATE SKIP LOCKED
)
RETURNING *;

-- name: GetMessageByID :one
SELECT * FROM message_queue
WHERE id = $1;

-- name: UpdateMessageStatus :exec
UPDATE message_queue
SET status = $1, processed_at = $2
WHERE id = $3;

-- name: UpdateMessageStatusWithError :exec
UPDATE message_queue
SET status = $1, processed_at = $2, error_message = $4
WHERE id = $3;

-- name: FailMessageWithRetry :exec
UPDATE message_queue
SET status = $1, retry_count = $2, error_message = $3, next_retry_at = $4
WHERE id = $5;

-- name: GetMessageState :one
SELECT retry_count, max_retries FROM message_queue
WHERE id = $1;

-- name: GetDeadLetterMessages :many
SELECT * FROM message_queue
WHERE status = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: GetPendingMessagesForRetry :many
SELECT * FROM message_queue
WHERE status = $1
    AND next_retry_at IS NOT NULL
    AND next_retry_at <= $2
ORDER BY priority ASC, created_at ASC
LIMIT $3;

