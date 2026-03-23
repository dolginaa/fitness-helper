package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Session struct {
	q *sqlcgen.Queries
}

func NewSession(q *sqlcgen.Queries) *Session {
	return &Session{q: q}
}

func (s *Session) Get(ctx context.Context, userID int64) (*int, domain.SessionState, error) {
	row, err := s.q.GetUserWorkoutState(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.SessionState{Phase: domain.PhaseIdle}, nil
		}
		return nil, domain.SessionState{}, err
	}
	var wid *int
	if row.ActiveWorkoutID.Valid {
		v := int(row.ActiveWorkoutID.Int32)
		wid = &v
	}
	var st domain.SessionState
	if len(row.State) > 0 && string(row.State) != "{}" {
		if err := json.Unmarshal(row.State, &st); err != nil {
			return wid, domain.SessionState{Phase: domain.PhaseIdle}, nil
		}
	}
	if st.Phase == "" {
		st.Phase = domain.PhaseIdle
	}
	return wid, st, nil
}

func (s *Session) Save(ctx context.Context, userID int64, activeWorkoutID *int, state domain.SessionState) error {
	b, err := json.Marshal(state)
	if err != nil {
		return err
	}
	var aw pgtype.Int4
	if activeWorkoutID != nil {
		aw = pgtype.Int4{Int32: int32(*activeWorkoutID), Valid: true}
	}
	return s.q.UpsertUserWorkoutState(ctx, sqlcgen.UpsertUserWorkoutStateParams{
		UserID:          userID,
		ActiveWorkoutID: aw,
		State:           b,
	})
}

func (s *Session) Clear(ctx context.Context, userID int64) error {
	return s.q.ClearUserWorkoutState(ctx, userID)
}
