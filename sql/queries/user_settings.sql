-- name: GetUserSettings :one
SELECT user_id, weekly_backup_enabled, last_backup_at
FROM user_settings
WHERE user_id = $1;

-- name: UpsertUserSettings :exec
INSERT INTO user_settings (user_id, weekly_backup_enabled, last_backup_at)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET
    weekly_backup_enabled = EXCLUDED.weekly_backup_enabled,
    last_backup_at = COALESCE(EXCLUDED.last_backup_at, user_settings.last_backup_at);

-- name: SetLastBackupAt :exec
INSERT INTO user_settings (user_id, weekly_backup_enabled, last_backup_at)
VALUES ($1, TRUE, $2)
ON CONFLICT (user_id) DO UPDATE SET last_backup_at = EXCLUDED.last_backup_at;

-- name: ListUsersDueWeeklyBackup :many
SELECT user_id
FROM user_settings
WHERE weekly_backup_enabled = TRUE
  AND (last_backup_at IS NULL OR last_backup_at < $1);
