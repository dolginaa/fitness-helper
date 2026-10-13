package domain

import "time"

type WorkoutDetail struct {
	Workout   Workout
	Exercises []WorkoutExerciseDetail
}

// SetsOf returns the sets logged for the exercise in this workout (nil if none).
func (d WorkoutDetail) SetsOf(exerciseID int) []SetEntry {
	for _, ex := range d.Exercises {
		if ex.ExerciseID == exerciseID {
			return ex.Sets
		}
	}
	return nil
}

type WorkoutExerciseDetail struct {
	ExerciseID   int
	ExerciseName string
	MuscleGroup  string
	Sets         []SetEntry
}

// ExerciseUsage is an exercise of the catalog with how often it was done.
type ExerciseUsage struct {
	Exercise Exercise
	Group    string
	Workouts int
}

// WorkoutSummary is a history list item: a workout and what was trained in it.
type WorkoutSummary struct {
	Workout       Workout
	MuscleGroups  []string
	ExerciseCount int
}

// ExerciseProgressPoint is how one exercise went in one workout.
type ExerciseProgressPoint struct {
	WorkoutID int
	Date      time.Time
	// Kind is the load kind most of the sets were done with.
	Kind          LoadKind
	MaxWeight     float64
	WorkingWeight float64
	// Band is the band level of the last band set, zero if there were none.
	Band      int
	MaxReps   int
	TotalReps int
	Volume    float64
	Sets      []SetEntry
}

// NewProgressPoint sums up the sets of an exercise in a workout.
func NewProgressPoint(workoutID int, date time.Time, sets []SetEntry) ExerciseProgressPoint {
	p := ExerciseProgressPoint{
		WorkoutID:     workoutID,
		Date:          date,
		Kind:          DominantLoad(sets),
		MaxWeight:     MaxWeightFromSets(sets),
		WorkingWeight: WorkingWeightFromSets(sets),
		Volume:        VolumeFromSets(sets),
		Sets:          sets,
	}
	for _, s := range sets {
		p.TotalReps += s.Reps
		p.MaxReps = max(p.MaxReps, s.Reps)
		if s.Load() == LoadBand {
			p.Band = s.Band
		}
	}
	return p
}

// DominantLoad is the kind of load most of the sets were done with; on a tie the earlier one wins.
func DominantLoad(sets []SetEntry) LoadKind {
	count := map[LoadKind]int{}
	best := LoadWeight
	for i, s := range sets {
		count[s.Load()]++
		if i == 0 || count[s.Load()] > count[best] {
			best = s.Load()
		}
	}
	return best
}

// ChartSeries picks the points that go on one chart. Weights, bodyweight reps and band levels
// cannot share an axis, so a chart shows one kind: that of the latest workout, or, if there are
// too few points of it, the kind with the most points.
func ChartSeries(points []ExerciseProgressPoint) (LoadKind, []ExerciseProgressPoint) {
	if len(points) == 0 {
		return LoadWeight, nil
	}
	of := func(k LoadKind) []ExerciseProgressPoint {
		var out []ExerciseProgressPoint
		for _, p := range points {
			if p.Kind == k {
				out = append(out, p)
			}
		}
		return out
	}
	kind := points[len(points)-1].Kind
	best := of(kind)
	if len(best) >= 2 {
		return kind, best
	}
	for _, k := range AllLoads {
		if s := of(k); len(s) > len(best) {
			kind, best = k, s
		}
	}
	return kind, best
}

type ExerciseProgressSeries struct {
	ExerciseID   int
	ExerciseName string
	Points       []ExerciseProgressPoint
}

// counted reports whether the set says something about the weight: band sets and
// bodyweight sets without extra weight do not.
func counted(s SetEntry) bool {
	return s.Load() == LoadWeight || (s.Load() == LoadBodyweight && s.Weight > 0)
}

// MaxWeightFromSets returns the maximum weight across sets (0 if empty).
func MaxWeightFromSets(sets []SetEntry) float64 {
	var max float64
	for _, s := range sets {
		if counted(s) && s.Weight > max {
			max = s.Weight
		}
	}
	return max
}

// WorkingWeightFromSets returns sum(weight*reps)/sum(reps) (0 if no reps).
func WorkingWeightFromSets(sets []SetEntry) float64 {
	var weighted float64
	var reps int
	for _, s := range sets {
		if !counted(s) {
			continue
		}
		weighted += s.Weight * float64(s.Reps)
		reps += s.Reps
	}
	if reps == 0 {
		return 0
	}
	return weighted / float64(reps)
}

// VolumeFromSets returns the lifted volume: sum(weight*reps).
func VolumeFromSets(sets []SetEntry) float64 {
	var volume float64
	for _, s := range sets {
		volume += s.Lifted() * float64(s.Reps)
	}
	return volume
}
