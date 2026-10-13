package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/keyboards"
	"github.com/adolgina/fitness-helper/internal/view"
)

func (b *Bot) showHistory(ctx context.Context, t target, userID int64, page int) {
	p, err := b.svc.History(ctx, userID, page)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	if len(p.Items) == 0 && p.Page == 0 {
		b.show(ctx, t, view.EmptyHistory, keyboards.NoWorkout())
		return
	}
	b.show(ctx, t, view.History(p), keyboards.History(p))
}

func (b *Bot) showHistoryWorkout(ctx context.Context, t target, userID int64, workoutID, page int) {
	d, err := b.svc.GetWorkout(ctx, userID, workoutID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.show(ctx, t, view.WorkoutDetail(d), keyboards.HistoryWorkout(page))
}

func (b *Bot) onWorkoutByDate(ctx context.Context, t target, userID int64, arg string) {
	if strings.TrimSpace(arg) == "" {
		b.say(ctx, t.chatID, view.WorkoutDateHint)
		return
	}
	day, err := parseWorkoutDate(arg, time.Now())
	if err != nil {
		b.say(ctx, t.chatID, "Не понял дату. "+view.WorkoutDateHint)
		return
	}
	ws, err := b.svc.WorkoutsOnDate(ctx, userID, day)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	if len(ws) == 0 {
		b.show(ctx, t, view.NoWorkoutOnDate, keyboards.Menu())
		return
	}
	for _, d := range ws {
		b.say(ctx, t.chatID, view.WorkoutDetail(d))
	}
}

// parseWorkoutDate reads a local calendar day. A date without a year means the current year.
func parseWorkoutDate(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"02.01.2006", "2.1.2006", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	if t, err := time.ParseInLocation("2.1", s, time.Local); err == nil {
		return time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), nil
	}
	return time.Time{}, domain.ErrInvalidInput
}
