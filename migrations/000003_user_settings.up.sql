CREATE TABLE user_settings (
    user_id BIGINT PRIMARY KEY,
    weekly_backup_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    last_backup_at TIMESTAMPTZ
);
