package domain

import (
	"strings"
	"time"
	"unicode"
)

type MuscleGroup struct {
	ID   int
	Name string
}

// LoadKind is what makes a set hard: a weight on the machine or the bar, the body itself,
// or the resistance of an elastic band.
type LoadKind string

const (
	LoadWeight     LoadKind = "weight"
	LoadBodyweight LoadKind = "bodyweight"
	LoadBand       LoadKind = "band"
)

// AllLoads is the order in which kinds are offered.
var AllLoads = []LoadKind{LoadWeight, LoadBodyweight, LoadBand}

// MaxBand is the thickest band: levels go from 1 to MaxBand.
const MaxBand = 10

// ParseLoadKind accepts a kind as stored and sent over the API.
func ParseLoadKind(s string) (LoadKind, bool) {
	for _, k := range AllLoads {
		if string(k) == s {
			return k, true
		}
	}
	return "", false
}

type Exercise struct {
	ID            int
	Name          string
	MuscleGroupID int
	// Loads are the kinds of load the exercise can be done with, the first is the default.
	// Empty means weight only.
	Loads []LoadKind
}

// Offered returns the kinds of load of the exercise; there is always at least one.
func (e Exercise) Offered() []LoadKind {
	if len(e.Loads) == 0 {
		return []LoadKind{LoadWeight}
	}
	return e.Loads
}

func (e Exercise) Allows(k LoadKind) bool {
	for _, l := range e.Offered() {
		if l == k {
			return true
		}
	}
	return false
}

// ExerciseKey is the name of an exercise reduced to what matters for telling exercises apart:
// case, spaces, punctuation and е/ё do not. "Жим  гантелей (45°)" and "жим гантелей 45" are one exercise.
func ExerciseKey(name string) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r == 'ё':
			sb.WriteRune('е')
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

type WorkoutStatus string

const (
	WorkoutActive    WorkoutStatus = "active"
	WorkoutCompleted WorkoutStatus = "completed"
)

type Workout struct {
	ID     int
	UserID int64
	Date   time.Time
	// FinishedAt is zero for workouts finished before the time was kept and for imported ones.
	FinishedAt time.Time
	Status     WorkoutStatus
}

// Upper bounds of a single set; anything above is treated as a typo.
const (
	MaxReps   = 1000
	MaxWeight = 1000
)

// SetEntry is one set. It is stored as JSON in the workout, so the zero Kind means weight:
// that is how every set was written before kinds existed.
type SetEntry struct {
	Reps int `json:"reps"`
	// Weight is the weight on the machine or the bar, or the extra weight on top of the body.
	Weight float64 `json:"weight"`
	// Band is the band level, only for LoadBand.
	Band int      `json:"band,omitempty"`
	Kind LoadKind `json:"kind,omitempty"`
}

func (s SetEntry) Load() LoadKind {
	if s.Kind == "" {
		return LoadWeight
	}
	return s.Kind
}

// Normalize writes the weight kind the way old sets are written, so equal sets compare equal.
func (s SetEntry) Normalize() SetEntry {
	if s.Kind == LoadWeight {
		s.Kind = ""
	}
	return s
}

func (s SetEntry) Valid() bool {
	if s.Reps < 1 || s.Reps > MaxReps {
		return false
	}
	switch s.Load() {
	case LoadWeight, LoadBodyweight:
		return s.Band == 0 && s.Weight >= 0 && s.Weight <= MaxWeight
	case LoadBand:
		return s.Weight == 0 && s.Band >= 1 && s.Band <= MaxBand
	}
	return false
}

// Lifted is the weight that counts as lifted in this set: the extra weight of a bodyweight set
// and nothing for a band.
func (s SetEntry) Lifted() float64 {
	if s.Load() == LoadBand {
		return 0
	}
	return s.Weight
}

type WorkoutExercise struct {
	ID         int
	WorkoutID  int
	ExerciseID int
	Sets       []SetEntry
}
