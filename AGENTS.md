# CFN Tracker Agent Notes

This file records project facts that should not be rediscovered every time.
Keep it updated when DB schema, data collection, or advice/PunkRecord behavior changes.

## Repository Workflow

- Do not push directly to `master`. Work on a branch and open/update a PR.
- Local untracked tool/runtime folders such as `.claude` may exist. Do not delete user tool state unless explicitly asked.
- Prefer the repo's existing Go/Wails/Vite patterns. Regenerate Wails bindings with `task bind` when command/model signatures change.

## Local Database

- The app uses SQLite.
- DB path is derived in `app/pkg/storage/sql/storage.go` from `os.UserCacheDir()`:
  - macOS current path: `~/Library/Caches/cfn-tracker/cfn-tracker.db`
- Migrations live in `app/pkg/storage/sql/migrations`.
- Migration state is stored in the `migrations` table.
- Main model/storage files:
  - `app/pkg/model/play_stats.go`
  - `app/pkg/model/benchmark.go`
  - `app/pkg/model/advice.go`
  - `app/pkg/storage/sql/play_stats.go`
  - `app/pkg/storage/sql/benchmark.go`
  - `app/pkg/storage/sql/advice.go`

## DB Tables

### users

Registered CFN users.

- `code` TEXT PRIMARY KEY
- `display_name` TEXT NOT NULL

### sessions

Tracking sessions for one user.

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `user_id` TEXT NOT NULL, FK to `users(code)`
- `lp` INTEGER NOT NULL DEFAULT 0
- `mr` INTEGER NOT NULL DEFAULT 0
- `created_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`

### matches

Per-match tracking records.

- Primary key: `(session_id, date, time)`
- FKs: `session_id` to `sessions(id)`, `user_id` to `users(code)`
- Columns:
  - `user_id`
  - `session_id`
  - `character`
  - `lp`, `lp_gain`
  - `mr`, `mr_gain`
  - `opponent`
  - `opponent_character`
  - `opponent_lp`
  - `opponent_mr`
  - `opponent_league`
  - `victory`
  - `wins`, `losses`, `win_streak`, `win_rate`
  - `date`, `time`
  - `replay_id`
  - `user_name`

### play_stats_snapshots

SF6 play-stat snapshots fetched from the authenticated Buckler/CFN site data.
This is not a public official API. It uses the same authenticated site data path as the rest of the tracker.

Important behavior:

- Data is user-wide SF6 play stats, not per-character stats.
- `character` is stored for app filtering/context, but the raw `/play` stats are not character-specific.
- A baseline snapshot is captured at tracking start if there is no recent snapshot.
- Additional snapshots are captured on new matches. `match_replay_id` is set when available.
- `corner_time` and `cornered_time` are REAL values after migration `000005`.

Identity/time columns:

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `user_id` TEXT NOT NULL
- `character` TEXT NOT NULL
- `match_replay_id` TEXT
- `snapshot_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`

Battle-stat columns:

- `battle_hub_match_play_count`
- `casual_match_play_count`
- `corner_time`
- `cornered_time`
- `custom_room_match_play_count`
- `drive_impact`
- `drive_impact_to_drive_impact`
- `drive_parry`
- `drive_reversal`
- `gauge_rate_ca`
- `gauge_rate_drive_arts`
- `gauge_rate_drive_guard`
- `gauge_rate_drive_impact`
- `gauge_rate_drive_other`
- `gauge_rate_drive_reversal`
- `gauge_rate_drive_rush_from_cancel`
- `gauge_rate_drive_rush_from_parry`
- `gauge_rate_sa_lv1`
- `gauge_rate_sa_lv2`
- `gauge_rate_sa_lv3`
- `just_parry`
- `punish_counter`
- `rank_match_play_count`
- `received_drive_impact`
- `received_drive_impact_to_drive_impact`
- `received_punish_counter`
- `received_stun`
- `received_throw_count`
- `received_throw_drive_parry`
- `rival_ai_achieved_challenge_count`
- `rival_ai_highest_league_rank`
- `rival_ai_highest_league_rank_txt`
- `stun`
- `target_clear_count`
- `throw_count`
- `throw_drive_parry`
- `throw_tech`
- `total_all_character_play_point`

