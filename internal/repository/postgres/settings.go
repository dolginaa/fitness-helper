package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Settings struct {
	q *sqlcgen.Queries
}

func NewSettings(q *sqlcgen.Queries) *Settings {
	return &Settings{q: q}
}

func (s *Settings) GetSettings(ctx context.Context, userID int64) (domain.UserSettings, error) {
	row, err := s.q.GetUserSettings(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.UserSettings{UserID: userID}, nil
		}
		return domain.UserSettings{}, err
	}
	var last *time.Time
	if row.LastBackupAt.Valid {
		t := row.LastBackupAt.Time
		last = &t
	}
	return domain.UserSettings{
		UserID:              row.UserID,
		WeeklyBackupEnabled: row.WeeklyBackupEnabled,
		LastBackupAt:        last,
	}, nil
}

func (s *Settings) SetWeeklyBackup(ctx context.Context, userID int64, enabled bool) error {
	cur, err := s.GetSettings(ctx, userID)
	if err != nil {
		return err
	}
	var last pgtype.Timestamptz
	if cur.LastBackupAt != nil {
		last = pgTimestamptz(*cur.LastBackupAt)
	}
	return s.q.UpsertUserSettings(ctx, sqlcgen.UpsertUserSettingsParams{
		UserID:              userID,
		WeeklyBackupEnabled: enabled,
		LastBackupAt:        last,
	})
}

func (s *Settings) SetLastBackupAt(ctx context.Context, userID int64, at time.Time) error {
	return s.q.SetLastBackupAt(ctx, sqlcgen.SetLastBackupAtParams{
		UserID:       userID,
		LastBackupAt: pgTimestamptz(at),
	})
}

func (s *Settings) ListUsersDueWeeklyBackup(ctx context.Context, before time.Time) ([]int64, error) {
	return s.q.ListUsersDueWeeklyBackup(ctx, pgTimestamptz(before))
}

var _ domain.SettingsRepository = (*Settings)(nil)
