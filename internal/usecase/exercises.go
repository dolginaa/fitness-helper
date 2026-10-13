package usecase

import (
	"context"
	"fmt"
	"sort"
	"unicode"
	"unicode/utf8"

	"github.com/adolgina/fitness-helper/internal/domain"
)

const (
	// similarMaxDistance and similarMinLen decide which differently written names are shown
	// as possible duplicates: up to two edits in names of at least eight letters.
	similarMaxDistance = 2
	similarMinLen      = 8
)

// MergePlan is a group of exercises that differ only in how the name is written.
type MergePlan struct {
	Group string
	// Keep is the exercise that stays; the records of Merge move to it.
	Keep  domain.ExerciseUsage
	Merge []domain.ExerciseUsage
}

// SimilarPair is two exercises whose names are close but not the same. Only a person can tell
// whether they are one exercise, so these are shown and never merged by themselves.
type SimilarPair struct {
	Group string
	A, B  domain.ExerciseUsage
}

// CatalogReport is what is untidy in the exercise list.
type CatalogReport struct {
	Merges  []MergePlan
	Similar []SimilarPair
	// Unused are exercises that are in no workout.
	Unused []domain.ExerciseUsage
}

// ExerciseCatalogReport finds duplicates by the written form of the name, near-duplicates and unused exercises.
func (s *Service) ExerciseCatalogReport(ctx context.Context) (CatalogReport, error) {
	usage, err := s.catalog.ListExerciseUsage(ctx)
	if err != nil {
		return CatalogReport{}, err
	}
	var report CatalogReport
	byGroup := map[int][]domain.ExerciseUsage{}
	var groupOrder []int
	for _, u := range usage {
		g := u.Exercise.MuscleGroupID
		if _, ok := byGroup[g]; !ok {
			groupOrder = append(groupOrder, g)
		}
		byGroup[g] = append(byGroup[g], u)
		if u.Workouts == 0 {
			report.Unused = append(report.Unused, u)
		}
	}
	for _, g := range groupOrder {
		report.Merges = append(report.Merges, mergePlans(byGroup[g])...)
		report.Similar = append(report.Similar, similarPairs(byGroup[g])...)
	}
	return report, nil
}

func mergePlans(group []domain.ExerciseUsage) []MergePlan {
	byKey := map[string][]domain.ExerciseUsage{}
	var keys []string
	for _, u := range group {
		k := domain.ExerciseKey(u.Exercise.Name)
		if _, ok := byKey[k]; !ok {
			keys = append(keys, k)
		}
		byKey[k] = append(byKey[k], u)
	}
	var plans []MergePlan
	for _, k := range keys {
		same := byKey[k]
		if len(same) < 2 {
			continue
		}
		keep := 0
		for i := range same {
			if better(same[i], same[keep]) {
				keep = i
			}
		}
		plan := MergePlan{Group: same[keep].Group, Keep: same[keep]}
		for i, u := range same {
			if i != keep {
				plan.Merge = append(plan.Merge, u)
			}
		}
		plans = append(plans, plan)
	}
	return plans
}

// better says which of two spellings of an exercise to keep: the one that starts with a capital
// letter like the rest of the list, then the one done more often, then the older one.
func better(a, b domain.ExerciseUsage) bool {
	if ca, cb := capitalized(a.Exercise.Name), capitalized(b.Exercise.Name); ca != cb {
		return ca
	}
	if a.Workouts != b.Workouts {
		return a.Workouts > b.Workouts
	}
	return a.Exercise.ID < b.Exercise.ID
}

func capitalized(name string) bool {
	r, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(r)
}

func similarPairs(group []domain.ExerciseUsage) []SimilarPair {
	var pairs []SimilarPair
	for i, a := range group {
		for _, b := range group[i+1:] {
			ka, kb := []rune(domain.ExerciseKey(a.Exercise.Name)), []rune(domain.ExerciseKey(b.Exercise.Name))
			if string(ka) == string(kb) || len(ka) < similarMinLen || len(kb) < similarMinLen {
				continue
			}
			if levenshtein(ka, kb) <= similarMaxDistance {
				pairs = append(pairs, SimilarPair{Group: a.Group, A: a, B: b})
			}
		}
	}
	return pairs
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// ApplyMerges merges every planned duplicate into the exercise that is kept.
// Each merge is atomic; a failure stops the rest and says which one failed.
func (s *Service) ApplyMerges(ctx context.Context, plans []MergePlan) error {
	for _, p := range plans {
		for _, m := range p.Merge {
			if err := s.catalog.MergeExercises(ctx, m.Exercise.ID, p.Keep.Exercise.ID); err != nil {
				return fmt.Errorf("merge %q (id %d) into %q (id %d): %w", m.Exercise.Name, m.Exercise.ID, p.Keep.Exercise.Name, p.Keep.Exercise.ID, err)
			}
		}
	}
	return nil
}

// MergeExercises moves the records of one exercise to another and removes the first.
func (s *Service) MergeExercises(ctx context.Context, from, to int) error {
	return s.catalog.MergeExercises(ctx, from, to)
}

// DeleteUnusedExercise removes an exercise that is in no workout.
func (s *Service) DeleteUnusedExercise(ctx context.Context, id int) error {
	return s.catalog.DeleteUnusedExercise(ctx, id)
}

// SortUsage orders exercises by group and name for printing.
func SortUsage(list []domain.ExerciseUsage) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Group != list[j].Group {
			return list[i].Group < list[j].Group
		}
		return list[i].Exercise.Name < list[j].Exercise.Name
	})
}
