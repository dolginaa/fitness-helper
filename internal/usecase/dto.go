package usecase

import (
	"fmt"
	"strings"

	"github.com/adolgina/fitness-helper/internal/domain"
)

type ExerciseRow struct {
	Exercise    domain.Exercise
	LastSummary string
}

func FormatSets(sets []domain.SetEntry) string {
	if len(sets) == 0 {
		return "нет данных"
	}
	parts := make([]string, 0, len(sets))
	for _, s := range sets {
		parts = append(parts, fmt.Sprintf("%d×%.1f", s.Reps, s.Weight))
	}
	return strings.Join(parts, ", ")
}
