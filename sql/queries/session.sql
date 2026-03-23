-- name: GetUserWorkoutState :one
SELECT user_id, active_workout_id, state, updated_at FROM user_workout_state WHERE user_id = $1;

-- name: UpsertUserWorkoutState :exec
INSERT INTO user_workout_state (user_id, active_workout_id, state, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (user_id) DO UPDATE SET
    active_workout_id = EXCLUDED.active_workout_id,
    state = EXCLUDED.state,
    updated_at = NOW();

-- name: ClearUserWorkoutState :exec
DELETE FROM user_workout_state WHERE user_id = $1;
