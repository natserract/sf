-- name: CreateDataExtension :one
INSERT INTO data_extensions (
    id, name, key, description, is_active, is_sendable, sendable_custom_object_field,
    sendable_subscriber_field, is_testable, category_id, owner_id, is_object_deletable,
    is_field_addition_allowed, is_field_modification_allowed, created_date, created_by_id,
    created_by_name, modified_date, modified_by_id, modified_by_name, owner_name,
    partner_api_object_type_id, partner_api_object_type_name, row_count, field_count
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
)
RETURNING *;

-- name: GetDataExtensionByID :one
SELECT * FROM data_extensions
WHERE id = $1;

-- name: GetDataExtensionByKey :one
SELECT * FROM data_extensions
WHERE key = $1;

-- name: GetDataExtensionsByCategoryID :many
SELECT * FROM data_extensions
WHERE category_id = $1
ORDER BY modified_date DESC;

-- name: GetDataExtensionsByCategoryIDPaginated :many
SELECT * FROM data_extensions
WHERE category_id = $1
ORDER BY modified_date DESC
LIMIT $2 OFFSET $3;

-- name: UpdateDataExtension :one
UPDATE data_extensions
SET name = $2, description = $3, is_active = $4, modified_date = $5, modified_by_id = $6, modified_by_name = $7, row_count = $8, field_count = $9
WHERE id = $1
RETURNING *;

-- name: DeleteDataExtension :exec
DELETE FROM data_extensions
WHERE id = $1;

