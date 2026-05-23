-- Reverse: rebuild with INTEGER for corner_time / cornered_time.
-- Decimal values will be truncated by SQLite's INTEGER affinity.

CREATE TABLE play_stats_snapshots_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  character TEXT NOT NULL,
  match_replay_id TEXT,
  snapshot_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,

  battle_hub_match_play_count INTEGER,
  casual_match_play_count INTEGER,
  corner_time INTEGER,
  cornered_time INTEGER,
  custom_room_match_play_count INTEGER,
  drive_impact REAL,
  drive_impact_to_drive_impact REAL,
  drive_parry REAL,
  drive_reversal REAL,
  gauge_rate_ca REAL,
  gauge_rate_drive_arts REAL,
  gauge_rate_drive_guard REAL,
  gauge_rate_drive_impact REAL,
  gauge_rate_drive_other REAL,
  gauge_rate_drive_reversal REAL,
  gauge_rate_drive_rush_from_cancel REAL,
  gauge_rate_drive_rush_from_parry REAL,
  gauge_rate_sa_lv1 REAL,
  gauge_rate_sa_lv2 REAL,
  gauge_rate_sa_lv3 REAL,
  just_parry REAL,
  punish_counter REAL,
  rank_match_play_count INTEGER,
  received_drive_impact REAL,
  received_drive_impact_to_drive_impact REAL,
  received_punish_counter REAL,
  received_stun REAL,
  received_throw_count REAL,
  received_throw_drive_parry REAL,
  rival_ai_achieved_challenge_count INTEGER,
  rival_ai_highest_league_rank INTEGER,
  rival_ai_highest_league_rank_txt TEXT,
  stun REAL,
  target_clear_count INTEGER,
  throw_count REAL,
  throw_drive_parry REAL,
  throw_tech REAL,
  total_all_character_play_point INTEGER,

  enjoy_fight_point INTEGER,
  enjoy_total_point INTEGER,
  enjoy_user_point INTEGER,

  world_tour_seconds INTEGER,
  ranked_match_seconds INTEGER,
  casual_match_seconds INTEGER,
  custom_room_seconds INTEGER,
  battle_hub_seconds INTEGER,
  offline_match_seconds INTEGER,
  arcade_seconds INTEGER,
  practice_seconds INTEGER,
  extreme_seconds INTEGER
);

INSERT INTO play_stats_snapshots_new
  SELECT * FROM play_stats_snapshots;

DROP TABLE play_stats_snapshots;

ALTER TABLE play_stats_snapshots_new RENAME TO play_stats_snapshots;

CREATE INDEX IF NOT EXISTS idx_play_stats_user_char_at
  ON play_stats_snapshots(user_id, character, snapshot_at);

CREATE INDEX IF NOT EXISTS idx_play_stats_match_replay_id
  ON play_stats_snapshots(match_replay_id);