Enjoy-point columns:

- `enjoy_fight_point`
- `enjoy_total_point`
- `enjoy_user_point`

Content play-time columns, stored in seconds:

- `world_tour_seconds`
- `ranked_match_seconds`
- `casual_match_seconds`
- `custom_room_seconds`
- `battle_hub_seconds`
- `offline_match_seconds`
- `arcade_seconds`
- `practice_seconds`
- `extreme_seconds`

Indexes:

- `idx_play_stats_user_char_at(user_id, character, snapshot_at)`
- `idx_play_stats_match_replay_id(match_replay_id)`

### benchmark_players

Cached comparison-player data for the analysis/advice screens.

Columns:

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `source_user_id` TEXT NOT NULL
- `target_user_id` TEXT NOT NULL
- `fighter_id` TEXT NOT NULL
- `character` TEXT NOT NULL
- `character_tool_name` TEXT NOT NULL
- `rank_offset` INTEGER NOT NULL
- `league_rank` INTEGER NOT NULL
- `lp` INTEGER NOT NULL DEFAULT 0
- `mr` INTEGER NOT NULL DEFAULT 0
- `mr_ranking` INTEGER NOT NULL DEFAULT 0
- `last_play_at` INTEGER NOT NULL DEFAULT 0
- `fetched_at` TEXT
- `stats_json` TEXT
- `last_error` TEXT NOT NULL DEFAULT ''
- `created_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`
- `updated_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`
- `wins` INTEGER NOT NULL DEFAULT 0
- `losses` INTEGER NOT NULL DEFAULT 0
- `win_diff` INTEGER NOT NULL DEFAULT 0

Constraints/indexes:

- Unique: `(source_user_id, target_user_id, character)`
- Index: `idx_benchmark_players_source_character(source_user_id, character, rank_offset)`

Benchmark collection spec:

- Target users are same-character players.
- Current target bands are:
  - `2つ上`
  - `次ランク帯`
- For Master, the target bands are MR +100 and MR +200 instead of league-rank bands.
- The app fetches about 20 candidates per band, stores all fetched candidates, then sorts by `win_diff = wins - losses`.
- Analysis/advice aggregation uses the top 5 per band. The UI should not show all 40 candidates in the summary table.
- Benchmark data is cached indefinitely for normal use.
- Refresh resets/replaces the cache for the current source user and character.
- Rate limits matter because this uses the authenticated site data path. Avoid tight loops and unnecessary refreshes.

### advice_runs

One generated advice batch.

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `user_id` TEXT NOT NULL
- `character` TEXT NOT NULL
- `input_window` INTEGER NOT NULL DEFAULT 30
- `snapshot_at` TEXT NOT NULL DEFAULT ''
- `created_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`

### advice_candidates

Generated advice candidate per mode/model.

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `run_id` INTEGER NOT NULL, FK to `advice_runs(id)` ON DELETE CASCADE
- `mode` TEXT NOT NULL
- `priority` TEXT NOT NULL DEFAULT ''
- `theme` TEXT NOT NULL DEFAULT ''
- `summary` TEXT NOT NULL DEFAULT ''
- `rationale` TEXT NOT NULL DEFAULT ''
- `action` TEXT NOT NULL DEFAULT ''
- `drill` TEXT NOT NULL DEFAULT ''
- `success_criteria` TEXT NOT NULL DEFAULT ''
- `watch_metrics` TEXT NOT NULL DEFAULT ''
- `risks` TEXT NOT NULL DEFAULT ''
- `evidence_json` TEXT NOT NULL DEFAULT '[]'
- `created_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`

Indexes:

- `idx_advice_candidates_run_mode(run_id, mode)`

### advice_feedback

