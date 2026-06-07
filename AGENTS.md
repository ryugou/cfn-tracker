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
  - `app/pkg/model/sf6_character_data.go`
  - `app/pkg/storage/sql/play_stats.go`
  - `app/pkg/storage/sql/benchmark.go`
  - `app/pkg/storage/sql/advice.go`
  - `app/pkg/storage/sql/sf6_character_data.go`

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
- `play_stats_snapshots` is raw acquisition data. UI/detail tables must not derive per-match values from this table at render time.
- When a replay-linked snapshot is saved, `SavePlayStats` computes per-match values from the previous snapshot and stores them in `match_play_stats`.
- `corner_time` and `cornered_time` are REAL values after migration `000005`.
- Automatic refresh:
  - Started from Wails `OnStartup` via `StartAutoDataRefresh`.
  - For each registered user, fetches `/play` and saves a snapshot when the latest local snapshot is missing or older than 1 hour.
  - Waits 1 minute after app startup before the first play-stats check.
  - Waits 30 seconds between registered users to reduce authenticated site load.

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

### match_play_stats

Derived per-match SF6 play stats calculated at acquisition time.

- Source: adjacent rows in `play_stats_snapshots`.
- Created only when a snapshot has `match_replay_id` and there is a previous snapshot for the same user.
- `GetMatchesWithPlayStats` reads this table, not raw `play_stats_snapshots`.
- UI must display these stored values directly and must not recalculate snapshot deltas.
- Current scale: `(current_average - previous_average) * 100`, matching Capcom's 100-match moving-average style data.

Columns:

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `user_id` TEXT NOT NULL
- `match_replay_id` TEXT NOT NULL UNIQUE
- `snapshot_id` INTEGER NOT NULL
- `previous_snapshot_id` INTEGER
- `computed_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`
- `drive_impact`
- `received_drive_impact`
- `just_parry`
- `throw_tech`
- `corner_time`
- `cornered_time`
- `throw_count`
- `received_punish_counter`

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
- Automatic refresh:
  - Started from Wails `OnStartup` via `StartAutoDataRefresh`.
  - Targets registered users and characters that already exist in tracked `matches`.
  - Refreshes only when the user/character benchmark cache is missing or older than 24 hours.
  - Waits 2 minutes after app startup before the first benchmark check.
  - Waits 2 minutes between stale benchmark jobs to reduce authenticated site load.

Match / play-stat automatic refresh:

- Started from Wails `OnStartup` via `StartAutoDataRefresh`.
- Registered CFN users are scanned every 3 minutes through the authenticated SF6 battlelog path.
- Missing ranked matches are imported with `SF6Tracker.BackfillMatches`, deduped by replay ID.
- Imported matches are saved to SQLite, text output, and the Vegapunk sync queue.
- When at least one missing match is imported, the app fetches one current `/play` snapshot and attaches it to the newest imported replay ID.
- Separate stale `/play` snapshots are still refreshed hourly for registered users when no fresh snapshot exists; the first startup check waits 4 minutes so the 3-minute match import can run first.
- Existing historical matches can be recovered from battlelog while they remain available, but exact historical `/play` per-match values cannot be reconstructed after the fact because Capcom returns only the current `/play` state.

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

### vegapunk_sync_queue

Persistent retry queue for PunkRecord/vegapunk growth-data sync.

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `kind` TEXT NOT NULL
- `dedupe_key` TEXT NOT NULL UNIQUE
- `payload_json` TEXT NOT NULL
- `attempts` INTEGER NOT NULL DEFAULT 0
- `last_error` TEXT NOT NULL DEFAULT ''
- `next_attempt_at` TEXT NOT NULL DEFAULT ''
- `processed_at` TEXT NOT NULL DEFAULT ''
- `created_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`
- `updated_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`

Behavior:

- Match, play-stat snapshot, and advice-run saves enqueue graph payloads here first.
- A background worker sends due jobs to vegapunk.
- Success fills `processed_at`.
- Failure keeps the row, increments `attempts`, records `last_error`, and sets `next_attempt_at` using exponential backoff.
- The app should not treat a saved local DB row as synced to PunkRecord unless the corresponding queue row has `processed_at` set.

### sf6_character_moves

Official SF6 character movelist and frame-data cache. Data is fetched from the public Street Fighter 6 character pages, not from the authenticated CFN/Buckler data path.

URL pattern:

- Movelist: `https://www.streetfighter.com/6/{locale}/character/{character}/movelist`
- Frame data: `https://www.streetfighter.com/6/{locale}/character/{character}/frame`
- Example: `https://www.streetfighter.com/6/ja-jp/character/ingrid/frame`

