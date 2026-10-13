-- name: ListCompletedWorkoutsInRange :many
SELECT id, user_id, date, status, finished_at
FROM workouts
WHERE user_id = $1 AND status = 'completed' AND date >= $2 AND date < $3
ORDER BY date DESC;

-- name: ListWorkoutSummaries :many
SELECT
    w.id,
    w.user_id,
    w.date,
    w.status,
    w.finished_at,
    COUNT(we.id)::int AS exercise_count,
    COALESCE(array_agg(DISTINCT mg.name) FILTER (WHERE mg.name IS NOT NULL), '{}')::text[] AS muscle_groups
FROM workouts w
JOIN workout_exercises we ON we.workout_id = w.id
JOIN exercises e ON e.id = we.exercise_id
JOIN muscle_groups mg ON mg.id = e.muscle_group_id
WHERE w.user_id = $1 AND w.status = 'completed'
GROUP BY w.id
ORDER BY w.date DESC, w.id DESC
LIMIT $2 OFFSET $3;

-- name: GetWorkoutDetailRows :many
SELECT
    w.id AS workout_id,
    w.user_id,
    w.date,
    w.status,
    w.finished_at,
    we.exercise_id,
    e.name AS exercise_name,
    mg.name AS muscle_group_name,
    we.sets
FROM workouts w
LEFT JOIN workout_exercises we ON we.workout_id = w.id
LEFT JOIN exercises e ON e.id = we.exercise_id
LEFT JOIN muscle_groups mg ON mg.id = e.muscle_group_id
WHERE w.id = $1 AND w.user_id = $2
ORDER BY we.id;

-- name: ListExerciseProgress :many
SELECT w.id AS workout_id, w.date, we.sets
FROM workout_exercises we
JOIN workouts w ON w.id = we.workout_id
WHERE w.user_id = $1 AND w.status = 'completed' AND we.exercise_id = $2 AND w.date >= $3
ORDER BY w.date ASC;

-- name: ListMuscleGroupProgress :many
SELECT w.id AS workout_id, w.date, e.id AS exercise_id, e.name AS exercise_name, we.sets
FROM workout_exercises we
JOIN workouts w ON w.id = we.workout_id
JOIN exercises e ON e.id = we.exercise_id
WHERE w.user_id = $1 AND w.status = 'completed' AND e.muscle_group_id = $2 AND w.date >= $3
ORDER BY w.date ASC, e.id;

-- name: ExportUserWorkoutRows :many
SELECT
    w.id AS workout_id,
    w.date,
    w.status,
    we.exercise_id,
    e.name AS exercise_name,
    mg.name AS muscle_group_name,
    we.sets
FROM workouts w
LEFT JOIN workout_exercises we ON we.workout_id = w.id
LEFT JOIN exercises e ON e.id = we.exercise_id
LEFT JOIN muscle_groups mg ON mg.id = e.muscle_group_id
WHERE w.user_id = $1 AND w.status = 'completed'
ORDER BY w.date ASC, e.id;

-- name: WorkoutExerciseExistsOnDate :one
SELECT EXISTS (
    SELECT 1
    FROM workout_exercises we
    JOIN workouts w ON w.id = we.workout_id
    WHERE w.user_id = $1
      AND we.exercise_id = $2
      AND (w.date AT TIME ZONE 'UTC')::date = $3::date
      AND w.status = 'completed'
) AS exists;

-- name: InsertImportedWorkout :one
INSERT INTO workouts (user_id, date, status)
VALUES ($1, $2, 'completed')
RETURNING id, user_id, date, status;

-- name: DeleteUserWorkouts :exec
DELETE FROM workouts WHERE user_id = $1;
