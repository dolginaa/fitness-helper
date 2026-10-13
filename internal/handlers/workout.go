package handlers

import (
	"context"
	"errors"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/keyboards"
	"github.com/adolgina/fitness-helper/internal/usecase"
	"github.com/adolgina/fitness-helper/internal/view"
)

// onNewWorkout starts a workout. Unless forced, it first asks what to do with an unfinished one.
func (b *Bot) onNewWorkout(ctx context.Context, t target, userID int64, force bool) {
	if !force {
		w, active, err := b.svc.ActiveWorkout(ctx, userID)
		if err != nil {
			b.fail(ctx, t, err)
			return
		}
		if active {
			b.show(ctx, t, view.ActiveWorkoutPrompt(w), keyboards.ActiveWorkoutPrompt())
			return
		}
	}
	if _, err := b.svc.StartNewWorkout(ctx, userID); err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showWorkoutGroups(ctx, t, view.WorkoutStarted)
}

func (b *Bot) showWorkoutGroups(ctx context.Context, t target, text string) {
	groups, err := b.svc.ListMuscleGroups(ctx)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.show(ctx, t, text, keyboards.WorkoutGroups(groups))
}

func (b *Bot) showExerciseList(ctx context.Context, t target, list usecase.ExerciseList) {
	b.show(ctx, t, view.ExerciseList(list, true), keyboards.WorkoutExercises(list))
}

func (b *Bot) showExerciseLog(ctx context.Context, t target, log usecase.ExerciseLog) {
	b.show(ctx, t, view.ExerciseLog(log), keyboards.ExerciseLog(log))
}

// showWorkout renders the screen the user stopped at in the active workout.
func (b *Bot) showWorkout(ctx context.Context, t target, userID int64) {
	st, err := b.svc.Resume(ctx, userID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	switch st.Phase {
	case domain.PhaseLoggingSets:
		log, err := b.svc.CurrentExerciseLog(ctx, userID)
		if err != nil {
			b.fail(ctx, t, err)
			return
		}
		b.showExerciseLog(ctx, t, log)
	case domain.PhaseSelectExercise, domain.PhaseAddingExercise:
		b.onBackToExercises(ctx, t, userID)
	default:
		b.onBackToGroups(ctx, t, userID)
	}
}

func (b *Bot) onMuscleGroup(ctx context.Context, t target, userID int64, muscleGroupID int) {
	list, err := b.svc.SelectMuscleGroup(ctx, userID, muscleGroupID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showExerciseList(ctx, t, list)
}

func (b *Bot) onExercise(ctx context.Context, t target, userID int64, exerciseID int) {
	log, err := b.svc.BeginExercise(ctx, userID, exerciseID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showExerciseLog(ctx, t, log)
	b.sendExerciseChart(ctx, t.chatID, userID, exerciseID)
}

func (b *Bot) onSetInput(ctx context.Context, t target, userID int64, text string) {
	log, err := b.svc.LogInput(ctx, userID, text)
	if errors.Is(err, domain.ErrInvalidInput) {
		b.say(ctx, t.chatID, view.SetInputHint)
		return
	}
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showExerciseLog(ctx, t, log)
}

func (b *Bot) onRepeatSet(ctx context.Context, t target, userID int64, exerciseID int) {
	log, err := b.svc.RepeatSet(ctx, userID, exerciseID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showExerciseLog(ctx, t, log)
}

func (b *Bot) onUndoSet(ctx context.Context, t target, userID int64, exerciseID int) {
	log, err := b.svc.UndoLastSet(ctx, userID, exerciseID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showExerciseLog(ctx, t, log)
}

func (b *Bot) onBackToExercises(ctx context.Context, t target, userID int64) {
	list, err := b.svc.BackToExercises(ctx, userID)
	if errors.Is(err, domain.ErrInvalidState) {
		// No muscle group chosen yet: the group choice is the closest valid screen.
		b.onBackToGroups(ctx, t, userID)
		return
	}
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showExerciseList(ctx, t, list)
}

func (b *Bot) onBackToGroups(ctx context.Context, t target, userID int64) {
	if err := b.svc.BackToGroups(ctx, userID); err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showWorkoutGroups(ctx, t, view.WorkoutGroups)
}

func (b *Bot) onAddExercise(ctx context.Context, t target, userID int64) {
	group, err := b.svc.BeginAddExercise(ctx, userID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.show(ctx, t, view.NewExercisePrompt(group), keyboards.NewExercisePrompt())
}

func (b *Bot) onExerciseName(ctx context.Context, t target, userID int64, name string) {
	log, err := b.svc.AddExercise(ctx, userID, name)
	if errors.Is(err, domain.ErrInvalidInput) {
		b.show(ctx, t, view.ExerciseNameHint, keyboards.NewExercisePrompt())
		return
	}
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.showExerciseLog(ctx, t, log)
}

// onEndWorkout shows what was done and asks to confirm; confirmed, it completes the workout.
func (b *Bot) onEndWorkout(ctx context.Context, t target, userID int64, confirmed bool) {
	if !confirmed {
		d, err := b.svc.CurrentWorkout(ctx, userID)
		if err != nil {
			b.fail(ctx, t, err)
			return
		}
		b.show(ctx, t, view.EndWorkoutConfirm(d), keyboards.EndWorkoutConfirm())
		return
	}
	d, err := b.svc.EndWorkout(ctx, userID)
	if err != nil {
		b.fail(ctx, t, err)
		return
	}
	b.show(ctx, t, view.WorkoutFinished(d), keyboards.WorkoutFinished())
}
