-- What an exercise can be done with: a weight, the body itself, an elastic band.
-- The first kind is the default one. Sets of other kinds are stored in the set JSON ("kind").
ALTER TABLE exercises ADD COLUMN loads TEXT[] NOT NULL DEFAULT '{weight}';

UPDATE exercises SET loads = '{weight,band,bodyweight}'
WHERE name IN ('Подтягивания', 'подтягивания');

UPDATE exercises SET loads = '{weight,bodyweight}'
WHERE name IN ('Негативные подтягивания', 'Гиперэкстензия');

UPDATE exercises SET loads = '{weight,bodyweight,band}'
WHERE name = 'Брусья';

UPDATE exercises SET loads = '{bodyweight,weight}'
WHERE name IN ('Обратные отжимания от скамьи', 'Планка', 'планка', 'Уголок', 'Скрутки на пресс', 'Вращения торса');

INSERT INTO exercises (name, muscle_group_id, loads)
SELECT 'Отжимания', mg.id, '{bodyweight,weight,band}'
FROM muscle_groups mg
WHERE mg.name = 'грудь'
  AND NOT EXISTS (SELECT 1 FROM exercises e WHERE e.name IN ('Отжимания', 'отжимания'));

-- Sets of bodyweight exercises were written as "reps × 0 kg". Mark them as bodyweight sets,
-- keeping every other field of the set and their order.
UPDATE workout_exercises we
SET sets = (
    SELECT COALESCE(jsonb_agg(
        CASE
            WHEN s.elem ? 'kind' OR COALESCE((s.elem ->> 'weight')::numeric, 0) <> 0 THEN s.elem
            ELSE s.elem || '{"kind": "bodyweight"}'::jsonb
        END
        ORDER BY s.ord), '[]'::jsonb)
    FROM jsonb_array_elements(we.sets) WITH ORDINALITY AS s(elem, ord)
)
FROM exercises e
WHERE e.id = we.exercise_id
  AND e.loads[1] = 'bodyweight'
  AND jsonb_typeof(we.sets) = 'array';
