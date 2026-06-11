CREATE TABLE IF NOT EXISTS match_play_stats (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  match_replay_id TEXT NOT NULL UNIQUE,
  snapshot_id INTEGER NOT NULL,
  previous_snapshot_id INTEGER,
  computed_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,

  drive_impact REAL NOT NULL DEFAULT 0,
  received_drive_impact REAL NOT NULL DEFAULT 0,
  just_parry REAL NOT NULL DEFAULT 0,
  throw_tech REAL NOT NULL DEFAULT 0,
  corner_time REAL NOT NULL DEFAULT 0,
  cornered_time REAL NOT NULL DEFAULT 0,
  throw_count REAL NOT NULL DEFAULT 0,
  received_punish_counter REAL NOT NULL DEFAULT 0,

  FOREIGN KEY(snapshot_id) REFERENCES play_stats_snapshots(id),
  FOREIGN KEY(previous_snapshot_id) REFERENCES play_stats_snapshots(id)
);

CREATE INDEX IF NOT EXISTS idx_match_play_stats_user_replay
  ON match_play_stats(user_id, match_replay_id);

INSERT OR IGNORE INTO match_play_stats (
  user_id, match_replay_id, snapshot_id, previous_snapshot_id,
  drive_impact, received_drive_impact, just_parry, throw_tech,
  corner_time, cornered_time, throw_count, received_punish_counter
)
WITH ordered AS (
  SELECT
    id,
    user_id,
    match_replay_id,
    drive_impact,
    received_drive_impact,
    just_parry,
    throw_tech,
    corner_time,
    cornered_time,
    throw_count,
    received_punish_counter,
    LAG(id) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_id,
    LAG(drive_impact) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_drive_impact,
    LAG(received_drive_impact) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_received_drive_impact,
    LAG(just_parry) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_just_parry,
    LAG(throw_tech) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_throw_tech,
    LAG(corner_time) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_corner_time,
    LAG(cornered_time) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_cornered_time,
    LAG(throw_count) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_throw_count,
    LAG(received_punish_counter) OVER (PARTITION BY user_id ORDER BY snapshot_at ASC, id ASC) AS previous_received_punish_counter
  FROM play_stats_snapshots
)
SELECT
  user_id,
  match_replay_id,
  id,
  previous_id,
  (drive_impact - previous_drive_impact) * 100.0,
  (received_drive_impact - previous_received_drive_impact) * 100.0,
  (just_parry - previous_just_parry) * 100.0,
  (throw_tech - previous_throw_tech) * 100.0,
  (corner_time - previous_corner_time) * 100.0,
  (cornered_time - previous_cornered_time) * 100.0,
  (throw_count - previous_throw_count) * 100.0,
  (received_punish_counter - previous_received_punish_counter) * 100.0
FROM ordered
WHERE match_replay_id IS NOT NULL
  AND match_replay_id != ''
  AND previous_id IS NOT NULL;
