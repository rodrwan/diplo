-- name: CreateApp :exec
INSERT INTO apps (id, name, repo_url, language, port, container_id, image_id, status, error_msg, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateApp :exec
UPDATE apps SET name = ?, repo_url = ?, language = ?, port = ?, container_id = ?, image_id = ?, status = ?, error_msg = ?, updated_at = ? WHERE id = ?;

-- name: GetApp :one
SELECT * FROM apps WHERE id = ?;

-- name: GetAppByRepoUrl :one
SELECT * FROM apps WHERE repo_url = ?;

-- name: GetAllApps :many
SELECT id, name, repo_url, language, port, container_id, image_id,
    status, error_msg, created_at, updated_at
FROM apps;

-- name: DeleteApp :exec
DELETE FROM apps WHERE id = ?;