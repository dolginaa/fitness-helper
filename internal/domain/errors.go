package domain

import "errors"

var (
	ErrNotFound           = errors.New("not found")
	ErrInvalidState       = errors.New("invalid session state")
	ErrNoActiveWorkout    = errors.New("no active workout")
	ErrUnauthorized       = errors.New("forbidden for this user")
	ErrNoLastExerciseData = errors.New("no previous data for exercise")
)
