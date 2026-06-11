CREATE TABLE IF NOT EXISTS sf6_character_moves (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    character TEXT NOT NULL,
    locale TEXT NOT NULL,
    source TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    command TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    startup TEXT NOT NULL DEFAULT '',
    active TEXT NOT NULL DEFAULT '',
    recovery TEXT NOT NULL DEFAULT '',
    hit_advantage TEXT NOT NULL DEFAULT '',
    block_advantage TEXT NOT NULL DEFAULT '',
    cancel TEXT NOT NULL DEFAULT '',
    damage TEXT NOT NULL DEFAULT '',
    combo_scaling TEXT NOT NULL DEFAULT '',
    drive_gauge_gain_hit TEXT NOT NULL DEFAULT '',
    drive_gauge_loss_block TEXT NOT NULL DEFAULT '',
    drive_gauge_loss_punish TEXT NOT NULL DEFAULT '',
    sa_gauge_gain TEXT NOT NULL DEFAULT '',
    attribute TEXT NOT NULL DEFAULT '',
    remarks TEXT NOT NULL DEFAULT '',
    raw_text TEXT NOT NULL DEFAULT '',
    source_url TEXT NOT NULL DEFAULT '',
    fetched_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (DATETIME('NOW')),
    updated_at TEXT NOT NULL DEFAULT (DATETIME('NOW')),
    UNIQUE(character, locale, source, category, name, command, startup, active)
);

CREATE INDEX IF NOT EXISTS idx_sf6_character_moves_character_locale
    ON sf6_character_moves(character, locale);

CREATE INDEX IF NOT EXISTS idx_sf6_character_moves_lookup
    ON sf6_character_moves(character, locale, source, category, name);
