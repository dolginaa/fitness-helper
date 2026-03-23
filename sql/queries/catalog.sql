-- name: ListMuscleGroups :many
SELECT id, name FROM muscle_groups ORDER BY id;

-- name: ListExercisesByMuscleGroup :many
SELECT id, name, muscle_group_id FROM exercises WHERE muscle_group_id = $1 ORDER BY id;

-- name: GetExerciseByID :one
SELECT id, name, muscle_group_id FROM exercises WHERE id = $1;

-- name: GetExerciseByIDAndGroup :one
SELECT id, name, muscle_group_id FROM exercises WHERE id = $1 AND muscle_group_id = $2;
