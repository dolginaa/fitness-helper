package domain

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrInvalidState       = errors.New("invalid session state")
	ErrInvalidInput       = errors.New("invalid input")
	ErrNoActiveWorkout    = errors.New("no active workout")
	ErrNoLastExerciseData = errors.New("no previous data for exercise")
	ErrNotEnoughData      = errors.New("not enough data")
	ErrInUse              = errors.New("exercise is in workouts")
)
