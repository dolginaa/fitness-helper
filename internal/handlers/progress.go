package handlers

import (
	"context"
	"errors"

	"github.com/adolgina/fitness-helper/internal/chat"
	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/keyboards"
	"github.com/adolgina/fitness-helper/internal/view"
)

func (b *Bot) showProgressGroups(ctx context.Context, t target) {
	groups, err := b.svc.ListMuscleGroups(ctx)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.show(ctx, t, view.ProgressGroups, keyboards.ProgressGroups(groups))
}

// showProgressGroup shows the last result of every exercise of the group.
func (b *Bot) showProgressGroup(ctx context.Context, t target, userID int64, muscleGroupID int) {
	list, err := b.svc.ExerciseOverview(ctx, userID, muscleGroupID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.show(ctx, t, view.ExerciseList(list, false), keyboards.ProgressExercises(list))
}

// sendExerciseProgress sends the recent results of the exercise, with a chart when there is enough data.
func (b *Bot) sendExerciseProgress(ctx context.Context, chatID int64, userID int64, exerciseID int) {
	p, err := b.svc.ExerciseProgress(ctx, userID, exerciseID)
	if err != nil {
		b.fail(ctx, newMessage(chatID), err)
		return
	}
	png, err := b.charts.ExerciseProgress(p.Exercise.Name, p.Points)
	if errors.Is(err, domain.ErrNotEnoughData) {
		b.say(ctx, chatID, view.ExerciseProgress(p))
		return
	}
	if err != nil {
		b.fail(ctx, newMessage(chatID), err)
		return
	}
	b.sendPhotos(ctx, chatID, chat.Photo{PNG: png, Caption: view.ExerciseProgress(p)})
}

// sendExerciseChart sends the chart of the exercise chosen in a workout.
// It stays silent when there is nothing to draw: the exercise screen already shows the last result.
func (b *Bot) sendExerciseChart(ctx context.Context, chatID int64, userID int64, exerciseID int) {
	p, err := b.svc.ExerciseProgress(ctx, userID, exerciseID)
	if err != nil {
		b.log.Error("exercise progress", "err", err)
		return
	}
	png, err := b.charts.ExerciseProgress(p.Exercise.Name, p.Points)
	if err != nil {
		if !errors.Is(err, domain.ErrNotEnoughData) {
			b.log.Error("exercise chart", "err", err)
		}
		return
	}
	b.sendPhotos(ctx, chatID, chat.Photo{PNG: png})
}

func (b *Bot) sendGroupChart(ctx context.Context, chatID int64, userID int64, muscleGroupID int) {
	p, err := b.svc.MuscleGroupProgress(ctx, userID, muscleGroupID)
	if err != nil {
		b.fail(ctx, newMessage(chatID), err)
		return
	}
	png, err := b.charts.MuscleGroupProgress(p.Group.Name, p.Series)
	if errors.Is(err, domain.ErrNotEnoughData) {
		b.say(ctx, chatID, view.NoGroupChartData)
		return
	}
	if err != nil {
		b.fail(ctx, newMessage(chatID), err)
		return
	}
	b.sendPhotos(ctx, chatID, chat.Photo{PNG: png})
}

// sendGroupExerciseCharts sends a chart for every exercise of the group that has enough data.
func (b *Bot) sendGroupExerciseCharts(ctx context.Context, chatID int64, userID int64, muscleGroupID int) {
	list, err := b.svc.ExerciseOverview(ctx, userID, muscleGroupID)
	if err != nil {
		b.fail(ctx, newMessage(chatID), err)
		return
	}
	var photos []chat.Photo
	for _, r := range list.Rows {
		p, err := b.svc.ExerciseProgress(ctx, userID, r.Exercise.ID)
		if err != nil {
			b.fail(ctx, newMessage(chatID), err)
			return
		}
		png, err := b.charts.ExerciseProgress(p.Exercise.Name, p.Points)
		if err != nil {
			if !errors.Is(err, domain.ErrNotEnoughData) {
				b.log.Error("exercise chart", "err", err)
			}
			continue
		}
		photos = append(photos, chat.Photo{PNG: png})
	}
	if len(photos) == 0 {
		b.say(ctx, chatID, view.NoGroupChartData)
		return
	}
	b.sendPhotos(ctx, chatID, photos...)
}
