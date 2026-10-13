-- name: CreateWorkout :one
INSERT INTO workouts (user_id, status) VALUES ($1, 'active') RETURNING id, user_id, date, status, finished_at;

-- name: CompleteActiveWorkoutsExcept :exec
UPDATE workouts
SET status = 'completed'
WHERE user_id = $1 AND status = 'active' AND id <> $2;

-- name: CompleteAllActiveWorkoutsForUser :exec
UPDATE workouts SET status = 'completed' WHERE user_id = $1 AND status = 'active';

-- name: GetActiveWorkoutForUser :one
SELECT id, user_id, date, status, finished_at FROM workouts WHERE user_id = $1 AND status = 'active' LIMIT 1;

-- name: GetWorkoutByIDForUser :one
SELECT id, user_id, date, status, finished_at FROM workouts WHERE id = $1 AND user_id = $2;

-- name: CompleteWorkout :exec
UPDATE workouts SET status = 'completed', finished_at = $3 WHERE id = $1 AND user_id = $2;

-- name: DeleteWorkout :exec
DELETE FROM workouts WHERE id = $1 AND user_id = $2;

-- name: GetLastWorkoutExerciseData :one
SELECT we.sets, w.date
FROM workout_exercises we
JOIN workouts w ON w.id = we.workout_id
WHERE w.user_id = $1 AND w.status = 'completed' AND we.exercise_id = $2
ORDER BY w.date DESC
LIMIT 1;

-- name: UpsertWorkoutExercise :one
INSERT INTO workout_exercises (workout_id, exercise_id, sets)
VALUES ($1, $2, $3)
ON CONFLICT (workout_id, exercise_id) DO UPDATE SET sets = EXCLUDED.sets
RETURNING id, workout_id, exercise_id, sets;

-- name: DeleteWorkoutExercise :exec
DELETE FROM workout_exercises WHERE workout_id = $1 AND exercise_id = $2;

-- name: ListWorkoutExercisesOfExercise :many
SELECT id, workout_id, exercise_id, sets FROM workout_exercises WHERE exercise_id = $1 ORDER BY workout_id FOR UPDATE;

-- name: GetWorkoutExerciseOfWorkout :one
SELECT id, workout_id, exercise_id, sets FROM workout_exercises WHERE workout_id = $1 AND exercise_id = $2 FOR UPDATE;

-- name: UpdateWorkoutExerciseSets :exec
UPDATE workout_exercises SET sets = $2 WHERE id = $1;

-- name: MoveWorkoutExercise :exec
UPDATE workout_exercises SET exercise_id = $2 WHERE id = $1;

-- name: DeleteWorkoutExerciseByID :exec
DELETE FROM workout_exercises WHERE id = $1;

-- name: CountWorkoutExercisesOfExercise :one
SELECT COUNT(*)::int FROM workout_exercises WHERE exercise_id = $1;
