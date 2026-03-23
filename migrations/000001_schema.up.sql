CREATE TABLE muscle_groups (
    id SERIAL PRIMARY KEY,
    name VARCHAR(50) NOT NULL UNIQUE
);

CREATE TABLE exercises (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    muscle_group_id INT NOT NULL REFERENCES muscle_groups (id) ON DELETE CASCADE
);

CREATE INDEX idx_exercises_muscle_group ON exercises (muscle_group_id);

CREATE TABLE workouts (
    id SERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    status VARCHAR(20) NOT NULL CHECK (status IN ('active', 'completed'))
);

CREATE INDEX idx_workouts_user_date ON workouts (user_id, date DESC);
CREATE INDEX idx_workouts_user_active ON workouts (user_id) WHERE status = 'active';

CREATE TABLE workout_exercises (
    id SERIAL PRIMARY KEY,
    workout_id INT NOT NULL REFERENCES workouts (id) ON DELETE CASCADE,
    exercise_id INT NOT NULL REFERENCES exercises (id),
    sets JSONB NOT NULL DEFAULT '[]'::jsonb,
    UNIQUE (workout_id, exercise_id)
);

CREATE INDEX idx_workout_exercises_workout ON workout_exercises (workout_id);

CREATE TABLE user_workout_state (
    user_id BIGINT PRIMARY KEY,
    active_workout_id INT REFERENCES workouts (id) ON DELETE SET NULL,
    state JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
