-- name: CreateFolder :one
INSERT INTO folders (id, type, last_updated, created_by, parent_id, name, description, icon_type)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetFolderByID :one
SELECT * FROM folders
WHERE id = $1;

-- name: GetFoldersByParentID :many
SELECT * FROM folders
WHERE parent_id = $1
ORDER BY name ASC;

-- name: GetFoldersByType :many
SELECT * FROM folders
WHERE type = $1
ORDER BY name ASC;

-- name: UpdateFolder :one
UPDATE folders
SET type = $2, last_updated = $3, name = $4, description = $5, icon_type = $6
WHERE id = $1
RETURNING *;

-- name: DeleteFolder :exec
DELETE FROM folders
WHERE id = $1;

-- name: ListAllFolders :many
SELECT * FROM folders
ORDER BY name ASC;

