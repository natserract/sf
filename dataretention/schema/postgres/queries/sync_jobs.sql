-- name: CreateSyncJob :one
INSERT INTO sync_jobs (job_type, status, total_items, metadata)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetSyncJobByID :one
SELECT * FROM sync_jobs
WHERE id = $1;

-- name: UpdateSyncJobStatus :exec
UPDATE sync_jobs
SET status = $1, updated_at = CURRENT_TIMESTAMP
WHERE id = $2;

-- name: UpdateSyncJobProgress :exec
UPDATE sync_jobs
SET processed_items = processed_items + $1,
    succeeded_items = succeeded_items + $2,
    failed_items = failed_items + $3,
    error_rate = CASE 
        WHEN (processed_items + $1) > 0 THEN ROUND(((failed_items + $3)::DECIMAL / (processed_items + $1)::DECIMAL) * 100, 2)
        ELSE 0.00
    END,
    success_rate = CASE
        WHEN (processed_items + $1) > 0 THEN ROUND(((succeeded_items + $2)::DECIMAL / (processed_items + $1)::DECIMAL) * 100, 2)
        ELSE 0.00
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $4;

-- name: CompleteSyncJob :exec
UPDATE sync_jobs
SET status = $1,
    completed_at = CURRENT_TIMESTAMP,
    duration_ms = $2,
    avg_processing_time_ms = $3,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $4;

-- name: FailSyncJob :exec
UPDATE sync_jobs
SET status = $1,
    completed_at = CURRENT_TIMESTAMP,
    error_message = $2,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $3;

-- name: CancelSyncJob :exec
UPDATE sync_jobs
SET status = $1,
    completed_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $2;

-- name: GetSyncJobsByStatus :many
SELECT * FROM sync_jobs
WHERE status = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: GetSyncJobsByType :many
SELECT * FROM sync_jobs
WHERE job_type = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: GetRecentSyncJobs :many
SELECT * FROM sync_jobs
WHERE created_at >= $1
ORDER BY created_at DESC;

-- name: GetSyncJobMetrics :one
SELECT 
    COUNT(*) as total_jobs,
    COUNT(*) FILTER (WHERE status = 'completed') as completed_jobs,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_jobs,
    COUNT(*) FILTER (WHERE status = 'running') as running_jobs,
    SUM(total_items) as total_items_processed,
    SUM(succeeded_items) as total_succeeded,
    SUM(failed_items) as total_failed,
    AVG(success_rate) as avg_success_rate,
    AVG(error_rate) as avg_error_rate,
    AVG(duration_ms) as avg_duration_ms
FROM sync_jobs
WHERE created_at >= $1;

-- name: ListAllSyncJobs :many
SELECT * FROM sync_jobs
ORDER BY created_at DESC
LIMIT $1;

