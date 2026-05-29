CREATE TABLE IF NOT EXISTS advice_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  character TEXT NOT NULL,
  input_window INTEGER DEFAULT 30 NOT NULL,
  snapshot_at TEXT DEFAULT '' NOT NULL,
  created_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL
);

CREATE TABLE IF NOT EXISTS advice_candidates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id INTEGER NOT NULL,
  mode TEXT NOT NULL,
  priority TEXT DEFAULT '' NOT NULL,
  theme TEXT DEFAULT '' NOT NULL,
  summary TEXT DEFAULT '' NOT NULL,
  rationale TEXT DEFAULT '' NOT NULL,
  action TEXT DEFAULT '' NOT NULL,
  drill TEXT DEFAULT '' NOT NULL,
  success_criteria TEXT DEFAULT '' NOT NULL,
  watch_metrics TEXT DEFAULT '' NOT NULL,
  risks TEXT DEFAULT '' NOT NULL,
  evidence_json TEXT DEFAULT '[]' NOT NULL,
  created_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,
  FOREIGN KEY(run_id) REFERENCES advice_runs(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_advice_runs_user_character_created
  ON advice_runs(user_id, character, created_at);

CREATE INDEX IF NOT EXISTS idx_advice_candidates_run_mode
  ON advice_candidates(run_id, mode);

CREATE TABLE IF NOT EXISTS advice_feedback (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id INTEGER NOT NULL,
  mode TEXT NOT NULL,
  rating INTEGER DEFAULT 0 NOT NULL,
  specificity INTEGER DEFAULT 0 NOT NULL,
  usefulness INTEGER DEFAULT 0 NOT NULL,
  trust INTEGER DEFAULT 0 NOT NULL,
  comment TEXT DEFAULT '' NOT NULL,
  created_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,
  FOREIGN KEY(run_id) REFERENCES advice_runs(id) ON DELETE CASCADE
);
