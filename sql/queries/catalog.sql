-- name: ListMuscleGroups :many
SELECT id, name FROM muscle_groups ORDER BY id;

-- name: ListExercisesByMuscleGroup :many
SELECT id, name, muscle_group_id FROM exercises WHERE muscle_group_id = $1 ORDER BY id;

-- name: GetExerciseByID :one
SELECT id, name, muscle_group_id FROM exercises WHERE id = $1;

-- name: GetMuscleGroupByID :one
SELECT id, name FROM muscle_groups WHERE id = $1;

-- name: GetExerciseByNameAndGroup :one
SELECT e.id, e.name, e.muscle_group_id
FROM exercises e
JOIN muscle_groups mg ON mg.id = e.muscle_group_id
WHERE e.name = $1 AND mg.name = $2;

-- name: GetMuscleGroupByName :one
SELECT id, name FROM muscle_groups WHERE name = $1;

-- name: InsertMuscleGroup :one
INSERT INTO muscle_groups (name) VALUES ($1) RETURNING id, name;

-- name: InsertExercise :one
INSERT INTO exercises (name, muscle_group_id) VALUES ($1, $2) RETURNING id, name, muscle_group_id;
