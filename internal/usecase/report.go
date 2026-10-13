package usecase

import (
	"context"
	"math"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

// minWorkoutDuration and maxWorkoutDuration bound the durations that are believed:
// a workout of seconds was opened and closed by accident, and a longer one was most
// likely left open and ended much later than it really finished.
const (
	minWorkoutDuration = time.Minute
	maxWorkoutDuration = 6 * time.Hour
)

type RecordKind string

const (
	RecordWeight    RecordKind = "weight"
	RecordVolume    RecordKind = "volume"
	RecordReps      RecordKind = "reps"
	RecordTotalReps RecordKind = "total_reps"
)

// weekDays is the window of the "workouts this week" count.
const weekDays = 7

// Record is something an exercise did better in this workout than ever before.
type Record struct {
	Exercise string
	Kind     RecordKind
	Value    float64
	Previous float64
}

// ExerciseChange compares an exercise with the last time it was done in the same kind of load.
type ExerciseChange struct {
	Exercise string
	Load     domain.LoadKind
	Before   domain.ExerciseProgressPoint
	Now      domain.ExerciseProgressPoint
}

// WorkoutReport is what is worth saying when a workout ends, apart from what was done in it.
type WorkoutReport struct {
	Workout domain.WorkoutDetail
	// Duration is zero when it is not known or not believable.
	Duration time.Duration
	Sets     int
	Reps     int
	Volume   float64
	Groups   []string

	// PrevDate and PrevVolume describe the workout before this one; PrevDate is zero if there was none.
	PrevDate   time.Time
	PrevVolume float64
	// DaysSincePrev is the number of calendar days since PrevDate.
	DaysSincePrev int
	// WeekWorkouts is the number of workouts in the 7 days up to and including this one.
	WeekWorkouts int

	Records   []Record
	Changes   []ExerciseChange
	FirstTime []string
}

// WorkoutReport sums up a completed workout. It is separate from EndWorkout so that a failure here
// cannot undo the workout that is already saved.
func (s *Service) WorkoutReport(ctx context.Context, userID int64, d domain.WorkoutDetail) (WorkoutReport, error) {
	r := WorkoutReport{Workout: d}
	if fin := d.Workout.FinishedAt; !fin.IsZero() {
		if dur := fin.Sub(d.Workout.Date); dur >= minWorkoutDuration && dur <= maxWorkoutDuration {
			r.Duration = dur
		}
	}
	seen := map[string]bool{}
	for _, ex := range d.Exercises {
		r.Sets += len(ex.Sets)
		r.Volume += domain.VolumeFromSets(ex.Sets)
		for _, set := range ex.Sets {
			r.Reps += set.Reps
		}
		if !seen[ex.MuscleGroup] {
			seen[ex.MuscleGroup] = true
			r.Groups = append(r.Groups, ex.MuscleGroup)
		}
	}
	if err := s.reportCompared(ctx, userID, d, &r); err != nil {
		return WorkoutReport{}, err
	}
	if err := s.reportFrequency(ctx, userID, d, &r); err != nil {
		return WorkoutReport{}, err
	}
	return r, nil
}

// reportCompared fills what each exercise did against its own history.
func (s *Service) reportCompared(ctx context.Context, userID int64, d domain.WorkoutDetail, r *WorkoutReport) error {
	for _, ex := range d.Exercises {
		history, err := s.analytics.ListExerciseProgress(ctx, userID, ex.ExerciseID, time.Time{})
		if err != nil {
			return err
		}
		var before []domain.ExerciseProgressPoint
		for _, p := range history {
			if p.WorkoutID != d.Workout.ID {
				before = append(before, p)
			}
		}
		if len(before) == 0 {
			r.FirstTime = append(r.FirstTime, ex.ExerciseName)
			continue
		}
		now := domain.NewProgressPoint(d.Workout.ID, d.Workout.Date, ex.Sets)
		var same []domain.ExerciseProgressPoint
		for _, p := range before {
			if p.Kind == now.Kind {
				same = append(same, p)
			}
		}
		if len(same) == 0 {
			continue
		}
		if rec, ok := bestRecord(ex.ExerciseName, now, same); ok {
			r.Records = append(r.Records, rec)
		}
		r.Changes = append(r.Changes, ExerciseChange{Exercise: ex.ExerciseName, Load: now.Kind, Before: same[len(same)-1], Now: now})
	}
	return nil
}

// bestRecord names the most telling record the exercise set, if any: weight first, then the
// volume of the exercise, which also catches more reps at the same weight.
func bestRecord(name string, now domain.ExerciseProgressPoint, before []domain.ExerciseProgressPoint) (Record, bool) {
	best := func(value func(domain.ExerciseProgressPoint) float64) float64 {
		var m float64
		for _, p := range before {
			m = math.Max(m, value(p))
		}
		return m
	}
	candidates := []struct {
		kind  RecordKind
		value func(domain.ExerciseProgressPoint) float64
	}{
		{RecordWeight, func(p domain.ExerciseProgressPoint) float64 { return p.MaxWeight }},
		{RecordVolume, func(p domain.ExerciseProgressPoint) float64 { return p.Volume }},
		{RecordReps, func(p domain.ExerciseProgressPoint) float64 { return float64(p.MaxReps) }},
		{RecordTotalReps, func(p domain.ExerciseProgressPoint) float64 { return float64(p.TotalReps) }},
	}
	for _, c := range candidates {
		prev := best(c.value)
		// Nothing to beat when there was no weight (or volume) before: that is not a record.
		if prev > 0 && c.value(now) > prev {
			return Record{Exercise: name, Kind: c.kind, Value: c.value(now), Previous: prev}, true
		}
	}
	return Record{}, false
}

// reportFrequency fills how often the user trains and how this workout compares to the previous one.
func (s *Service) reportFrequency(ctx context.Context, userID int64, d domain.WorkoutDetail, r *WorkoutReport) error {
	week, err := s.analytics.ListCompletedWorkoutsInRange(ctx, userID, d.Workout.Date.AddDate(0, 0, -weekDays), d.Workout.Date.Add(time.Second))
	if err != nil {
		return err
	}
	r.WeekWorkouts = len(week)

	recent, err := s.analytics.ListWorkoutSummaries(ctx, userID, 2, 0)
	if err != nil {
		return err
	}
	for _, item := range recent {
		if item.Workout.ID == d.Workout.ID {
			continue
		}
		prev, err := s.analytics.GetWorkoutDetail(ctx, userID, item.Workout.ID)
		if err != nil {
			return err
		}
		r.PrevDate = prev.Workout.Date
		r.DaysSincePrev = daysBetween(prev.Workout.Date, d.Workout.Date)
		for _, ex := range prev.Exercises {
			r.PrevVolume += domain.VolumeFromSets(ex.Sets)
		}
		break
	}
	return nil
}

// daysBetween counts calendar days in the local time zone.
func daysBetween(from, to time.Time) int {
	day := func(t time.Time) time.Time {
		t = t.Local()
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	return int(math.Round(day(to).Sub(day(from)).Hours() / 24))
}
