-- +goose Up
-- Allergens and workout activities get a stable code (#158): the interface
-- translates them by code ("reference.allergen.gluten") instead of showing
-- the Italian name stored here. Rows with no code (not seeded by 0015/0016)
-- keep showing their stored name.
ALTER TABLE allergen_tags ADD COLUMN code TEXT;
UPDATE allergen_tags SET code = CASE name
    WHEN 'glutine'            THEN 'gluten'
    WHEN 'lattosio'           THEN 'lactose'
    WHEN 'uova'               THEN 'eggs'
    WHEN 'frutta a guscio'    THEN 'tree_nuts'
    WHEN 'arachidi'           THEN 'peanuts'
    WHEN 'soia'               THEN 'soy'
    WHEN 'pesce'              THEN 'fish'
    WHEN 'crostacei'          THEN 'crustaceans'
    WHEN 'sedano'             THEN 'celery'
    WHEN 'senape'             THEN 'mustard'
    WHEN 'sesamo'             THEN 'sesame'
    WHEN 'molluschi'          THEN 'mollusks'
    WHEN 'lupino'             THEN 'lupin'
    WHEN 'anidride solforosa' THEN 'sulfur_dioxide'
END;
CREATE UNIQUE INDEX idx_allergen_tags_code ON allergen_tags(code);

ALTER TABLE workout_activities ADD COLUMN code TEXT;
UPDATE workout_activities SET code = CASE name
    WHEN 'Palestra'          THEN 'gym'
    WHEN 'Beach Volley'      THEN 'beach_volleyball'
    WHEN 'Pallavolo'         THEN 'volleyball'
    WHEN 'Tennis'            THEN 'tennis'
    WHEN 'Calcio'            THEN 'soccer'
    WHEN 'Nuoto'             THEN 'swimming'
    WHEN 'Corsa'             THEN 'running'
    WHEN 'Ciclismo'          THEN 'cycling'
    WHEN 'Camminata veloce'  THEN 'brisk_walking'
    WHEN 'Yoga / Stretching' THEN 'yoga_stretching'
    WHEN 'Altro'             THEN 'other'
END;
CREATE UNIQUE INDEX idx_workout_activities_code ON workout_activities(code);

-- +goose Down
DROP INDEX idx_workout_activities_code;
ALTER TABLE workout_activities DROP COLUMN code;
DROP INDEX idx_allergen_tags_code;
ALTER TABLE allergen_tags DROP COLUMN code;
