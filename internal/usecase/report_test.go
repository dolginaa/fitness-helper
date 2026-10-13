package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

func TestWorkoutReport(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	pull := f.pullups()
	squat := f.store.AddExercise(f.legs.ID, "Присед")
	start := time.Now().Add(-50 * time.Minute)

	f.workoutAt(t, start.AddDate(0, 0, -5), map[int][]domain.SetEntry{
		f.press.ID: sets(10, 90, 8, 95),
		f.calf.ID:  sets(20, 40),
		pull.ID:    {band(8, 4), band(7, 4)},
	})
	f.workoutAt(t, start.AddDate(0, 0, -2), map[int][]domain.SetEntry{f.press.ID: sets(10, 100), pull.ID: {band(10, 4)}})

	f.store.Now = func() time.Time { return start }
	f.begin(t, f.press)
	for _, text := range []string{"10 102,5", "8 105"} {
		if _, err := f.svc.LogInput(ctx, testUser, text); err != nil {
			t.Fatal(err)
		}
	}
	f.begin(t, f.calf)
	if _, err := f.svc.LogInput(ctx, testUser, "20 40"); err != nil {
		t.Fatal(err)
	}
	f.begin(t, pull)
	if _, err := f.svc.SetLoadKind(ctx, testUser, pull.ID, domain.LoadBand); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.LogInput(ctx, testUser, "12 4, 11 4"); err != nil {
		t.Fatal(err)
	}
	f.begin(t, squat)
	if _, err := f.svc.LogInput(ctx, testUser, "10 60"); err != nil {
		t.Fatal(err)
	}
	detail, err := f.svc.EndWorkout(ctx, testUser)
	if err != nil {
		t.Fatal(err)
	}

	r, err := f.svc.WorkoutReport(ctx, testUser, detail)
	if err != nil {
		t.Fatal(err)
	}
	if r.Duration < 49*time.Minute || r.Duration > 51*time.Minute {
		t.Fatalf("duration: %v", r.Duration)
	}
	if r.Sets != 6 || r.Reps != 10+8+20+12+11+10 || r.Volume != 10*102.5+8*105+20*40+10*60 {
		t.Fatalf("totals: %d sets, %d reps, volume %v", r.Sets, r.Reps, r.Volume)
	}
	if len(r.Groups) != 1 || r.Groups[0] != "ноги" {
		t.Fatalf("groups: %v", r.Groups)
	}

	records := map[string]Record{}
	for _, rec := range r.Records {
		records[rec.Exercise] = rec
	}
	if rec := records["Жим ногами"]; rec.Kind != RecordWeight || rec.Value != 105 || rec.Previous != 100 {
		t.Fatalf("a heavier set is a weight record: %+v", rec)
	}
	if rec := records["Подтягивания"]; rec.Kind != RecordReps || rec.Value != 12 || rec.Previous != 10 {
		t.Fatalf("more reps on a band is a reps record: %+v", rec)
	}
	if _, ok := records["Икры"]; ok {
		t.Fatalf("the same result as before is no record: %+v", records["Икры"])
	}
	if len(r.FirstTime) != 1 || r.FirstTime[0] != "Присед" {
		t.Fatalf("first time: %v", r.FirstTime)
	}
	for _, c := range r.Changes {
		if c.Exercise == "Присед" {
			t.Fatalf("an exercise without history has nothing to compare with: %+v", c)
		}
		if c.Exercise == "Жим ногами" && (c.Before.MaxWeight != 100 || c.Now.MaxWeight != 105) {
			t.Fatalf("compared with the last time, not the best: %+v", c)
		}
	}

	if r.WeekWorkouts != 3 {
		t.Fatalf("this one and two earlier in the last 7 days: %d", r.WeekWorkouts)
	}
	if r.DaysSincePrev != 2 || r.PrevVolume != 10*100 {
		t.Fatalf("previous workout: %d days, volume %v", r.DaysSincePrev, r.PrevVolume)
	}
}

func TestWorkoutReportFirstWorkoutAndOldOnes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	// A workout ended long after it started was left open: its duration is not believed.
	w := f.workoutAt(t, time.Now().Add(-30*time.Hour), map[int][]domain.SetEntry{f.press.ID: sets(10, 100)})
	if err := f.store.CompleteWorkout(ctx, testUser, w.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	d, err := f.svc.GetWorkout(ctx, testUser, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.svc.WorkoutReport(ctx, testUser, d)
	if err != nil {
		t.Fatal(err)
	}
	if r.Duration != 0 {
		t.Fatalf("an unbelievable duration must be dropped: %v", r.Duration)
	}
	if !r.PrevDate.IsZero() || len(r.Records) != 0 || len(r.FirstTime) != 1 || r.WeekWorkouts != 1 {
		t.Fatalf("a first workout has no previous one and no records: %+v", r)
	}
}
