-- name: CreateDataRetentionProperties :one
INSERT INTO data_retention_properties (
    data_extension_id, data_retention_period_length, data_retention_period_unit_of_measure,
    is_delete_at_end_of_retention_period, is_row_based_retention, is_reset_retention_period_on_import
) VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetDataRetentionPropertiesByDataExtensionID :one
SELECT * FROM data_retention_properties
WHERE data_extension_id = $1;

-- name: UpdateDataRetentionProperties :one
UPDATE data_retention_properties
SET data_retention_period_length = $2,
    data_retention_period_unit_of_measure = $3,
    is_delete_at_end_of_retention_period = $4,
    is_row_based_retention = $5,
    is_reset_retention_period_on_import = $6
WHERE data_extension_id = $1
RETURNING *;

-- name: DeleteDataRetentionProperties :exec
DELETE FROM data_retention_properties
WHERE data_extension_id = $1;

-- name: UpdateDataRetentionAPIUpdateStatus :one
UPDATE data_retention_properties
SET last_api_update_at = CURRENT_TIMESTAMP,
    last_api_update_status = sqlc.arg('last_api_update_status')::VARCHAR,
    last_api_update_error = sqlc.arg('last_api_update_error'),
    api_update_retry_count = CASE 
        WHEN sqlc.arg('last_api_update_status')::VARCHAR = 'failed' THEN api_update_retry_count + 1
        WHEN sqlc.arg('last_api_update_status')::VARCHAR = 'succeeded' THEN 0
        ELSE api_update_retry_count
    END,
    -- Update retention properties when status is 'succeeded'
    data_retention_period_length = CASE 
        WHEN sqlc.arg('last_api_update_status')::VARCHAR = 'succeeded' THEN sqlc.arg('data_retention_period_length')
        ELSE data_retention_period_length
    END,
    data_retention_period_unit_of_measure = CASE 
        WHEN sqlc.arg('last_api_update_status')::VARCHAR = 'succeeded' THEN sqlc.arg('data_retention_period_unit_of_measure')
        ELSE data_retention_period_unit_of_measure
    END,
    is_row_based_retention = CASE 
        WHEN sqlc.arg('last_api_update_status')::VARCHAR = 'succeeded' THEN sqlc.arg('is_row_based_retention')
        ELSE is_row_based_retention
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE data_extension_id = sqlc.arg('data_extension_id')
RETURNING *;

-- name: GetDataExtensionsNeedingRetentionUpdate :many
SELECT drp.*, de.name as data_extension_name
FROM data_retention_properties drp
INNER JOIN data_extensions de ON drp.data_extension_id = de.id
WHERE drp.last_api_update_status IN ('pending', 'failed')
  AND (drp.api_update_retry_count < 5 OR drp.api_update_retry_count IS NULL)
ORDER BY drp.last_api_update_at ASC NULLS FIRST, drp.api_update_retry_count ASC
LIMIT $1;

-- name: ResetDataRetentionAPIUpdateStatus :one
UPDATE data_retention_properties
SET last_api_update_status = 'pending',
    last_api_update_error = NULL,
    api_update_retry_count = 0,
    updated_at = CURRENT_TIMESTAMP
WHERE data_extension_id = $1
RETURNING *;

