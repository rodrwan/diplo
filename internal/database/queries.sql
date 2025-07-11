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

-- Environment Variables queries
-- name: CreateAppEnvVar :exec
INSERT INTO app_env_vars (app_id, key, value, is_secret, updated_at)
VALUES (?, ?, ?, ?, ?);

-- name: GetAppEnvVars :many
SELECT id, app_id, key, value, is_secret, created_at, updated_at
FROM app_env_vars WHERE app_id = ?;

-- name: GetAppEnvVar :one
SELECT id, app_id, key, value, is_secret, created_at, updated_at
FROM app_env_vars WHERE app_id = ? AND key = ?;

-- name: UpdateAppEnvVar :exec
UPDATE app_env_vars SET value = ?, is_secret = ?, updated_at = ? WHERE app_id = ? AND key = ?;

-- name: DeleteAppEnvVar :exec
DELETE FROM app_env_vars WHERE app_id = ? AND key = ?;

-- name: DeleteAllAppEnvVars :exec
DELETE FROM app_env_vars WHERE app_id = ?;