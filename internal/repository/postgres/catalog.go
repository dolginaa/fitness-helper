package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Catalog struct {
	pool *pgxpool.Pool
	q    *sqlcgen.Queries
}

func NewCatalog(pool *pgxpool.Pool, q *sqlcgen.Queries) *Catalog {
	return &Catalog{pool: pool, q: q}
}

func (c *Catalog) ListMuscleGroups(ctx context.Context) ([]domain.MuscleGroup, error) {
	rows, err := c.q.ListMuscleGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.MuscleGroup, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.MuscleGroup{ID: int(r.ID), Name: r.Name})
	}
	return out, nil
}

func (c *Catalog) GetMuscleGroup(ctx context.Context, id int) (domain.MuscleGroup, error) {
	r, err := c.q.GetMuscleGroupByID(ctx, int32(id))
	if err != nil {
		return domain.MuscleGroup{}, mapErr(err)
	}
	return domain.MuscleGroup{ID: int(r.ID), Name: r.Name}, nil
}

func (c *Catalog) ListExercisesByGroup(ctx context.Context, muscleGroupID int) ([]domain.Exercise, error) {
	rows, err := c.q.ListExercisesByMuscleGroup(ctx, int32(muscleGroupID))
	if err != nil {
		return nil, err
	}
	out := make([]domain.Exercise, 0, len(rows))
	for _, r := range rows {
		out = append(out, mapExercise(r))
	}
	return out, nil
}

func (c *Catalog) GetExercise(ctx context.Context, id int) (domain.Exercise, error) {
	r, err := c.q.GetExerciseByID(ctx, int32(id))
	if err != nil {
		return domain.Exercise{}, mapErr(err)
	}
	return mapExercise(r), nil
}

func (c *Catalog) CreateExercise(ctx context.Context, name string, muscleGroupID int, loads []domain.LoadKind) (domain.Exercise, error) {
	r, err := c.q.InsertExercise(ctx, sqlcgen.InsertExerciseParams{
		Name:          name,
		MuscleGroupID: int32(muscleGroupID),
		Loads:         loadStrings(loads),
	})
	if err != nil {
		return domain.Exercise{}, err
	}
	return mapExercise(r), nil
}

func (c *Catalog) SetExerciseLoads(ctx context.Context, id int, loads []domain.LoadKind) error {
	return c.q.UpdateExerciseLoads(ctx, sqlcgen.UpdateExerciseLoadsParams{ID: int32(id), Loads: loadStrings(loads)})
}

// GetOrCreateExerciseByNameAndGroup finds the group and the exercise by the key of their names,
// so "подтягивания" and "Подтягивания" are one exercise.
func (c *Catalog) GetOrCreateExerciseByNameAndGroup(ctx context.Context, name, muscleGroup string) (domain.Exercise, error) {
	groups, err := c.ListMuscleGroups(ctx)
	if err != nil {
		return domain.Exercise{}, err
	}
	var group domain.MuscleGroup
	for _, g := range groups {
		if domain.ExerciseKey(g.Name) == domain.ExerciseKey(muscleGroup) {
			group = g
			break
		}
	}
	if group.ID == 0 {
		row, err := c.q.InsertMuscleGroup(ctx, muscleGroup)
		if err != nil {
			return domain.Exercise{}, err
		}
		group = domain.MuscleGroup{ID: int(row.ID), Name: row.Name}
	}
	exercises, err := c.ListExercisesByGroup(ctx, group.ID)
	if err != nil {
		return domain.Exercise{}, err
	}
	for _, ex := range exercises {
		if domain.ExerciseKey(ex.Name) == domain.ExerciseKey(name) {
			return ex, nil
		}
	}
	return c.CreateExercise(ctx, name, group.ID, nil)
}

func (c *Catalog) ListExerciseUsage(ctx context.Context) ([]domain.ExerciseUsage, error) {
	rows, err := c.q.ListExerciseUsage(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ExerciseUsage, 0, len(rows))
	for _, r := range rows {
		out = append(out, domain.ExerciseUsage{
			Exercise: mapExercise(sqlcgen.Exercise{ID: r.ID, Name: r.Name, MuscleGroupID: r.MuscleGroupID, Loads: r.Loads}),
			Group:    r.GroupName,
			Workouts: int(r.Workouts),
		})
	}
	return out, nil
}

