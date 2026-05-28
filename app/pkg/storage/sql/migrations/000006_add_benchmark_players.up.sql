CREATE TABLE IF NOT EXISTS benchmark_players (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source_user_id TEXT NOT NULL,
  target_user_id TEXT NOT NULL,
  fighter_id TEXT NOT NULL,
  character TEXT NOT NULL,
  character_tool_name TEXT NOT NULL,
  rank_offset INTEGER NOT NULL,
  league_rank INTEGER NOT NULL,
  lp INTEGER DEFAULT 0 NOT NULL,
  mr INTEGER DEFAULT 0 NOT NULL,
  mr_ranking INTEGER DEFAULT 0 NOT NULL,
  last_play_at INTEGER DEFAULT 0 NOT NULL,
  fetched_at TEXT,
  stats_json TEXT,
  last_error TEXT DEFAULT '' NOT NULL,
  created_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,
  updated_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,
  UNIQUE(source_user_id, target_user_id, character)
);

CREATE INDEX IF NOT EXISTS idx_benchmark_players_source_character
  ON benchmark_players(source_user_id, character, rank_offset);