Columns:

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `character` TEXT NOT NULL
- `locale` TEXT NOT NULL
- `source` TEXT NOT NULL: `movelist` or `frame`
- `category` TEXT NOT NULL DEFAULT ''
- `name` TEXT NOT NULL
- `command` TEXT NOT NULL DEFAULT ''
- `description` TEXT NOT NULL DEFAULT ''
- `startup` TEXT NOT NULL DEFAULT ''
- `active` TEXT NOT NULL DEFAULT ''
- `recovery` TEXT NOT NULL DEFAULT ''
- `hit_advantage` TEXT NOT NULL DEFAULT ''
- `block_advantage` TEXT NOT NULL DEFAULT ''
- `cancel` TEXT NOT NULL DEFAULT ''
- `damage` TEXT NOT NULL DEFAULT ''
- `combo_scaling` TEXT NOT NULL DEFAULT ''
- `drive_gauge_gain_hit` TEXT NOT NULL DEFAULT ''
- `drive_gauge_loss_block` TEXT NOT NULL DEFAULT ''
- `drive_gauge_loss_punish` TEXT NOT NULL DEFAULT ''
- `sa_gauge_gain` TEXT NOT NULL DEFAULT ''
- `attribute` TEXT NOT NULL DEFAULT ''
- `remarks` TEXT NOT NULL DEFAULT ''
- `raw_text` TEXT NOT NULL DEFAULT ''
- `source_url` TEXT NOT NULL DEFAULT ''
- `fetched_at` TEXT NOT NULL DEFAULT ''
- `created_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`
- `updated_at` TEXT NOT NULL DEFAULT `DATETIME('NOW')`

Constraints/indexes:

- Unique: `(character, locale, source, category, name, command, startup, active)`
- Index: `idx_sf6_character_moves_character_locale(character, locale)`
- Index: `idx_sf6_character_moves_lookup(character, locale, source, category, name)`

Collection spec:

- Parser lives in `app/pkg/tracker/sf6/official`.
- Manual single-character sync: `go run ./tools/sync-sf6-character-data -character ingrid -locale ja-jp`.
- Manual all-character sync: `go run ./tools/sync-sf6-character-data -all -locale ja-jp -delay 500ms`.
- App command: `SyncSF6CharacterData(character, locale)`.
- The advice prompt receives a relevant subset as `characterKnowledge`.
- Character-specific move names, commands, frame values, combos, or sequence names may be used only when grounded by `characterKnowledge` or PunkRecord evidence. Missing character-specific details must not be guessed.
- Official character data does not auto-refresh on startup because this data rarely changes. If Capcom updates move/frame data, run the manual all-character sync.
- Official character-page slugs differ from some CFN/internal names:
  - Akuma: `gouki_akuma`
  - M. Bison: `vega_mbison`
  - E. Honda: `ehonda`

## Advice Generation

Command entry point:

- `GenerateAdviceComparison(userID, character)`

Inputs:

- Latest `play_stats_snapshots` row.
- Recent play-stat snapshots, currently treated as a 30-snapshot window.
- Benchmark averages from `benchmark_players`.
- Relevant official character movelist/frame rows from `sf6_character_moves`, passed as `characterKnowledge`.
- PunkRecord/vegapunk evidence for PunkRecord modes.

Source-of-truth boundary:

- Exact metric values, deltas, benchmark averages, match counts, LP/MR values, and win/loss records must come from SQLite/RDB queries.
- Exact character move names, commands, frame values, cancel properties, attributes, and official notes must come from `sf6_character_moves` or PunkRecord evidence.
- PunkRecord/vegapunk is a narrative memory layer. It stores SF6 knowledge, advice actions, qualitative observation summaries, side-effect hypotheses, feedback, and similar past cases.
- Do not use vegapunk as a numeric fact store or an effect-measurement engine.
- GraphRAG evidence can support plausible explanations and analogies, but it must not be presented as causal proof.

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

Role:

- RDB/API: numeric source of truth and all exact aggregation.
- Vegapunk/PunkRecord: narrative knowledge, prior advice, qualitative observation summaries, side-effect hypotheses, and similar-case retrieval.
- LLM: combines the current RDB facts with retrieved PunkRecord context.

Sync payload rule:

- Do not enqueue raw metric values, metric deltas, LP/MR, wins/losses, or win-rate facts as vegapunk node attributes/text.
- Sync only RDB references, timestamps, character context, qualitative observation summaries, advice candidates, evidence text, and feedback/side-effect narrative.
- If exact numbers are needed for advice, query SQLite again instead of reading them from vegapunk.

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