// MergeExercises moves every record of "from" to "to" in one transaction. The number of sets
// of both exercises is counted before and after; if it differs, nothing is applied.
func (c *Catalog) MergeExercises(ctx context.Context, from, to int) error {
	if from == to {
		return fmt.Errorf("%w: an exercise cannot be merged into itself", domain.ErrInvalidInput)
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := c.q.WithTx(tx)

	if _, err := q.GetExerciseByID(ctx, int32(to)); err != nil {
		return mapErr(err)
	}
	src, err := q.GetExerciseByID(ctx, int32(from))
	if err != nil {
		return mapErr(err)
	}
	moved, err := q.ListWorkoutExercisesOfExercise(ctx, int32(from))
	if err != nil {
		return err
	}
	before, err := countSetsOf(ctx, q, from, to)
	if err != nil {
		return err
	}
	for _, row := range moved {
		if err := moveRecord(ctx, q, row, int32(to)); err != nil {
			return err
		}
	}
	if err := mergeLoads(ctx, q, src, int32(to)); err != nil {
		return err
	}
	if err := q.DeleteExercise(ctx, int32(from)); err != nil {
		return err
	}
	after, err := countSetsOf(ctx, q, to)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("merge exercises: %d sets before and %d after, nothing was changed", before, after)
	}
	return tx.Commit(ctx)
}

// moveRecord gives the record of a workout to another exercise; if the workout already has
// that exercise, the sets are joined into its record.
func moveRecord(ctx context.Context, q *sqlcgen.Queries, row sqlcgen.WorkoutExercise, to int32) error {
	target, err := q.GetWorkoutExerciseOfWorkout(ctx, sqlcgen.GetWorkoutExerciseOfWorkoutParams{
		WorkoutID: row.WorkoutID, ExerciseID: to,
	})
	if errors.Is(mapErr(err), domain.ErrNotFound) {
		return q.MoveWorkoutExercise(ctx, sqlcgen.MoveWorkoutExerciseParams{ID: row.ID, ExerciseID: to})
	}
	if err != nil {
		return err
	}
	joined, err := joinSets(target.Sets, row.Sets)
	if err != nil {
		return err
	}
	if err := q.UpdateWorkoutExerciseSets(ctx, sqlcgen.UpdateWorkoutExerciseSetsParams{ID: target.ID, Sets: joined}); err != nil {
		return err
	}
	return q.DeleteWorkoutExerciseByID(ctx, row.ID)
}

// countSetsOf counts the sets logged for the exercises.
func countSetsOf(ctx context.Context, q *sqlcgen.Queries, exerciseIDs ...int) (int, error) {
	total := 0
	for _, id := range exerciseIDs {
		rows, err := q.ListWorkoutExercisesOfExercise(ctx, int32(id))
		if err != nil {
			return 0, err
		}
		for _, row := range rows {
			n, err := countSets(row.Sets)
			if err != nil {
				return 0, err
			}
			total += n
		}
	}
	return total, nil
}

func (c *Catalog) DeleteUnusedExercise(ctx context.Context, id int) error {
	n, err := c.q.CountWorkoutExercisesOfExercise(ctx, int32(id))
	if err != nil {
		return err
	}
	if n != 0 {
		return fmt.Errorf("%w: it is in %d workouts", domain.ErrInUse, n)
	}
	return c.q.DeleteExercise(ctx, int32(id))
}

func mapExercise(e sqlcgen.Exercise) domain.Exercise {
	loads := make([]domain.LoadKind, 0, len(e.Loads))
	for _, l := range e.Loads {
		if k, ok := domain.ParseLoadKind(l); ok {
			loads = append(loads, k)
		}
	}
	return domain.Exercise{
		ID:            int(e.ID),
		Name:          e.Name,
		MuscleGroupID: int(e.MuscleGroupID),
		Loads:         loads,
	}
}

func loadStrings(loads []domain.LoadKind) []string {
	if len(loads) == 0 {
		return []string{string(domain.LoadWeight)}
	}
	out := make([]string, 0, len(loads))
	for _, l := range loads {
		out = append(out, string(l))
	}
	return out
}

func countSets(raw []byte) (int, error) {
	var sets []json.RawMessage
	if len(raw) == 0 {
		return 0, nil
	}
	if err := json.Unmarshal(raw, &sets); err != nil {
		return 0, err
	}
	return len(sets), nil
}

// joinSets appends the sets of b to those of a without touching their content, so fields
// this version does not know about are kept.
func joinSets(a, b []byte) ([]byte, error) {
	var first, second []json.RawMessage
	if len(a) > 0 {
		if err := json.Unmarshal(a, &first); err != nil {
			return nil, err
		}
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &second); err != nil {
			return nil, err
		}
	}
	return json.Marshal(append(first, second...))
}

// mergeLoads lets the target exercise be done with every kind the source was done with.
func mergeLoads(ctx context.Context, q *sqlcgen.Queries, src sqlcgen.Exercise, to int32) error {
	target, err := q.GetExerciseByID(ctx, to)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, l := range target.Loads {
		have[l] = true
	}
	loads := target.Loads
	for _, l := range src.Loads {
		if !have[l] {
			loads = append(loads, l)
		}
	}
	if len(loads) == len(target.Loads) {
		return nil
	}
	return q.UpdateExerciseLoads(ctx, sqlcgen.UpdateExerciseLoadsParams{ID: to, Loads: loads})
}
