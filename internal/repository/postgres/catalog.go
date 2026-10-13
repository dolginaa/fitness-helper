package postgres

import (
	"context"
	"errors"

	"github.com/adolgina/fitness-helper/internal/domain"
	"github.com/adolgina/fitness-helper/internal/repository/sqlcgen"
)

type Catalog struct {
	q *sqlcgen.Queries
}

func NewCatalog(q *sqlcgen.Queries) *Catalog {
	return &Catalog{q: q}
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

func (c *Catalog) CreateExercise(ctx context.Context, name string, muscleGroupID int) (domain.Exercise, error) {
	r, err := c.q.InsertExercise(ctx, sqlcgen.InsertExerciseParams{
		Name:          name,
		MuscleGroupID: int32(muscleGroupID),
	})
	if err != nil {
		return domain.Exercise{}, err
	}
	return mapExercise(r), nil
}

func (c *Catalog) GetOrCreateExerciseByNameAndGroup(ctx context.Context, name, muscleGroup string) (domain.Exercise, error) {
	r, err := c.q.GetExerciseByNameAndGroup(ctx, sqlcgen.GetExerciseByNameAndGroupParams{
		Name:   name,
		Name_2: muscleGroup,
	})
	if err == nil {
		return mapExercise(r), nil
	}
	if err := mapErr(err); !errors.Is(err, domain.ErrNotFound) {
		return domain.Exercise{}, err
	}
	mg, err := c.q.GetMuscleGroupByName(ctx, muscleGroup)
	if err != nil {
		if err := mapErr(err); !errors.Is(err, domain.ErrNotFound) {
			return domain.Exercise{}, err
		}
		mg, err = c.q.InsertMuscleGroup(ctx, muscleGroup)
		if err != nil {
			return domain.Exercise{}, err
		}
	}
	return c.CreateExercise(ctx, name, int(mg.ID))
}

func mapExercise(e sqlcgen.Exercise) domain.Exercise {
	return domain.Exercise{
		ID:            int(e.ID),
		Name:          e.Name,
		MuscleGroupID: int(e.MuscleGroupID),
	}
}

var _ domain.CatalogRepository = (*Catalog)(nil)
