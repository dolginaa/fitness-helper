package usecase

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/adolgina/fitness-helper/internal/domain"
)

func (s *Service) ExportUserData(ctx context.Context, userID int64) ([]byte, string, error) {
	workouts, err := s.backup.ExportRows(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	bundle := domain.ExportBundle{
		Version:    domain.ExportVersion,
		ExportedAt: time.Now().UTC(),
		Workouts:   workouts,
	}
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, "", err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("export.json")
	if err != nil {
		return nil, "", err
	}
	if _, err := w.Write(raw); err != nil {
		return nil, "", err
	}
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	name := fmt.Sprintf("fitness-backup-%d.zip", userID)
	return buf.Bytes(), name, nil
}

func (s *Service) ImportUserData(ctx context.Context, userID int64, zipBytes []byte, mode domain.ImportMode) (domain.ImportResult, error) {
	bundle, err := parseExportZIP(zipBytes)
	if err != nil {
		return domain.ImportResult{}, err
	}
	res := domain.ImportResult{}
	if mode == domain.ImportModeReplace {
		if err := s.backup.DeleteAllUserWorkouts(ctx, userID); err != nil {
			return res, err
		}
	}
	for _, wo := range bundle.Workouts {
		exAdded, skipped, err := s.importOneWorkout(ctx, userID, wo, mode)
		if err != nil {
			return res, err
		}
		if skipped {
			res.WorkoutsSkipped++
			continue
		}
		res.WorkoutsAdded++
		res.ExercisesAdded += exAdded
	}
	return res, nil
}

func parseExportZIP(data []byte) (domain.ExportBundle, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return domain.ExportBundle{}, err
	}
	var raw []byte
	for _, f := range zr.File {
		if f.Name == "export.json" {
			rc, err := f.Open()
			if err != nil {
				return domain.ExportBundle{}, err
			}
			raw, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return domain.ExportBundle{}, err
			}
			break
		}
	}
	if len(raw) == 0 {
		return domain.ExportBundle{}, fmt.Errorf("export.json not found in archive")
	}
	var bundle domain.ExportBundle
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return domain.ExportBundle{}, err
	}
	if bundle.Version != domain.ExportVersion {
		return domain.ExportBundle{}, fmt.Errorf("unsupported export version %d", bundle.Version)
	}
	return bundle, nil
}

func (s *Service) importOneWorkout(ctx context.Context, userID int64, wo domain.ExportWorkout, mode domain.ImportMode) (exercisesAdded int, skipped bool, err error) {
	if len(wo.Exercises) == 0 {
		return 0, true, nil
	}
	if mode == domain.ImportModeMerge {
		allExist := true
		for _, ex := range wo.Exercises {
			if len(ex.Sets) == 0 {
				continue
			}
			e, err := s.catalog.GetOrCreateExerciseByNameAndGroup(ctx, ex.ExerciseName, ex.MuscleGroup)
			if err != nil {
				return 0, false, fmt.Errorf("упражнение %q (%s): %w", ex.ExerciseName, ex.MuscleGroup, err)
			}
			ok, err := s.backup.WorkoutExerciseExistsOnDate(ctx, userID, e.ID, wo.Date)
			if err != nil {
				return 0, false, err
			}
			if !ok {
				allExist = false
				break
			}
		}
		if allExist {
			return 0, true, nil
		}
	}
	wid, err := s.backup.CreateImportedWorkout(ctx, userID, wo.Date)
	if err != nil {
		return 0, false, err
	}
	added := 0
	for _, ex := range wo.Exercises {
		if len(ex.Sets) == 0 {
			continue
		}
		e, err := s.catalog.GetOrCreateExerciseByNameAndGroup(ctx, ex.ExerciseName, ex.MuscleGroup)
		if err != nil {
			return added, false, fmt.Errorf("упражнение %q (%s): %w", ex.ExerciseName, ex.MuscleGroup, err)
		}
		if mode == domain.ImportModeMerge {
			ok, err := s.backup.WorkoutExerciseExistsOnDate(ctx, userID, e.ID, wo.Date)
			if err != nil {
				return added, false, err
			}
			if ok {
				continue
			}
		}
		if err := s.ensureLoads(ctx, e, ex.Sets); err != nil {
			return added, false, err
		}
		if err := s.backup.AddExerciseToWorkout(ctx, wid, e.ID, ex.Sets); err != nil {
			return added, false, err
		}
		added++
	}
	if added == 0 {
		return 0, true, nil
	}
	return added, false, nil
}

func (s *Service) SetWeeklyBackup(ctx context.Context, userID int64, enabled bool) error {
	return s.settings.SetWeeklyBackup(ctx, userID, enabled)
}

func (s *Service) BackupSettings(ctx context.Context, userID int64) (domain.UserSettings, error) {
	return s.settings.GetSettings(ctx, userID)
}

func (s *Service) ListUsersDueWeeklyBackup(ctx context.Context) ([]int64, error) {
	return s.settings.ListUsersDueWeeklyBackup(ctx, time.Now().Add(-7*24*time.Hour))
}

func (s *Service) MarkBackupSent(ctx context.Context, userID int64) error {
	return s.settings.SetLastBackupAt(ctx, userID, time.Now())
}

// ensureLoads lets the exercise be done with every kind of load its imported sets were done with.
func (s *Service) ensureLoads(ctx context.Context, e domain.Exercise, sets []domain.SetEntry) error {
	loads := slices.Clone(e.Offered())
	for _, set := range sets {
		if k := set.Load(); !slices.Contains(loads, k) && slices.Contains(domain.AllLoads, k) {
			loads = append(loads, k)
		}
	}
	if len(loads) == len(e.Offered()) {
		return nil
	}
	return s.catalog.SetExerciseLoads(ctx, e.ID, loads)
}