User evaluation for generated advice.

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `run_id` INTEGER NOT NULL, FK to `advice_runs(id)` ON DELETE CASCADE
- `mode` TEXT NOT NULL
- `rating` INTEGER NOT NULL DEFAULT 0
- `specificity` INTEGER NOT NULL DEFAULT 0
- `usefulness` INTEGER NOT NULL DEFAULT 0
- `trust` INTEGER NOT NULL DEFAULT 0
- `comment` TEXT NOT NULL DEFAULT ''
- `created_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`

## Advice Generation

Command entry point:

- `GenerateAdviceComparison(userID, character)`

Inputs:

- Latest `play_stats_snapshots` row.
- Recent play-stat snapshots, currently treated as a 30-snapshot window.
- Benchmark averages from `benchmark_players`.
- PunkRecord/vegapunk evidence for PunkRecord modes.

Current generated modes, displayed as tabs:

- `punk_record_opus_4_6`: Punk Record with Claude Opus 4.6.
- `punk_record_sonnet_4_6`: Punk Record with Claude Sonnet 4.6.
- `db_only`: DB-only advice without PunkRecord evidence.

Advice signals currently emphasized:

- `received_drive_impact`
- `just_parry`
- `throw_tech`
- `cornered_time`
- `received_punish_counter`
- `throw_count`

Persistence:

- Every generated candidate is saved to `advice_candidates`.
- Advice history is intentionally retained so changes in advice can be compared during the PunkRecord/GraphRAG validation.
- User feedback is saved separately in `advice_feedback`.

Data-cleanup note:

- On 2026-05-30, bad implementation/test-generated advice history was deleted from the local DB only:
  - `advice_runs`: 22 to 0
  - `advice_candidates`: 65 to 0
  - `advice_feedback`: 0
- The cleanup preserved:
  - `play_stats_snapshots`: 22
  - `benchmark_players`: 80
  - `matches`: 98
- Backup was created at:
  - `~/Library/Caches/cfn-tracker/cfn-tracker.db.backup-before-advice-clean-20260530`

## PunkRecord / Vegapunk

Vegapunk is the GraphRAG backend used for Punk Record advice.

Local/runtime facts:

- SSH host alias: `vegapunk`
- gRPC target used by the app: `vegapunk.local:6840`
- Do not use `vegapunk:6840` for app gRPC unless local DNS is changed; it failed with `lookup vegapunk: no such host`.
- Proto path:
  - `/Users/ryugo/Developer/src/AI-Project/vegapunk/proto/graphrag.proto`
- gRPC reflection is not available, so CLI checks need the proto file.
- Schema name:
  - `sf6-advice`

Current `sf6-advice` seed state:

- The initial SF6 advice graph was seeded directly with `UpsertGraph`.
- Do not use large `IngestRaw` for deterministic seed data unless intentionally testing async extraction; it previously created partial async extraction jobs.
- Current seed count:
  - 314 nodes
  - 428 edges
  - 314 vectors

Search behavior:

- App env key: `VEGAPUNK_SEARCH_TIMEOUT_SECONDS`
- Default timeout: 30 seconds.
- A measured real search took about 7.69 seconds; the earlier 8-second timeout was too tight and could surface as `signal: killed`.

## Environment and Secrets

`app/.env` is ignored and should contain 1Password references, not raw secrets.

Expected local keys:

- `ANTHROPIC_API_KEY=op://ai-agents/CFN-Tracker/credential`
- `VEGAPUNK_TOKEN=op://ai-agents/vegapunk_ryugo_dev/credential`
- `VEGAPUNK_GRPC_TARGET=vegapunk.local:6840`
- `VEGAPUNK_PROTO=/Users/ryugo/Developer/src/AI-Project/vegapunk/proto/graphrag.proto`

Rules:

- Do not commit raw API keys or bearer tokens.
- Do not print secrets in terminal output.
- Avoid commands such as `ps` while a `grpcurl` process has `authorization: Bearer ...` in argv; that can expose the token in process args.
- The real app should resolve `op://` references synchronously during env load. Tests/binding generation should not require 1Password.
