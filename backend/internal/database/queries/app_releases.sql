-- name: InsertAppRelease :one
INSERT INTO app_releases (version, git_sha, built_at, notes)
VALUES ($1, $2, $3, $4)
RETURNING id, deployed_at;

-- name: GetLatestAppRelease :one
SELECT id, version, git_sha, built_at, notes, deployed_at, created_at
FROM app_releases
ORDER BY deployed_at DESC, id DESC
LIMIT 1;
