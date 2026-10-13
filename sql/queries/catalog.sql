-- name: ListMuscleGroups :many
SELECT id, name FROM muscle_groups ORDER BY id;

-- name: ListExercisesByMuscleGroup :many
SELECT id, name, muscle_group_id, loads FROM exercises WHERE muscle_group_id = $1 ORDER BY id;

-- name: GetExerciseByID :one
SELECT id, name, muscle_group_id, loads FROM exercises WHERE id = $1;

-- name: GetMuscleGroupByID :one
SELECT id, name FROM muscle_groups WHERE id = $1;

-- name: InsertMuscleGroup :one
INSERT INTO muscle_groups (name) VALUES ($1) RETURNING id, name;

-- name: InsertExercise :one
INSERT INTO exercises (name, muscle_group_id, loads) VALUES ($1, $2, $3) RETURNING id, name, muscle_group_id, loads;

-- name: UpdateExerciseLoads :exec
UPDATE exercises SET loads = $2 WHERE id = $1;

-- name: ListExerciseUsage :many
SELECT e.id, e.name, e.muscle_group_id, e.loads, mg.name AS group_name, COUNT(we.id)::int AS workouts
FROM exercises e
JOIN muscle_groups mg ON mg.id = e.muscle_group_id
LEFT JOIN workout_exercises we ON we.exercise_id = e.id
GROUP BY e.id, mg.id
ORDER BY mg.id, e.id;

-- name: DeleteExercise :exec
DELETE FROM exercises WHERE id = $1;
