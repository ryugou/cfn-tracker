CREATE TABLE IF NOT EXISTS vegapunk_sync_queue (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  dedupe_key TEXT NOT NULL UNIQUE,
  payload_json TEXT NOT NULL,
  attempts INTEGER DEFAULT 0 NOT NULL,
  last_error TEXT DEFAULT '' NOT NULL,
  next_attempt_at TEXT DEFAULT '' NOT NULL,
  processed_at TEXT DEFAULT '' NOT NULL,
  created_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,
  updated_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_vegapunk_sync_queue_due
  ON vegapunk_sync_queue(processed_at, next_attempt_at, id);
