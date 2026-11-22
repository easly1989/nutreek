-- +goose Up
-- Notifications (#237): Web Push to the installed PWA. A device that allows them
-- is a push subscription; each user has built-in reminders (water, gym, meals)
-- and custom ones, one quiet-hours band and the time zone of their browser. The
-- server sends from a minute ticker; notification_log keeps what was sent so a
-- reminder is not sent twice. The VAPID keys live in instance_settings.
CREATE TABLE push_subscriptions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   TEXT    NOT NULL UNIQUE,
    p256dh     TEXT    NOT NULL,
    auth       TEXT    NOT NULL,
    user_agent TEXT    NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_push_subscriptions_user ON push_subscriptions(user_id);

CREATE TABLE reminders (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         TEXT    NOT NULL CHECK (kind IN ('water', 'gym', 'meals', 'custom')),
    title        TEXT    NOT NULL DEFAULT '',
    enabled      INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    start_time   TEXT    NOT NULL DEFAULT '08:00',  -- HH:MM: the first water slot, the gym or custom time
    end_time     TEXT    NOT NULL DEFAULT '20:00',  -- HH:MM: the last water slot
    every_min    INTEGER NOT NULL DEFAULT 120,      -- minutes between water slots
    days         INTEGER NOT NULL DEFAULT 127,      -- bit 0 = Monday … bit 6 = Sunday
    lead_min     INTEGER NOT NULL DEFAULT 0,        -- minutes before the time (gym, meals)
    skip_done    INTEGER NOT NULL DEFAULT 1 CHECK (skip_done IN (0, 1))
);
CREATE INDEX idx_reminders_user ON reminders(user_id);
-- At most one built-in reminder of each kind per user.
CREATE UNIQUE INDEX idx_reminders_builtin ON reminders(user_id, kind) WHERE kind <> 'custom';

-- The meal names the «Meals» reminder stays quiet for.
CREATE TABLE notification_meal_mutes (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name    TEXT    NOT NULL COLLATE NOCASE,
    PRIMARY KEY (user_id, name)
);

CREATE TABLE notification_settings (
    user_id     INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    quiet_on    INTEGER NOT NULL DEFAULT 1 CHECK (quiet_on IN (0, 1)),
    quiet_from  TEXT    NOT NULL DEFAULT '22:00',
    quiet_to    TEXT    NOT NULL DEFAULT '07:00',
    time_zone   TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE notification_log (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key     TEXT    NOT NULL,
    sent_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, key)
);

-- +goose Down
DROP TABLE notification_log;
DROP TABLE notification_settings;
DROP TABLE notification_meal_mutes;
DROP TABLE reminders;
DROP TABLE push_subscriptions;
