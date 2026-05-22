# Play Stats Phase A Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** SF6 `/play` ページの戦闘統計を新マッチ検知ごとにスナップショットとして蓄積し、`/stats` ダッシュボードで時系列推移を可視化する。

**Architecture:** 既存 `GameTracker` インターフェース不変、SF6 専用機能として `SF6Tracker.PollPlayStats` を追加。`cmd.TrackingHandler` から型アサーションで呼び出し、新規 `play_stats_snapshots` テーブルへ保存。GUI は新ページ `/stats` で `recharts` の折れ線 + KPI カード + 詳細展開テーブル。

**Tech Stack:** Go 1.25 / Wails v2 / modernc.org/sqlite / golang-migrate / go-rod / React 19 / TypeScript 6 / Tailwind v4 / XState v5 / recharts

**Spec:** `specs/2026-05-22-play-stats-phase-a-design.md`

---

## File Map

### 新規作成

| パス | 責務 |
|---|---|
| `app/pkg/storage/sql/migrations/000004_add_play_stats_snapshots.up.sql` | テーブル + INDEX 作成 |
| `app/pkg/storage/sql/migrations/000004_add_play_stats_snapshots.down.sql` | テーブル + INDEX 削除 |
| `app/pkg/tracker/sf6/cfn/testdata/play-stats-sample.json` | `__NEXT_DATA__` テスト用フィクスチャ (`tools/dump-play-stats` 出力から `play` セクションを切り出し) |
| `app/pkg/tracker/sf6/cfn/model_test.go` | `PlayPage` / `BattleStats` / `BaseInfo` の JSON unmarshal テスト |
| `app/pkg/model/play_stats.go` | DB マッピング `PlayStatsSnapshot` + 派生型 `MatchWithStats` |
| `app/pkg/storage/sql/play_stats.go` | `SavePlayStats` / `GetPlayStatsHistory` / `GetPlayStatsCharacters` / `GetMatchesWithPlayStats` |
| `app/pkg/storage/sql/play_stats_test.go` | 上記 4 メソッドのラウンドトリップテスト (in-memory SQLite + TestMain) |
| `app/gui/src/pages/stats.tsx` | `/stats` ダッシュボードページ |
| `app/gui/src/pages/stats/kpi-card.tsx` | KPI カードコンポーネント |
| `app/gui/src/pages/stats/trend-chart.tsx` | recharts ベース折れ線グラフ |
| `app/gui/src/pages/stats/detail-table.tsx` | matches JOIN スナップショット詳細展開 |
| `app/gui/src/pages/stats/formatters.ts` | 数値フォーマッタ (比率 → %, 秒 → `1h 23m` 等) |

### 修正

| パス | 内容 |
|---|---|
| `app/pkg/tracker/sf6/cfn/model.go` | `PlayPage` / `BattleStats` / `BaseInfo` / `ContentPlayTime` を追加 |
| `app/pkg/tracker/sf6/cfn/client.go` | `GetPlayStats(ctx, cfn) (*PlayPage, error)` を追加 |
| `app/pkg/tracker/sf6/track.go` | `PlayStatsResult` 型と `PollPlayStats` メソッドを追加 |
| `app/cmd/tracking.go` | `StartTracking` に baseline + per-match スナップショット保存を追加 |
| `app/cmd/cmd.go` | `GetPlayStatsHistory` / `GetPlayStatsCharacters` / `GetMatchesWithPlayStats` を追加 |
| `app/pkg/model/i18n.go` | `Localization` 構造体に新規キー追加 |
| `app/pkg/i18n/locales/{en-GB,ja-JP,fr-FR}.json` | 新規キーを 3 言語で追加 |
| `app/gui/src/main/router.tsx` | `/stats` ルート追加 |
| `app/gui/src/main/app-sidebar.tsx` | サイドバーに `stats` 項目追加 (常時表示) |
| `app/gui/package.json` | `recharts` を依存追加 |

---

## Task 1: Add SQL migration for play_stats_snapshots

**Files:**
- Create: `app/pkg/storage/sql/migrations/000004_add_play_stats_snapshots.up.sql`
- Create: `app/pkg/storage/sql/migrations/000004_add_play_stats_snapshots.down.sql`

- [ ] **Step 1: Write the UP migration**

`app/pkg/storage/sql/migrations/000004_add_play_stats_snapshots.up.sql`:

```sql
CREATE TABLE IF NOT EXISTS play_stats_snapshots (
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

CREATE INDEX IF NOT EXISTS idx_play_stats_user_char_at
  ON play_stats_snapshots(user_id, character, snapshot_at);

CREATE INDEX IF NOT EXISTS idx_play_stats_match_replay_id
  ON play_stats_snapshots(match_replay_id);
```

- [ ] **Step 2: Write the DOWN migration**

`app/pkg/storage/sql/migrations/000004_add_play_stats_snapshots.down.sql`:

```sql
DROP INDEX IF EXISTS idx_play_stats_match_replay_id;
DROP INDEX IF EXISTS idx_play_stats_user_char_at;
DROP TABLE IF EXISTS play_stats_snapshots;
```

- [ ] **Step 3: Verify migration applies**

```sh
cd app
mise exec -- go build ./...
```

Expected: build succeeds (migration is embedded but only applied on `NewStorage()` call).

Optional manual verification: stop `task dev-hard` if running, then:

```sh
mise exec -- go test -run TestMain ./cmd -v 2>&1 | head -5
```

Expected: `TestMain` succeeds (it calls `sql.NewStorage(true)` which applies migrations to in-memory DB).

- [ ] **Step 4: Commit**

```sh
git add app/pkg/storage/sql/migrations/000004_add_play_stats_snapshots.up.sql \
        app/pkg/storage/sql/migrations/000004_add_play_stats_snapshots.down.sql
git commit -m "feat(stats): add play_stats_snapshots schema migration"
```

---

## Task 2: Add Go structs for PlayPage JSON

**Files:**
- Modify: `app/pkg/tracker/sf6/cfn/model.go`
- Create: `app/pkg/tracker/sf6/cfn/testdata/play-stats-sample.json`
- Create: `app/pkg/tracker/sf6/cfn/model_test.go`

- [ ] **Step 1: Extract test fixture from dump output**

If `play-stats-sample.json` is not present at repo root (it's gitignored), regenerate it first:

```sh
cd app
mise exec -- go run ./tools/dump-play-stats -cfn 1766731922 -out ../play-stats-sample.json
```

Then copy the relevant `play` section into the testdata fixture:

```sh
cd /Users/ryugo/Developer/src/personal/cfn-tracker
mkdir -p app/pkg/tracker/sf6/cfn/testdata
mise exec -- bunx --bun jq '.props.pageProps' play-stats-sample.json > app/pkg/tracker/sf6/cfn/testdata/play-stats-sample.json
```

This produces a fixture containing `common`, `fighter_banner_info`, and `play` so tests can assert against the full unmarshal path.

- [ ] **Step 2: Write failing unmarshal test**

`app/pkg/tracker/sf6/cfn/model_test.go`:

```go
package cfn

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPlayPageUnmarshal(t *testing.T) {
	data, err := os.ReadFile("testdata/play-stats-sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var pp PlayPageProps
	if err := json.Unmarshal(data, &pp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if pp.Common.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", pp.Common.StatusCode)
	}
	if pp.Play.BattleStats.DriveImpact <= 0 {
		t.Errorf("BattleStats.DriveImpact = %v, want > 0", pp.Play.BattleStats.DriveImpact)
	}
	if got := pp.Play.BattleStats.GaugeRateSALv1 + pp.Play.BattleStats.GaugeRateSALv2 +
		pp.Play.BattleStats.GaugeRateSALv3 + pp.Play.BattleStats.GaugeRateCA; got <= 0 {
		t.Errorf("SA gauge rates sum = %v, want > 0", got)
	}
	if len(pp.Play.BaseInfo.ContentPlayTimeList) != 9 {
		t.Errorf("ContentPlayTimeList len = %d, want 9", len(pp.Play.BaseInfo.ContentPlayTimeList))
	}
	if pp.FighterBannerInfo.FavoriteCharacterName == "" {
		t.Errorf("FavoriteCharacterName is empty")
	}
}
```

- [ ] **Step 3: Run test to see it fail**

```sh
cd app
mise exec -- go test ./pkg/tracker/sf6/cfn/ -run TestPlayPageUnmarshal -v
```

Expected: FAIL with `undefined: PlayPageProps` (or similar).

- [ ] **Step 4: Add struct definitions to model.go**

Append to `app/pkg/tracker/sf6/cfn/model.go` (end of file, after `SearchResult`):

```go
// PlayPageProps mirrors the props.pageProps shape of the
// /buckler/profile/<code>/play page. It is shared with BattleLog only at the
// outer common / fighter_banner_info layer.
type PlayPageProps struct {
	Common            CommonProps `json:"common"`
	FighterBannerInfo struct {
		FavoriteCharacterID       int    `json:"favorite_character_id"`
		FavoriteCharacterName     string `json:"favorite_character_name"`
		FavoriteCharacterToolName string `json:"favorite_character_tool_name"`
		PersonalInfo              struct {
			FighterID string `json:"fighter_id"`
			ShortID   int64  `json:"short_id"`
		} `json:"personal_info"`
	} `json:"fighter_banner_info"`
	Play PlayProps `json:"play"`
}

// PlayPageDoc is the full __NEXT_DATA__ root for /play.
type PlayPageDoc struct {
	Props struct {
		PageProps PlayPageProps `json:"pageProps"`
	} `json:"props"`
}

type CommonProps struct {
	StatusCode int  `json:"statusCode"`
	IsError    bool `json:"isError"`
}

type PlayProps struct {
	BattleStats BattleStats `json:"battle_stats"`
	BaseInfo    BaseInfo    `json:"base_info"`
}

// BattleStats is the over-last-100-ranked-matches summary for the player's
// favorite character. Counters with float types are averages per match; *_play_count
// are cumulative totals; gauge_rate_* sum to 1.0 within each gauge.
type BattleStats struct {
	BattleHubMatchPlayCount         int     `json:"battle_hub_match_play_count"`
	CasualMatchPlayCount            int     `json:"casual_match_play_count"`
	CornerTime                      int     `json:"corner_time"`
	CorneredTime                    int     `json:"cornered_time"`
	CustomRoomMatchPlayCount        int     `json:"custom_room_match_play_count"`
	DriveImpact                     float64 `json:"drive_impact"`
	DriveImpactToDriveImpact        float64 `json:"drive_impact_to_drive_impact"`
	DriveParry                      float64 `json:"drive_parry"`
	DriveReversal                   float64 `json:"drive_reversal"`
	GaugeRateCA                     float64 `json:"gauge_rate_ca"`
	GaugeRateDriveArts              float64 `json:"gauge_rate_drive_arts"`
	GaugeRateDriveGuard             float64 `json:"gauge_rate_drive_guard"`
	GaugeRateDriveImpact            float64 `json:"gauge_rate_drive_impact"`
	GaugeRateDriveOther             float64 `json:"gauge_rate_drive_other"`
	GaugeRateDriveReversal          float64 `json:"gauge_rate_drive_reversal"`
	GaugeRateDriveRushFromCancel    float64 `json:"gauge_rate_drive_rush_from_cancel"`
	GaugeRateDriveRushFromParry     float64 `json:"gauge_rate_drive_rush_from_parry"`
	GaugeRateSALv1                  float64 `json:"gauge_rate_sa_lv1"`
	GaugeRateSALv2                  float64 `json:"gauge_rate_sa_lv2"`
	GaugeRateSALv3                  float64 `json:"gauge_rate_sa_lv3"`
	JustParry                       float64 `json:"just_parry"`
	PunishCounter                   float64 `json:"punish_counter"`
	RankMatchPlayCount              int     `json:"rank_match_play_count"`
	ReceivedDriveImpact             float64 `json:"received_drive_impact"`
	ReceivedDriveImpactToDriveImpact float64 `json:"received_drive_impact_to_drive_impact"`
	ReceivedPunishCounter           float64 `json:"received_punish_counter"`
	ReceivedStun                    float64 `json:"received_stun"`
	ReceivedThrowCount              float64 `json:"received_throw_count"`
	ReceivedThrowDriveParry         float64 `json:"received_throw_drive_parry"`
	RivalAIAchievedChallengeCount   int     `json:"rival_ai_achieved_challenge_count"`
	RivalAIHighestLeagueRank        int     `json:"rival_ai_highest_league_rank"`
	RivalAIHighestLeagueRankTxt     string  `json:"rival_ai_highest_league_rank_txt"`
	Stun                            float64 `json:"stun"`
	TargetClearCount                int     `json:"target_clear_count"`
	ThrowCount                      float64 `json:"throw_count"`
	ThrowDriveParry                 float64 `json:"throw_drive_parry"`
	ThrowTech                       float64 `json:"throw_tech"`
	TotalAllCharacterPlayPoint      int     `json:"total_all_character_play_point"`
}

type BaseInfo struct {
	ContentPlayTimeList []ContentPlayTime `json:"content_play_time_list"`
	EnjoyFightPoint     int               `json:"enjoy_fight_point"`
	EnjoyTotalPoint     int               `json:"enjoy_total_point"`
	EnjoyUserPoint      int               `json:"enjoy_user_point"`
}

type ContentPlayTime struct {
	ContentType     int    `json:"content_type"`
	ContentTypeName string `json:"content_type_name"`
	PlayTime        int    `json:"play_time"` // seconds, cumulative
}

// ContentType IDs are stable enum values from Capcom.
const (
	ContentTypeWorldTour      = 1
	ContentTypeRankedMatch    = 2
	ContentTypeCasualMatch    = 3
	ContentTypeCustomRoom     = 4
	ContentTypeBattleHub      = 5
	ContentTypeOfflineMatch   = 6
	ContentTypeArcade         = 7
	ContentTypePractice       = 8
	ContentTypeExtreme        = 9
)
```

- [ ] **Step 5: Run test to see it pass**

```sh
mise exec -- go test ./pkg/tracker/sf6/cfn/ -run TestPlayPageUnmarshal -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```sh
git add app/pkg/tracker/sf6/cfn/model.go \
        app/pkg/tracker/sf6/cfn/model_test.go \
        app/pkg/tracker/sf6/cfn/testdata/play-stats-sample.json
git commit -m "feat(stats): add Go types for /play page __NEXT_DATA__ shape"
```

---

## Task 3: Add cfn.Client.GetPlayStats

**Files:**
- Modify: `app/pkg/tracker/sf6/cfn/client.go`

`GetPlayStats` reuses the same rod page + `#__NEXT_DATA__` extraction pattern as `GetBattleLog`. Unit-testing the rod path is impractical, so this task relies on the unmarshal coverage from Task 2 plus end-to-end runtime exercise in Task 18.

- [ ] **Step 1: Extend CFNClient interface**

In `app/pkg/tracker/sf6/cfn/client.go`, modify the `CFNClient` interface block (around line 18):

```go
type CFNClient interface {
	GetBattleLog(ctx context.Context, cfn string) (*BattleLog, error)
	GetPlayStats(ctx context.Context, cfn string) (*PlayPageProps, error)
	Authenticate(ctx context.Context, email string, password string, statChan chan tracker.AuthStatus)
}
```

- [ ] **Step 2: Implement GetPlayStats**

Append to `app/pkg/tracker/sf6/cfn/client.go` (after `GetBattleLog`, before `Authenticate`):

```go
func (c *Client) GetPlayStats(ctx context.Context, cfn string) (*PlayPageProps, error) {
	page := c.browser.Page.Context(ctx)
	err := page.Navigate(fmt.Sprintf("https://www.streetfighter.com/6/buckler/profile/%s/play", cfn))
	if err != nil {
		return nil, fmt.Errorf("navigate to play page: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait for play page to load: %w", err)
	}
	nextData, err := page.Element("#__NEXT_DATA__")
	if err != nil {
		return nil, fmt.Errorf("get __NEXT_DATA__ element: %w", err)
	}
	body, err := nextData.Text()
	if err != nil {
		return nil, fmt.Errorf("read __NEXT_DATA__ json: %w", err)
	}

	var doc PlayPageDoc
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return nil, fmt.Errorf("unmarshal play page: %w", err)
	}

	pp := &doc.Props.PageProps
	if pp.Common.StatusCode != 200 {
		return nil, fmt.Errorf("fetch play page, received status code %v", pp.Common.StatusCode)
	}
	return pp, nil
}
```

- [ ] **Step 3: Verify build**

```sh
cd app
mise exec -- go build ./...
```

Expected: success.

- [ ] **Step 4: Run existing tests to verify no regression**

```sh
mise exec -- go test ./...
```

Expected: PASS (Task 2 test + existing `cmd_test`).

- [ ] **Step 5: Commit**

```sh
git add app/pkg/tracker/sf6/cfn/client.go
git commit -m "feat(stats): add CFNClient.GetPlayStats for /play scraping"
```

---

## Task 4: Add PlayStatsSnapshot DB model

**Files:**
- Create: `app/pkg/model/play_stats.go`

- [ ] **Step 1: Create the DB-mapped struct**

`app/pkg/model/play_stats.go`:

```go
package model

import "database/sql"

// PlayStatsSnapshot is one row of the play_stats_snapshots table. Field tags
// align with both DB column names (db:) and JSON keys emitted to the
// frontend (json:). MatchReplayId is nullable because the baseline snapshot
// captured at tracking start has no associated match yet.
type PlayStatsSnapshot struct {
	Id            int64          `db:"id" json:"id"`
	UserId        string         `db:"user_id" json:"userId"`
	Character     string         `db:"character" json:"character"`
	MatchReplayId sql.NullString `db:"match_replay_id" json:"matchReplayId"`
	SnapshotAt    string         `db:"snapshot_at" json:"snapshotAt"`

	// battle_stats
	BattleHubMatchPlayCount          int     `db:"battle_hub_match_play_count" json:"battleHubMatchPlayCount"`
	CasualMatchPlayCount             int     `db:"casual_match_play_count" json:"casualMatchPlayCount"`
	CornerTime                       int     `db:"corner_time" json:"cornerTime"`
	CorneredTime                     int     `db:"cornered_time" json:"corneredTime"`
	CustomRoomMatchPlayCount         int     `db:"custom_room_match_play_count" json:"customRoomMatchPlayCount"`
	DriveImpact                      float64 `db:"drive_impact" json:"driveImpact"`
	DriveImpactToDriveImpact         float64 `db:"drive_impact_to_drive_impact" json:"driveImpactToDriveImpact"`
	DriveParry                       float64 `db:"drive_parry" json:"driveParry"`
	DriveReversal                    float64 `db:"drive_reversal" json:"driveReversal"`
	GaugeRateCA                      float64 `db:"gauge_rate_ca" json:"gaugeRateCA"`
	GaugeRateDriveArts               float64 `db:"gauge_rate_drive_arts" json:"gaugeRateDriveArts"`
	GaugeRateDriveGuard              float64 `db:"gauge_rate_drive_guard" json:"gaugeRateDriveGuard"`
	GaugeRateDriveImpact             float64 `db:"gauge_rate_drive_impact" json:"gaugeRateDriveImpact"`
	GaugeRateDriveOther              float64 `db:"gauge_rate_drive_other" json:"gaugeRateDriveOther"`
	GaugeRateDriveReversal           float64 `db:"gauge_rate_drive_reversal" json:"gaugeRateDriveReversal"`
	GaugeRateDriveRushFromCancel     float64 `db:"gauge_rate_drive_rush_from_cancel" json:"gaugeRateDriveRushFromCancel"`
	GaugeRateDriveRushFromParry      float64 `db:"gauge_rate_drive_rush_from_parry" json:"gaugeRateDriveRushFromParry"`
	GaugeRateSALv1                   float64 `db:"gauge_rate_sa_lv1" json:"gaugeRateSALv1"`
	GaugeRateSALv2                   float64 `db:"gauge_rate_sa_lv2" json:"gaugeRateSALv2"`
	GaugeRateSALv3                   float64 `db:"gauge_rate_sa_lv3" json:"gaugeRateSALv3"`
	JustParry                        float64 `db:"just_parry" json:"justParry"`
	PunishCounter                    float64 `db:"punish_counter" json:"punishCounter"`
	RankMatchPlayCount               int     `db:"rank_match_play_count" json:"rankMatchPlayCount"`
	ReceivedDriveImpact              float64 `db:"received_drive_impact" json:"receivedDriveImpact"`
	ReceivedDriveImpactToDriveImpact float64 `db:"received_drive_impact_to_drive_impact" json:"receivedDriveImpactToDriveImpact"`
	ReceivedPunishCounter            float64 `db:"received_punish_counter" json:"receivedPunishCounter"`
	ReceivedStun                     float64 `db:"received_stun" json:"receivedStun"`
	ReceivedThrowCount               float64 `db:"received_throw_count" json:"receivedThrowCount"`
	ReceivedThrowDriveParry          float64 `db:"received_throw_drive_parry" json:"receivedThrowDriveParry"`
	RivalAIAchievedChallengeCount    int     `db:"rival_ai_achieved_challenge_count" json:"rivalAIAchievedChallengeCount"`
	RivalAIHighestLeagueRank         int     `db:"rival_ai_highest_league_rank" json:"rivalAIHighestLeagueRank"`
	RivalAIHighestLeagueRankTxt      string  `db:"rival_ai_highest_league_rank_txt" json:"rivalAIHighestLeagueRankTxt"`
	Stun                             float64 `db:"stun" json:"stun"`
	TargetClearCount                 int     `db:"target_clear_count" json:"targetClearCount"`
	ThrowCount                       float64 `db:"throw_count" json:"throwCount"`
	ThrowDriveParry                  float64 `db:"throw_drive_parry" json:"throwDriveParry"`
	ThrowTech                        float64 `db:"throw_tech" json:"throwTech"`
	TotalAllCharacterPlayPoint       int     `db:"total_all_character_play_point" json:"totalAllCharacterPlayPoint"`

	// base_info enjoy
	EnjoyFightPoint int `db:"enjoy_fight_point" json:"enjoyFightPoint"`
	EnjoyTotalPoint int `db:"enjoy_total_point" json:"enjoyTotalPoint"`
	EnjoyUserPoint  int `db:"enjoy_user_point" json:"enjoyUserPoint"`

	// base_info content_play_time_list (seconds per content_type)
	WorldTourSeconds     int `db:"world_tour_seconds" json:"worldTourSeconds"`
	RankedMatchSeconds   int `db:"ranked_match_seconds" json:"rankedMatchSeconds"`
	CasualMatchSeconds   int `db:"casual_match_seconds" json:"casualMatchSeconds"`
	CustomRoomSeconds    int `db:"custom_room_seconds" json:"customRoomSeconds"`
	BattleHubSeconds     int `db:"battle_hub_seconds" json:"battleHubSeconds"`
	OfflineMatchSeconds  int `db:"offline_match_seconds" json:"offlineMatchSeconds"`
	ArcadeSeconds        int `db:"arcade_seconds" json:"arcadeSeconds"`
	PracticeSeconds      int `db:"practice_seconds" json:"practiceSeconds"`
	ExtremeSeconds       int `db:"extreme_seconds" json:"extremeSeconds"`
}

// MatchWithStats joins one Match row with its corresponding play stats
// snapshot (if any). Stats is nil when no snapshot exists for the match.
type MatchWithStats struct {
	Match Match              `json:"match"`
	Stats *PlayStatsSnapshot `json:"stats"`
}
```

- [ ] **Step 2: Verify build**

```sh
cd app
mise exec -- go build ./...
```

Expected: success.

- [ ] **Step 3: Commit**

```sh
git add app/pkg/model/play_stats.go
git commit -m "feat(stats): add PlayStatsSnapshot DB model"
```

---

## Task 5: SavePlayStats SQL storage + test

**Files:**
- Create: `app/pkg/storage/sql/play_stats.go`
- Create: `app/pkg/storage/sql/play_stats_test.go`

- [ ] **Step 1: Write failing roundtrip test**

`app/pkg/storage/sql/play_stats_test.go`:

```go
package sql_test

import (
	"context"
	"database/sql"
	"log"
	"os"
	"testing"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	sqlstore "github.com/williamsjokvist/cfn-tracker/pkg/storage/sql"
)

var store *sqlstore.Storage

func TestMain(m *testing.M) {
	s, err := sqlstore.NewStorage(true)
	if err != nil {
		log.Fatalf("init sql storage: %v", err)
	}
	store = s
	os.Exit(m.Run())
}

func sampleSnapshot(userId, character, replayId string) model.PlayStatsSnapshot {
	snap := model.PlayStatsSnapshot{
		UserId:    userId,
		Character: character,
		DriveImpact: 1.2,
		ReceivedDriveImpact: 1.9,
		JustParry: 0,
		ThrowTech: 0.1,
		CornerTime: 3,
		CorneredTime: 10,
		GaugeRateSALv3: 0.2786,
		RankMatchPlayCount: 59,
		WorldTourSeconds: 59936,
		RankedMatchSeconds: 4692,
		PracticeSeconds: 159402,
		RivalAIHighestLeagueRankTxt: "Rookie 2",
	}
	if replayId != "" {
		snap.MatchReplayId = sql.NullString{String: replayId, Valid: true}
	}
	return snap
}

func TestSavePlayStatsAndRoundtrip(t *testing.T) {
	ctx := context.Background()
	in := sampleSnapshot("user-1", "JP", "replay-A")

	if err := store.SavePlayStats(ctx, in); err != nil {
		t.Fatalf("SavePlayStats: %v", err)
	}

	got, err := store.GetPlayStatsHistory(ctx, "user-1", "JP", "", "", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].DriveImpact != 1.2 {
		t.Errorf("DriveImpact = %v, want 1.2", got[0].DriveImpact)
	}
	if got[0].MatchReplayId.String != "replay-A" {
		t.Errorf("MatchReplayId = %q, want replay-A", got[0].MatchReplayId.String)
	}
	if got[0].RivalAIHighestLeagueRankTxt != "Rookie 2" {
		t.Errorf("RivalAIHighestLeagueRankTxt = %q", got[0].RivalAIHighestLeagueRankTxt)
	}
	if got[0].WorldTourSeconds != 59936 {
		t.Errorf("WorldTourSeconds = %d", got[0].WorldTourSeconds)
	}
}

func TestSaveBaselineSnapshot(t *testing.T) {
	ctx := context.Background()
	in := sampleSnapshot("user-2", "Ken", "")

	if err := store.SavePlayStats(ctx, in); err != nil {
		t.Fatalf("SavePlayStats: %v", err)
	}

	got, err := store.GetPlayStatsHistory(ctx, "user-2", "Ken", "", "", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 baseline row, got %d", len(got))
	}
	if got[0].MatchReplayId.Valid {
		t.Errorf("baseline snapshot should have NULL match_replay_id, got %q", got[0].MatchReplayId.String)
	}
}
```

- [ ] **Step 2: Run test to see it fail**

```sh
cd app
mise exec -- go test ./pkg/storage/sql/ -run TestSavePlayStats -v
```

Expected: FAIL (`SavePlayStats undefined` / `GetPlayStatsHistory undefined`).

- [ ] **Step 3: Implement SavePlayStats and GetPlayStatsHistory**

`app/pkg/storage/sql/play_stats.go`:

```go
package sql

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

const playStatsInsertColumns = `
	user_id, character, match_replay_id,
	battle_hub_match_play_count, casual_match_play_count, corner_time, cornered_time,
	custom_room_match_play_count, drive_impact, drive_impact_to_drive_impact,
	drive_parry, drive_reversal,
	gauge_rate_ca, gauge_rate_drive_arts, gauge_rate_drive_guard, gauge_rate_drive_impact,
	gauge_rate_drive_other, gauge_rate_drive_reversal,
	gauge_rate_drive_rush_from_cancel, gauge_rate_drive_rush_from_parry,
	gauge_rate_sa_lv1, gauge_rate_sa_lv2, gauge_rate_sa_lv3,
	just_parry, punish_counter, rank_match_play_count,
	received_drive_impact, received_drive_impact_to_drive_impact,
	received_punish_counter, received_stun, received_throw_count, received_throw_drive_parry,
	rival_ai_achieved_challenge_count, rival_ai_highest_league_rank, rival_ai_highest_league_rank_txt,
	stun, target_clear_count,
	throw_count, throw_drive_parry, throw_tech,
	total_all_character_play_point,
	enjoy_fight_point, enjoy_total_point, enjoy_user_point,
	world_tour_seconds, ranked_match_seconds, casual_match_seconds, custom_room_seconds,
	battle_hub_seconds, offline_match_seconds, arcade_seconds, practice_seconds, extreme_seconds
`

const playStatsInsertValues = `
	:user_id, :character, :match_replay_id,
	:battle_hub_match_play_count, :casual_match_play_count, :corner_time, :cornered_time,
	:custom_room_match_play_count, :drive_impact, :drive_impact_to_drive_impact,
	:drive_parry, :drive_reversal,
	:gauge_rate_ca, :gauge_rate_drive_arts, :gauge_rate_drive_guard, :gauge_rate_drive_impact,
	:gauge_rate_drive_other, :gauge_rate_drive_reversal,
	:gauge_rate_drive_rush_from_cancel, :gauge_rate_drive_rush_from_parry,
	:gauge_rate_sa_lv1, :gauge_rate_sa_lv2, :gauge_rate_sa_lv3,
	:just_parry, :punish_counter, :rank_match_play_count,
	:received_drive_impact, :received_drive_impact_to_drive_impact,
	:received_punish_counter, :received_stun, :received_throw_count, :received_throw_drive_parry,
	:rival_ai_achieved_challenge_count, :rival_ai_highest_league_rank, :rival_ai_highest_league_rank_txt,
	:stun, :target_clear_count,
	:throw_count, :throw_drive_parry, :throw_tech,
	:total_all_character_play_point,
	:enjoy_fight_point, :enjoy_total_point, :enjoy_user_point,
	:world_tour_seconds, :ranked_match_seconds, :casual_match_seconds, :custom_room_seconds,
	:battle_hub_seconds, :offline_match_seconds, :arcade_seconds, :practice_seconds, :extreme_seconds
`

func (s *Storage) SavePlayStats(ctx context.Context, snap model.PlayStatsSnapshot) error {
	query := fmt.Sprintf(
		`INSERT INTO play_stats_snapshots (%s) VALUES (%s)`,
		strings.TrimSpace(playStatsInsertColumns),
		strings.TrimSpace(playStatsInsertValues),
	)
	if _, err := s.db.NamedExecContext(ctx, query, snap); err != nil {
		return fmt.Errorf("insert play stats snapshot: %w", err)
	}
	return nil
}

func (s *Storage) GetPlayStatsHistory(
	ctx context.Context,
	userId, character, from, to string,
	limit uint16,
) ([]*model.PlayStatsSnapshot, error) {
	wheres := []string{"user_id = ?", "character = ?"}
	args := []interface{}{userId, character}
	if from != "" {
		wheres = append(wheres, "DATE(snapshot_at) >= ?")
		args = append(args, from)
	}
	if to != "" {
		wheres = append(wheres, "DATE(snapshot_at) <= ?")
		args = append(args, to)
	}
	limitClause := ""
	if limit > 0 {
		limitClause = fmt.Sprintf("LIMIT %d", limit)
	}
	query, args, err := sqlx.In(fmt.Sprintf(`
		SELECT * FROM play_stats_snapshots
		WHERE %s
		ORDER BY snapshot_at ASC
		%s
	`, strings.Join(wheres, " AND "), limitClause), args...)
	if err != nil {
		return nil, fmt.Errorf("prepare play stats query: %w", err)
	}
	var rows []*model.PlayStatsSnapshot
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("execute play stats query: %w", err)
	}
	return rows, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

```sh
mise exec -- go test ./pkg/storage/sql/ -run TestSavePlayStats -v
```

Expected: PASS for both `TestSavePlayStatsAndRoundtrip` and `TestSaveBaselineSnapshot`.

- [ ] **Step 5: Commit**

```sh
git add app/pkg/storage/sql/play_stats.go app/pkg/storage/sql/play_stats_test.go
git commit -m "feat(stats): add SavePlayStats and GetPlayStatsHistory storage"
```

---

## Task 6: GetPlayStatsCharacters

**Files:**
- Modify: `app/pkg/storage/sql/play_stats.go`
- Modify: `app/pkg/storage/sql/play_stats_test.go`

- [ ] **Step 1: Write failing test**

Append to `app/pkg/storage/sql/play_stats_test.go`:

```go
func TestGetPlayStatsCharacters(t *testing.T) {
	ctx := context.Background()
	_ = store.SavePlayStats(ctx, sampleSnapshot("user-3", "JP", ""))
	_ = store.SavePlayStats(ctx, sampleSnapshot("user-3", "Ken", "rA"))
	_ = store.SavePlayStats(ctx, sampleSnapshot("user-3", "Ken", "rB"))
	_ = store.SavePlayStats(ctx, sampleSnapshot("user-4", "Ryu", ""))

	got, err := store.GetPlayStatsCharacters(ctx, "user-3")
	if err != nil {
		t.Fatalf("GetPlayStatsCharacters: %v", err)
	}
	want := map[string]bool{"JP": true, "Ken": true}
	if len(got) != len(want) {
		t.Fatalf("character count = %d, want %d (got %v)", len(got), len(want), got)
	}
	for _, c := range got {
		if !want[c] {
			t.Errorf("unexpected character: %q", c)
		}
	}
}
```

- [ ] **Step 2: Run test to see it fail**

```sh
cd app
mise exec -- go test ./pkg/storage/sql/ -run TestGetPlayStatsCharacters -v
```

Expected: FAIL (`GetPlayStatsCharacters undefined`).

- [ ] **Step 3: Implement GetPlayStatsCharacters**

Append to `app/pkg/storage/sql/play_stats.go`:

```go
func (s *Storage) GetPlayStatsCharacters(ctx context.Context, userId string) ([]string, error) {
	query := `
		SELECT DISTINCT character
		FROM play_stats_snapshots
		WHERE user_id = ?
		ORDER BY character ASC
	`
	rows, err := s.db.QueryContext(ctx, query, userId)
	if err != nil {
		return nil, fmt.Errorf("execute distinct characters query: %w", err)
	}
	defer rows.Close()
	characters := make([]string, 0, 4)
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scan character row: %w", err)
		}
		characters = append(characters, c)
	}
	return characters, nil
}
```

- [ ] **Step 4: Run test to verify pass**

```sh
mise exec -- go test ./pkg/storage/sql/ -run TestGetPlayStatsCharacters -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```sh
git add app/pkg/storage/sql/play_stats.go app/pkg/storage/sql/play_stats_test.go
git commit -m "feat(stats): add GetPlayStatsCharacters distinct query"
```

---

## Task 7: GetMatchesWithPlayStats (LEFT JOIN)

**Files:**
- Modify: `app/pkg/storage/sql/play_stats.go`
- Modify: `app/pkg/storage/sql/play_stats_test.go`

- [ ] **Step 1: Write failing test**

Append to `app/pkg/storage/sql/play_stats_test.go`:

```go
func TestGetMatchesWithPlayStatsLeftJoin(t *testing.T) {
	ctx := context.Background()

	if err := store.SaveUser(ctx, model.User{Code: "user-5", DisplayName: "tester"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "user-5")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	matchWithStats := model.Match{
		UserId: "user-5", UserName: "tester", SessionId: sesh.Id,
		ReplayID: "replay-with-stats", Character: "JP",
		Date: "2026-05-22", Time: "10:00",
	}
	if err := store.SaveMatch(ctx, matchWithStats); err != nil {
		t.Fatalf("SaveMatch: %v", err)
	}
	matchNoStats := model.Match{
		UserId: "user-5", UserName: "tester", SessionId: sesh.Id,
		ReplayID: "replay-no-stats", Character: "JP",
		Date: "2026-05-22", Time: "10:30",
	}
	if err := store.SaveMatch(ctx, matchNoStats); err != nil {
		t.Fatalf("SaveMatch: %v", err)
	}
	if err := store.SavePlayStats(ctx, sampleSnapshot("user-5", "JP", "replay-with-stats")); err != nil {
		t.Fatalf("SavePlayStats: %v", err)
	}

	got, err := store.GetMatchesWithPlayStats(ctx, "user-5", "JP", 0, 0)
	if err != nil {
		t.Fatalf("GetMatchesWithPlayStats: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matches via LEFT JOIN, got %d", len(got))
	}

	byReplay := map[string]*model.MatchWithStats{}
	for _, m := range got {
		byReplay[m.Match.ReplayID] = m
	}
	if byReplay["replay-with-stats"].Stats == nil {
		t.Errorf("expected stats for replay-with-stats, got nil")
	}
	if byReplay["replay-no-stats"].Stats != nil {
		t.Errorf("expected nil stats for replay-no-stats, got %+v", byReplay["replay-no-stats"].Stats)
	}
}
```

- [ ] **Step 2: Run test to see it fail**

```sh
cd app
mise exec -- go test ./pkg/storage/sql/ -run TestGetMatchesWithPlayStatsLeftJoin -v
```

Expected: FAIL (`GetMatchesWithPlayStats undefined`).

- [ ] **Step 3: Implement GetMatchesWithPlayStats**

A denormalized one-shot LEFT JOIN would require mapping every nullable snapshot column manually (~50 lines of boilerplate). Instead, run **two queries** and merge in Go — simpler and equivalent for the row counts we expect (one user × one character × matches page).

Append to `app/pkg/storage/sql/play_stats.go`:

```go
func (s *Storage) GetMatchesWithPlayStats(
	ctx context.Context,
	userId, character string,
	limit uint8,
	offset uint16,
) ([]*model.MatchWithStats, error) {
	matches, err := s.GetMatches(ctx, 0, userId, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("get matches for stats join: %w", err)
	}
	// Narrow to the requested character on the matches side (the WHERE
	// belongs to m.character per spec §3 to preserve LEFT JOIN semantics).
	filtered := make([]*model.Match, 0, len(matches))
	for _, m := range matches {
		if m.Character == character {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) == 0 {
		return []*model.MatchWithStats{}, nil
	}

	replayIds := make([]string, 0, len(filtered))
	for _, m := range filtered {
		if m.ReplayID != "" {
			replayIds = append(replayIds, m.ReplayID)
		}
	}
	statsByReplay := map[string]*model.PlayStatsSnapshot{}
	if len(replayIds) > 0 {
		query, args, err := sqlx.In(`
			SELECT * FROM play_stats_snapshots
			WHERE user_id = ? AND character = ? AND match_replay_id IN (?)
		`, userId, character, replayIds)
		if err != nil {
			return nil, fmt.Errorf("prepare stats lookup: %w", err)
		}
		var rows []*model.PlayStatsSnapshot
		if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
			return nil, fmt.Errorf("fetch stats for matches: %w", err)
		}
		for _, r := range rows {
			if r.MatchReplayId.Valid {
				statsByReplay[r.MatchReplayId.String] = r
			}
		}
	}

	out := make([]*model.MatchWithStats, 0, len(filtered))
	for _, m := range filtered {
		entry := &model.MatchWithStats{Match: *m}
		if snap, ok := statsByReplay[m.ReplayID]; ok {
			entry.Stats = snap
		}
		out = append(out, entry)
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify pass**

```sh
mise exec -- go test ./pkg/storage/sql/ -run TestGetMatchesWithPlayStatsLeftJoin -v
```

Expected: PASS — both replay-with-stats and replay-no-stats present, only the first has non-nil Stats.

- [ ] **Step 5: Run all storage tests**

```sh
mise exec -- go test ./pkg/storage/sql/ -v
```

Expected: ALL PASS.

- [ ] **Step 6: Commit**

```sh
git add app/pkg/storage/sql/play_stats.go app/pkg/storage/sql/play_stats_test.go
git commit -m "feat(stats): add GetMatchesWithPlayStats (matches LEFT JOIN snapshots)"
```

---

## Task 8: SF6Tracker.PollPlayStats

**Files:**
- Modify: `app/pkg/tracker/sf6/track.go`

- [ ] **Step 1: Add PlayStatsResult type and PollPlayStats method**

Append to `app/pkg/tracker/sf6/track.go` (after `Authenticate`):

```go
// PlayStatsResult bundles the parsed /play data for storage. Character is
// the display name (FavoriteCharacterName) to match the existing
// matches.character column.
type PlayStatsResult struct {
	Character string
	Stats     *cfn.BattleStats
	BaseInfo  *cfn.BaseInfo
}

// PollPlayStats fetches a single /play snapshot for the given CFN user code.
// It is intentionally not part of the tracker.GameTracker interface — only
// SF6 has this concept — so callers must type-assert to *SF6Tracker.
func (t *SF6Tracker) PollPlayStats(ctx context.Context, userCode string) (*PlayStatsResult, error) {
	pp, err := t.cfnClient.GetPlayStats(ctx, userCode)
	if err != nil {
		return nil, fmt.Errorf("cfn: get play stats: %w", err)
	}
	return &PlayStatsResult{
		Character: pp.FighterBannerInfo.FavoriteCharacterName,
		Stats:     &pp.Play.BattleStats,
		BaseInfo:  &pp.Play.BaseInfo,
	}, nil
}
```

- [ ] **Step 2: Verify build**

```sh
cd app
mise exec -- go build ./...
```

Expected: success.

- [ ] **Step 3: Commit**

```sh
git add app/pkg/tracker/sf6/track.go
git commit -m "feat(stats): add SF6Tracker.PollPlayStats"
```

---

## Task 9: Wire play stats into TrackingHandler

**Files:**
- Modify: `app/cmd/tracking.go`

This task adds two snapshot capture points:

1. **Baseline** — at tracking start, if no recent snapshot exists for `(user_id, character)`
2. **Per-match** — after each successful match save, in the polling loop

- [ ] **Step 1: Add helper for snapshot persistence**

Add near the bottom of `app/cmd/tracking.go`:

```go
// playStatsSf6 captures a /play snapshot and saves it to SQLite. Returns
// nil error on success; warnings are emitted via slog on failure but never
// propagate so the surrounding match flow stays alive.
func (ch *TrackingHandler) playStatsSf6(
	ctx context.Context,
	userCode string,
	matchReplayId string,
) {
	sf6Tracker, ok := ch.gameTracker.(*sf6.SF6Tracker)
	if !ok {
		return
	}
	res, err := sf6Tracker.PollPlayStats(ctx, userCode)
	if err != nil {
		slog.Warn("get play stats failed, skipping snapshot", slog.Any("error", err))
		return
	}
	snap := buildSnapshot(userCode, res, matchReplayId)
	if err := ch.sqlDb.SavePlayStats(ctx, snap); err != nil {
		slog.Warn("save play stats failed", slog.Any("error", err))
	}
}

func buildSnapshot(userCode string, res *sf6.PlayStatsResult, matchReplayId string) model.PlayStatsSnapshot {
	snap := model.PlayStatsSnapshot{
		UserId:    userCode,
		Character: res.Character,
	}
	if matchReplayId != "" {
		snap.MatchReplayId = sql.NullString{String: matchReplayId, Valid: true}
	}
	if res.Stats != nil {
		bs := res.Stats
		snap.BattleHubMatchPlayCount = bs.BattleHubMatchPlayCount
		snap.CasualMatchPlayCount = bs.CasualMatchPlayCount
		snap.CornerTime = bs.CornerTime
		snap.CorneredTime = bs.CorneredTime
		snap.CustomRoomMatchPlayCount = bs.CustomRoomMatchPlayCount
		snap.DriveImpact = bs.DriveImpact
		snap.DriveImpactToDriveImpact = bs.DriveImpactToDriveImpact
		snap.DriveParry = bs.DriveParry
		snap.DriveReversal = bs.DriveReversal
		snap.GaugeRateCA = bs.GaugeRateCA
		snap.GaugeRateDriveArts = bs.GaugeRateDriveArts
		snap.GaugeRateDriveGuard = bs.GaugeRateDriveGuard
		snap.GaugeRateDriveImpact = bs.GaugeRateDriveImpact
		snap.GaugeRateDriveOther = bs.GaugeRateDriveOther
		snap.GaugeRateDriveReversal = bs.GaugeRateDriveReversal
		snap.GaugeRateDriveRushFromCancel = bs.GaugeRateDriveRushFromCancel
		snap.GaugeRateDriveRushFromParry = bs.GaugeRateDriveRushFromParry
		snap.GaugeRateSALv1 = bs.GaugeRateSALv1
		snap.GaugeRateSALv2 = bs.GaugeRateSALv2
		snap.GaugeRateSALv3 = bs.GaugeRateSALv3
		snap.JustParry = bs.JustParry
		snap.PunishCounter = bs.PunishCounter
		snap.RankMatchPlayCount = bs.RankMatchPlayCount
		snap.ReceivedDriveImpact = bs.ReceivedDriveImpact
		snap.ReceivedDriveImpactToDriveImpact = bs.ReceivedDriveImpactToDriveImpact
		snap.ReceivedPunishCounter = bs.ReceivedPunishCounter
		snap.ReceivedStun = bs.ReceivedStun
		snap.ReceivedThrowCount = bs.ReceivedThrowCount
		snap.ReceivedThrowDriveParry = bs.ReceivedThrowDriveParry
		snap.RivalAIAchievedChallengeCount = bs.RivalAIAchievedChallengeCount
		snap.RivalAIHighestLeagueRank = bs.RivalAIHighestLeagueRank
		snap.RivalAIHighestLeagueRankTxt = bs.RivalAIHighestLeagueRankTxt
		snap.Stun = bs.Stun
		snap.TargetClearCount = bs.TargetClearCount
		snap.ThrowCount = bs.ThrowCount
		snap.ThrowDriveParry = bs.ThrowDriveParry
		snap.ThrowTech = bs.ThrowTech
		snap.TotalAllCharacterPlayPoint = bs.TotalAllCharacterPlayPoint
	}
	if res.BaseInfo != nil {
		bi := res.BaseInfo
		snap.EnjoyFightPoint = bi.EnjoyFightPoint
		snap.EnjoyTotalPoint = bi.EnjoyTotalPoint
		snap.EnjoyUserPoint = bi.EnjoyUserPoint
		for _, cpt := range bi.ContentPlayTimeList {
			switch cpt.ContentType {
			case cfn.ContentTypeWorldTour:
				snap.WorldTourSeconds = cpt.PlayTime
			case cfn.ContentTypeRankedMatch:
				snap.RankedMatchSeconds = cpt.PlayTime
			case cfn.ContentTypeCasualMatch:
				snap.CasualMatchSeconds = cpt.PlayTime
			case cfn.ContentTypeCustomRoom:
				snap.CustomRoomSeconds = cpt.PlayTime
			case cfn.ContentTypeBattleHub:
				snap.BattleHubSeconds = cpt.PlayTime
			case cfn.ContentTypeOfflineMatch:
				snap.OfflineMatchSeconds = cpt.PlayTime
			case cfn.ContentTypeArcade:
				snap.ArcadeSeconds = cpt.PlayTime
			case cfn.ContentTypePractice:
				snap.PracticeSeconds = cpt.PlayTime
			case cfn.ContentTypeExtreme:
				snap.ExtremeSeconds = cpt.PlayTime
			}
		}
	}
	return snap
}
```

Add the needed imports to the top of `app/cmd/tracking.go`:

```go
import (
	// ... existing imports
	"database/sql"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/cfn"
)
```

- [ ] **Step 2: Add baseline capture in StartTracking**

Locate the existing block in `StartTracking` just after `session.UserName = user.DisplayName` (around line 99 in current code). Insert:

```go
// Baseline play stats snapshot (SF6 only). Skip if a recent snapshot
// already exists for the same (user, character) within 30 minutes —
// repeated stop/restart should not pile up empty match_replay_id NULL rows.
if sf6Tracker, ok := ch.gameTracker.(*sf6.SF6Tracker); ok {
	if res, err := sf6Tracker.PollPlayStats(ctx, user.Code); err == nil {
		recent, _ := ch.sqlDb.GetPlayStatsHistory(
			ctx, user.Code, res.Character,
			time.Now().Add(-30*time.Minute).Format("2006-01-02"),
			"", 1,
		)
		shouldSave := true
		if len(recent) > 0 {
			if parsed, parseErr := time.Parse("2006-01-02 15:04:05", recent[len(recent)-1].SnapshotAt); parseErr == nil &&
				time.Since(parsed) < 30*time.Minute {
				shouldSave = false
			}
		}
		if shouldSave {
			snap := buildSnapshot(user.Code, res, "")
			if err := ch.sqlDb.SavePlayStats(ctx, snap); err != nil {
				slog.Warn("save baseline play stats failed", slog.Any("error", err))
			}
		}
	} else {
		slog.Warn("baseline play stats fetch failed", slog.Any("error", err))
	}
}
```

- [ ] **Step 3: Add per-match capture inside the consume loop**

Locate the existing `for match := range matchChan` loop (around line 175). After the existing `ch.txtDb.SaveMatch(match)` line, append:

```go
		// Per-match play stats snapshot (SF6 only, best-effort)
		ch.playStatsSf6(ctx, session.UserId, match.ReplayID)
```

- [ ] **Step 4: Verify build**

```sh
cd app
mise exec -- go build ./...
```

Expected: success.

- [ ] **Step 5: Run existing tests**

```sh
mise exec -- go test ./...
```

Expected: PASS for `pkg/storage/sql/` (Tasks 5-7) and existing `cmd/` tests.

- [ ] **Step 6: Commit**

```sh
git add app/cmd/tracking.go
git commit -m "feat(stats): capture play stats baseline and per-match snapshots"
```

---

## Task 10: Add command-handler read APIs

**Files:**
- Modify: `app/cmd/cmd.go`

- [ ] **Step 1: Add three methods**

Append to `app/cmd/cmd.go` (before `GetFGCTrackerErrorModelUnused`):

```go
func (ch *CommandHandler) GetPlayStatsHistory(
	userId, character, from, to string,
	limit uint16,
) ([]*model.PlayStatsSnapshot, error) {
	rows, err := ch.sqlDb.GetPlayStatsHistory(context.Background(), userId, character, from, to, limit)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return rows, nil
}

func (ch *CommandHandler) GetPlayStatsCharacters(userId string) ([]string, error) {
	chars, err := ch.sqlDb.GetPlayStatsCharacters(context.Background(), userId)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return chars, nil
}

func (ch *CommandHandler) GetMatchesWithPlayStats(
	userId, character string,
	limit uint8,
	offset uint16,
) ([]*model.MatchWithStats, error) {
	rows, err := ch.sqlDb.GetMatchesWithPlayStats(context.Background(), userId, character, limit, offset)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return rows, nil
}
```

- [ ] **Step 2: Add the localization error key**

In `app/pkg/model/error.go`, add to the const block (after `tKeyErrReadThemeCSS`):

```go
	tKeyErrGetPlayStats ErrorLocalizationKey = "errGetPlayStats"
```

Add to the `AllErrorKeys` slice:

```go
	{tKeyErrGetPlayStats, string(tKeyErrGetPlayStats)},
```

Add to the var block at the bottom:

```go
	ErrGetPlayStats = newError(tKeyErrGetPlayStats, errors.New("get play stats"))
```

- [ ] **Step 3: Verify build**

```sh
cd app
mise exec -- go build ./...
```

Expected: success.

- [ ] **Step 4: Commit**

```sh
git add app/cmd/cmd.go app/pkg/model/error.go
git commit -m "feat(stats): expose play stats read APIs via CommandHandler"
```

---

## Task 11: Add i18n keys for new UI strings

**Files:**
- Modify: `app/pkg/model/i18n.go`
- Modify: `app/pkg/i18n/locales/en-GB.json`
- Modify: `app/pkg/i18n/locales/ja-JP.json`
- Modify: `app/pkg/i18n/locales/fr-FR.json`

- [ ] **Step 1: Add fields to Localization struct**

In `app/pkg/model/i18n.go`, add to the struct (alongside existing fields):

```go
	StatsNav            string `json:"statsNav"`
	StatsTitle          string `json:"statsTitle"`
	StatsPeriod7Days    string `json:"statsPeriod7Days"`
	StatsPeriod30Days   string `json:"statsPeriod30Days"`
	StatsPeriodAllTime  string `json:"statsPeriodAllTime"`
	StatsCharacter      string `json:"statsCharacter"`
	StatsUser           string `json:"statsUser"`
	StatsPreviousDelta  string `json:"statsPreviousDelta"`
	StatsTooltip        string `json:"statsTooltip"`
	StatsEmptyTracking  string `json:"statsEmptyTracking"`
	StatsSf6Only        string `json:"statsSf6Only"`
	StatsExpandDetails  string `json:"statsExpandDetails"`
	KpiDriveImpact      string `json:"kpiDriveImpact"`
	KpiReceivedDi       string `json:"kpiReceivedDi"`
	KpiJustParry        string `json:"kpiJustParry"`
	KpiThrowTech        string `json:"kpiThrowTech"`
	KpiCornerTime       string `json:"kpiCornerTime"`
	KpiSaLv3            string `json:"kpiSaLv3"`
	ErrGetPlayStats     string `json:"errGetPlayStats"`
```

Add the field name in the correct alphabetical neighborhood — match the convention of the existing struct.

- [ ] **Step 2: Add keys to en-GB.json**

Add to `app/pkg/i18n/locales/en-GB.json`:

```json
  "statsNav": "Play Stats",
  "statsTitle": "Play Stats Trend",
  "statsPeriod7Days": "Last 7 days",
  "statsPeriod30Days": "Last 30 days",
  "statsPeriodAllTime": "All time",
  "statsCharacter": "Character",
  "statsUser": "User",
  "statsPreviousDelta": "vs. previous",
  "statsTooltip": "Delta from previous 100-match average snapshot",
  "statsEmptyTracking": "No snapshots yet. They are recorded after each new match.",
  "statsSf6Only": "Play stats are an SF6-only feature. Track an SF6 user to see trends here.",
  "statsExpandDetails": "Show all fields",
  "kpiDriveImpact": "Drive Impact landed",
  "kpiReceivedDi": "Drive Impact taken",
  "kpiJustParry": "Just Parry",
  "kpiThrowTech": "Throw tech",
  "kpiCornerTime": "Corner pressure (sec)",
  "kpiSaLv3": "SA Lv3 usage",
  "errGetPlayStats": "Failed to get play stats",
```

- [ ] **Step 3: Add keys to ja-JP.json**

Add to `app/pkg/i18n/locales/ja-JP.json`:

```json
  "statsNav": "実績推移",
  "statsTitle": "実績推移",
  "statsPeriod7Days": "直近7日",
  "statsPeriod30Days": "直近30日",
  "statsPeriodAllTime": "全期間",
  "statsCharacter": "キャラクター",
  "statsUser": "ユーザー",
  "statsPreviousDelta": "前回比",
  "statsTooltip": "100戦平均の前回スナップショットからの差分",
  "statsEmptyTracking": "まだ記録がありません。新マッチごとに自動で記録されます。",
  "statsSf6Only": "実績推移はSF6専用機能です。SF6をトラッキングするとここに統計が表示されます。",
  "statsExpandDetails": "全項目を表示",
  "kpiDriveImpact": "DI命中",
  "kpiReceivedDi": "DI被弾",
  "kpiJustParry": "ジャストパリィ",
  "kpiThrowTech": "投げ抜け",
  "kpiCornerTime": "壁際追い詰め(秒)",
  "kpiSaLv3": "SA Lv3使用率",
  "errGetPlayStats": "実績データを取得できませんでした",
```

- [ ] **Step 4: Add keys to fr-FR.json**

Add to `app/pkg/i18n/locales/fr-FR.json`:

```json
  "statsNav": "Tendance",
  "statsTitle": "Tendance des statistiques",
  "statsPeriod7Days": "7 derniers jours",
  "statsPeriod30Days": "30 derniers jours",
  "statsPeriodAllTime": "Tout l'historique",
  "statsCharacter": "Personnage",
  "statsUser": "Utilisateur",
  "statsPreviousDelta": "vs précédent",
  "statsTooltip": "Différence par rapport au précédent instantané (moyenne sur 100 matchs)",
  "statsEmptyTracking": "Aucun instantané pour le moment. Ils sont enregistrés après chaque nouveau match.",
  "statsSf6Only": "Cette fonctionnalité est réservée à SF6. Suivez un joueur SF6 pour voir des tendances.",
  "statsExpandDetails": "Afficher tous les champs",
  "kpiDriveImpact": "Drive Impact réussis",
  "kpiReceivedDi": "Drive Impact subis",
  "kpiJustParry": "Just Parry",
  "kpiThrowTech": "Casser une projection",
  "kpiCornerTime": "Pression au mur (s)",
  "kpiSaLv3": "Usage SA Lv3",
  "errGetPlayStats": "Impossible de récupérer les statistiques",
```

- [ ] **Step 5: Verify build**

```sh
cd app
mise exec -- go build ./...
```

Expected: success (the i18n struct mapping is verified at runtime; build only confirms code compiles).

- [ ] **Step 6: Commit**

```sh
git add app/pkg/model/i18n.go \
        app/pkg/i18n/locales/en-GB.json \
        app/pkg/i18n/locales/ja-JP.json \
        app/pkg/i18n/locales/fr-FR.json
git commit -m "feat(stats): add i18n keys for stats UI"
```

---

## Task 12: Regenerate Wails bindings

**Files:**
- (auto-generated) `app/gui/wailsjs/go/cmd/CommandHandler.{ts,js}`
- (auto-generated) `app/gui/wailsjs/go/models.ts`

- [ ] **Step 1: Run bindings generator**

```sh
cd app
task bind
```

Expected: regenerates `app/gui/wailsjs/go/cmd/CommandHandler.d.ts` with new methods `GetPlayStatsHistory`, `GetPlayStatsCharacters`, `GetMatchesWithPlayStats`, and `model.PlayStatsSnapshot` / `MatchWithStats` in `models.ts`.

- [ ] **Step 2: Verify generated TS types include new APIs**

```sh
grep -E "GetPlayStatsHistory|GetPlayStatsCharacters|GetMatchesWithPlayStats" gui/wailsjs/go/cmd/CommandHandler.d.ts
grep -E "PlayStatsSnapshot|MatchWithStats" gui/wailsjs/go/models.ts
```

Expected: matching lines present.

- [ ] **Step 3: Commit**

```sh
git add app/gui/wailsjs/go/
git commit -m "chore(stats): regenerate Wails TS bindings"
```

---

## Task 13: Install recharts

**Files:**
- Modify: `app/gui/package.json`
- Modify: `app/gui/bun.lock` (auto-updated)

- [ ] **Step 1: Add dependency**

```sh
cd app/gui
mise exec -- bun add recharts
```

- [ ] **Step 2: Verify install**

```sh
mise exec -- bun pm ls recharts 2>&1 | head -3
```

Expected: shows `recharts@...` installed.

- [ ] **Step 3: Verify TS compile**

```sh
mise exec -- bun run tsc
```

Expected: still passes (`baseUrl` deprecation warning is pre-existing, OK).

- [ ] **Step 4: Commit**

```sh
git add package.json bun.lock
git commit -m "chore(stats): add recharts for trend chart"
```

---

## Task 14: Build /stats page — skeleton + selectors + empty states

**Files:**
- Create: `app/gui/src/pages/stats.tsx`
- Create: `app/gui/src/pages/stats/formatters.ts`

The page is split across multiple files (one responsibility each). Subsequent tasks fill in the children.

- [ ] **Step 1: Add formatters**

`app/gui/src/pages/stats/formatters.ts`:

```ts
export function formatRate(value: number | null | undefined): string {
  if (value == null) return '—'
  return `${(value * 100).toFixed(1)}%`
}

export function formatPerMatchCount(value: number | null | undefined): string {
  if (value == null) return '—'
  return `${value.toFixed(1)} 回`
}

export function formatSeconds(value: number | null | undefined): string {
  if (value == null) return '—'
  if (value < 60) return `${value}s`
  const m = Math.floor(value / 60)
  const s = value % 60
  if (m < 60) return `${m}m ${s}s`
  const h = Math.floor(m / 60)
  return `${h}h ${m % 60}m`
}

export function formatDelta(curr: number, prev: number | undefined, fmt: (n: number) => string): {
  text: string
  direction: 'up' | 'down' | 'flat'
} {
  if (prev === undefined) return { text: '—', direction: 'flat' }
  const diff = curr - prev
  if (Math.abs(diff) < 1e-6) return { text: '—', direction: 'flat' }
  const sign = diff > 0 ? '+' : ''
  return {
    text: `${sign}${fmt(diff)}`,
    direction: diff > 0 ? 'up' : 'down',
  }
}
```

- [ ] **Step 2: Page skeleton**

`app/gui/src/pages/stats.tsx`:

```tsx
import React from 'react'
import { useTranslation } from 'react-i18next'

import * as Page from '@/ui/page'
import { TrackingMachineContext } from '@/state/tracking-machine'
import { AuthMachineContext } from '@/state/auth-machine'
import { GetUsers, GetPlayStatsCharacters, GetPlayStatsHistory } from '@cmd/CommandHandler'
import { model } from '@model'

type Period = '7' | '30' | 'all'

export function StatsPage() {
  const { t } = useTranslation()
  const trackingUser = TrackingMachineContext.useSelector(s => s.context.user)
  const game = AuthMachineContext.useSelector(s => s.context.game)

  const [users, setUsers] = React.useState<model.User[]>([])
  const [characters, setCharacters] = React.useState<string[]>([])
  const [selectedUser, setSelectedUser] = React.useState<string>('')
  const [selectedChar, setSelectedChar] = React.useState<string>('')
  const [period, setPeriod] = React.useState<Period>('30')
  const [history, setHistory] = React.useState<model.PlayStatsSnapshot[]>([])
  const [loading, setLoading] = React.useState(false)

  React.useEffect(() => {
    GetUsers().then(us => {
      setUsers(us ?? [])
      if (!selectedUser && trackingUser) {
        setSelectedUser(trackingUser.code)
      } else if (!selectedUser && us?.length) {
        setSelectedUser(us[0].code)
      }
    })
  }, [trackingUser])

  React.useEffect(() => {
    if (!selectedUser) return
    GetPlayStatsCharacters(selectedUser).then(cs => {
      setCharacters(cs ?? [])
      if (cs?.length && !selectedChar) setSelectedChar(cs[0])
    })
  }, [selectedUser])

  React.useEffect(() => {
    if (!selectedUser || !selectedChar) return
    setLoading(true)
    const today = new Date()
    let from = ''
    if (period === '7' || period === '30') {
      const days = period === '7' ? 7 : 30
      const d = new Date(today.getTime() - days * 86400000)
      from = d.toISOString().slice(0, 10)
    }
    GetPlayStatsHistory(selectedUser, selectedChar, from, '', 0)
      .then(rows => setHistory(rows ?? []))
      .finally(() => setLoading(false))
  }, [selectedUser, selectedChar, period])

  // Empty / placeholder states (spec §5)
  if (!users.length && history.length === 0) {
    return (
      <Page.Root>
        <Page.Header>
          <Page.Title>{t('statsTitle')}</Page.Title>
        </Page.Header>
        <p className='text-center text-white/60 mt-12'>
          {game === model.GameType.STREET_FIGHTER_6 ? t('statsEmptyTracking') : t('statsSf6Only')}
        </p>
      </Page.Root>
    )
  }

  return (
    <Page.Root>
      <Page.Header>
        <Page.Title>{t('statsTitle')}</Page.Title>
      </Page.Header>

      <div className='flex gap-2 mb-4'>
        <select value={selectedUser} onChange={e => setSelectedUser(e.target.value)} className='bg-zinc-800 px-2 py-1'>
          {users.map(u => (
            <option key={u.code} value={u.code}>{u.displayName}</option>
          ))}
        </select>
        <select value={selectedChar} onChange={e => setSelectedChar(e.target.value)} className='bg-zinc-800 px-2 py-1'>
          {characters.map(c => (
            <option key={c} value={c}>{c}</option>
          ))}
        </select>
        <select value={period} onChange={e => setPeriod(e.target.value as Period)} className='bg-zinc-800 px-2 py-1'>
          <option value='7'>{t('statsPeriod7Days')}</option>
          <option value='30'>{t('statsPeriod30Days')}</option>
          <option value='all'>{t('statsPeriodAllTime')}</option>
        </select>
      </div>

      {loading && <p className='text-white/60'>{t('loading')}</p>}

      {history.length === 0 && !loading && (
        <p className='text-center text-white/60 mt-8'>{t('statsEmptyTracking')}</p>
      )}

      {history.length > 0 && (
        <pre className='text-xs text-white/60'>
          {/* KPI cards (Task 15) and trend chart (Task 16) go here */}
          {JSON.stringify(history[history.length - 1], null, 2).slice(0, 400)}
        </pre>
      )}
    </Page.Root>
  )
}
```

- [ ] **Step 3: Verify TS compile**

```sh
cd app/gui
mise exec -- bun run tsc
```

Expected: success.

- [ ] **Step 4: Commit**

```sh
git add app/gui/src/pages/stats.tsx app/gui/src/pages/stats/formatters.ts
git commit -m "feat(stats): scaffold /stats page with selectors and empty states"
```

---

## Task 15: Add KPI cards

**Files:**
- Create: `app/gui/src/pages/stats/kpi-card.tsx`
- Modify: `app/gui/src/pages/stats.tsx`

- [ ] **Step 1: Create the card component**

`app/gui/src/pages/stats/kpi-card.tsx`:

```tsx
import React from 'react'

import { cn } from '@/helpers/cn'

type Props = {
  label: string
  value: string
  delta: { text: string; direction: 'up' | 'down' | 'flat' }
  tooltip?: string
}

export function KpiCard({ label, value, delta, tooltip }: Props) {
  const arrow = delta.direction === 'up' ? '↑' : delta.direction === 'down' ? '↓' : ''
  return (
    <div className='rounded-lg bg-zinc-800/80 p-3 min-w-[120px]' title={tooltip}>
      <div className='text-xs text-white/60 mb-1'>{label}</div>
      <div className='text-xl text-white tabular-nums'>{value}</div>
      <div
        className={cn(
          'text-xs mt-1',
          delta.direction === 'up' && 'text-emerald-400',
          delta.direction === 'down' && 'text-rose-400',
          delta.direction === 'flat' && 'text-white/40'
        )}
      >
        {delta.text} {arrow}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Render 6 cards in stats.tsx**

In `app/gui/src/pages/stats.tsx`, replace the placeholder `<pre>` block (the one that prints JSON) with:

```tsx
      {history.length > 0 && (
        <>
          <div className='grid grid-cols-3 gap-3 mb-6'>
            {kpiSpecs.map(spec => {
              const curr = history[history.length - 1]
              const prev = history.length > 1 ? history[history.length - 2] : undefined
              const delta = formatDelta(spec.value(curr), prev ? spec.value(prev) : undefined, spec.format)
              return (
                <KpiCard
                  key={spec.key}
                  label={t(spec.key)}
                  value={spec.format(spec.value(curr))}
                  delta={delta}
                  tooltip={t('statsTooltip')}
                />
              )
            })}
          </div>
        </>
      )}
```

Add new imports at the top of stats.tsx:

```tsx
import { KpiCard } from './stats/kpi-card'
import { formatRate, formatPerMatchCount, formatSeconds, formatDelta } from './stats/formatters'
```

Add `kpiSpecs` constant just below the component-level imports:

```tsx
type Snapshot = model.PlayStatsSnapshot

const kpiSpecs: Array<{
  key: string
  value: (s: Snapshot) => number
  format: (n: number) => string
}> = [
  { key: 'kpiDriveImpact',  value: s => s.driveImpact,         format: formatPerMatchCount },
  { key: 'kpiReceivedDi',   value: s => s.receivedDriveImpact, format: formatPerMatchCount },
  { key: 'kpiJustParry',    value: s => s.justParry,           format: formatPerMatchCount },
  { key: 'kpiThrowTech',    value: s => s.throwTech,           format: formatPerMatchCount },
  { key: 'kpiCornerTime',   value: s => s.cornerTime,          format: formatSeconds },
  { key: 'kpiSaLv3',        value: s => s.gaugeRateSALv3,      format: formatRate },
]
```

- [ ] **Step 3: TS compile check**

```sh
cd app/gui
mise exec -- bun run tsc
```

Expected: PASS.

- [ ] **Step 4: Commit**

```sh
git add app/gui/src/pages/stats.tsx app/gui/src/pages/stats/kpi-card.tsx
git commit -m "feat(stats): render KPI cards with previous-delta"
```

---

## Task 16: Add trend chart

**Files:**
- Create: `app/gui/src/pages/stats/trend-chart.tsx`
- Modify: `app/gui/src/pages/stats.tsx`

- [ ] **Step 1: Create the chart component**

`app/gui/src/pages/stats/trend-chart.tsx`:

```tsx
import React from 'react'
import { LineChart, Line, XAxis, YAxis, Tooltip, Legend, ResponsiveContainer } from 'recharts'

import { model } from '@model'

type Props = {
  history: model.PlayStatsSnapshot[]
}

const series: Array<{
  key: keyof model.PlayStatsSnapshot
  label: string
  color: string
}> = [
  { key: 'driveImpact',         label: 'DI 命中',   color: '#34d399' },
  { key: 'receivedDriveImpact', label: 'DI 被弾',   color: '#f87171' },
  { key: 'justParry',           label: 'ジャパリ',  color: '#a78bfa' },
  { key: 'throwTech',           label: '投げ抜け',  color: '#fbbf24' },
  { key: 'cornerTime',          label: '壁際秒',    color: '#60a5fa' },
  { key: 'gaugeRateSALv3',      label: 'SA Lv3 %',  color: '#fb923c' },
]

export function TrendChart({ history }: Props) {
  const data = history.map(s => ({
    snapshotAt: s.snapshotAt,
    ...Object.fromEntries(series.map(sr => [sr.key, s[sr.key] as number])),
  }))

  if (history.length < 2) {
    return (
      <div className='h-64 flex items-center justify-center text-white/40'>
        — 2 points required for a trend —
      </div>
    )
  }

  return (
    <div className='h-64 w-full mb-6'>
      <ResponsiveContainer>
        <LineChart data={data} margin={{ top: 10, right: 20, left: 0, bottom: 0 }}>
          <XAxis dataKey='snapshotAt' tick={{ fontSize: 10 }} stroke='#888' />
          <YAxis tick={{ fontSize: 10 }} stroke='#888' />
          <Tooltip
            contentStyle={{ background: '#1f1f23', border: '1px solid #333' }}
            labelStyle={{ color: '#aaa' }}
          />
          <Legend wrapperStyle={{ fontSize: 11 }} />
          {series.map(s => (
            <Line key={s.key} type='monotone' dataKey={s.key} name={s.label} stroke={s.color} dot={false} />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
```

- [ ] **Step 2: Wire into stats.tsx**

In `app/gui/src/pages/stats.tsx`, add import:

```tsx
import { TrendChart } from './stats/trend-chart'
```

After the KPI grid block, add:

```tsx
          <TrendChart history={history} />
```

- [ ] **Step 3: TS compile check**

```sh
cd app/gui
mise exec -- bun run tsc
```

Expected: PASS.

- [ ] **Step 4: Commit**

```sh
git add app/gui/src/pages/stats.tsx app/gui/src/pages/stats/trend-chart.tsx
git commit -m "feat(stats): add recharts trend chart"
```

---

## Task 17: Add detail expand table

**Files:**
- Create: `app/gui/src/pages/stats/detail-table.tsx`
- Modify: `app/gui/src/pages/stats.tsx`

- [ ] **Step 1: Create the table component**

`app/gui/src/pages/stats/detail-table.tsx`:

```tsx
import React from 'react'
import { useTranslation } from 'react-i18next'

import { GetMatchesWithPlayStats } from '@cmd/CommandHandler'
import { model } from '@model'

type Props = {
  userId: string
  character: string
}

export function DetailTable({ userId, character }: Props) {
  const { t } = useTranslation()
  const [open, setOpen] = React.useState(false)
  const [rows, setRows] = React.useState<model.MatchWithStats[]>([])
  const [loading, setLoading] = React.useState(false)

  const load = () => {
    setLoading(true)
    GetMatchesWithPlayStats(userId, character, 50, 0)
      .then(r => setRows(r ?? []))
      .finally(() => setLoading(false))
  }

  React.useEffect(() => {
    if (open && rows.length === 0) load()
  }, [open])

  return (
    <div className='mt-6'>
      <button
        onClick={() => setOpen(o => !o)}
        className='text-white/70 hover:text-white text-sm'
      >
        {open ? '▼' : '▶'} {t('statsExpandDetails')}
      </button>

      {open && (
        <div className='mt-3 overflow-x-auto'>
          {loading && <p className='text-white/60'>{t('loading')}</p>}
          <table className='text-xs w-full'>
            <thead className='text-white/60'>
              <tr>
                <th className='text-left p-1'>Date</th>
                <th className='text-left p-1'>Time</th>
                <th className='text-left p-1'>Char</th>
                <th className='text-left p-1'>Opp</th>
                <th className='text-left p-1'>W/L</th>
                <th className='text-right p-1'>LPΔ</th>
                <th className='text-right p-1'>DI</th>
                <th className='text-right p-1'>DI被</th>
                <th className='text-right p-1'>ジャパリ</th>
                <th className='text-right p-1'>投抜</th>
                <th className='text-right p-1'>壁秒</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r, i) => (
                <tr key={`${r.match.replayId}-${i}`} className='border-t border-white/10'>
                  <td className='p-1'>{r.match.date}</td>
                  <td className='p-1'>{r.match.time}</td>
                  <td className='p-1'>{r.match.character}</td>
                  <td className='p-1'>{r.match.opponent ?? '—'}</td>
                  <td className='p-1'>{r.match.victory ? 'W' : 'L'}</td>
                  <td className='p-1 text-right'>{r.match.lpGain ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.driveImpact?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.receivedDriveImpact?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.justParry?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.throwTech?.toFixed(1) ?? '—'}</td>
                  <td className='p-1 text-right'>{r.stats?.cornerTime ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Wire into stats.tsx**

Add import:

```tsx
import { DetailTable } from './stats/detail-table'
```

After the `<TrendChart />` block:

```tsx
          <DetailTable userId={selectedUser} character={selectedChar} />
```

- [ ] **Step 3: TS compile check**

```sh
cd app/gui
mise exec -- bun run tsc
```

Expected: PASS.

- [ ] **Step 4: Commit**

```sh
git add app/gui/src/pages/stats.tsx app/gui/src/pages/stats/detail-table.tsx
git commit -m "feat(stats): add detail expand table with matches LEFT JOIN"
```

---

## Task 18: Wire route and sidebar

**Files:**
- Modify: `app/gui/src/main/router.tsx`
- Modify: `app/gui/src/main/app-sidebar.tsx`

- [ ] **Step 1: Add the route**

In `app/gui/src/main/router.tsx`, add import:

```tsx
import { StatsPage } from '@/pages/stats'
```

In the `children` array (after `MatchesListPage`), add:

```tsx
          {
            element: <StatsPage />,
            path: '/stats'
          },
```

- [ ] **Step 2: Add sidebar link**

In `app/gui/src/main/app-sidebar.tsx`, locate the existing sidebar items array and add an entry matching the existing pattern:

```tsx
  { href: '/stats', labelKey: 'statsNav' },
```

(Adapt to the actual data shape — if the sidebar uses `<NavLink to='/stats'>{t('statsNav')}</NavLink>` directly, add an analogous block.)

- [ ] **Step 3: Bring up the app and visually verify**

```sh
cd app
task dev-hard
```

Click the "実績推移" / "Play Stats" link in the sidebar. Empty state should appear if no snapshots are saved yet.

- [ ] **Step 4: Commit**

```sh
git add app/gui/src/main/router.tsx app/gui/src/main/app-sidebar.tsx
git commit -m "feat(stats): expose /stats route and sidebar link"
```

---

## Task 19: End-to-end smoke verification

This is the final manual check — no commits expected unless something needs fixing.

- [ ] **Step 1: Reset DB to apply fresh migration**

Stop `task dev-hard` if running, then remove the local SQLite DB so migration 000004 applies cleanly:

```sh
rm -f ~/Library/Caches/cfn-tracker/cfn-tracker.db
```

- [ ] **Step 2: Start the app and trigger a tracking session**

```sh
cd app
task dev-hard
```

In the app:

1. Choose Street Fighter 6 (authentication completes)
2. Enter your CFN code, click Start
3. Wait for at least one new match to be detected (you may need to play a ranked match)
4. Open `/stats` from the sidebar

- [ ] **Step 3: Inspect the DB to confirm snapshots**

```sh
mise exec -- bunx --bun sqlite3 ~/Library/Caches/cfn-tracker/cfn-tracker.db \
  "SELECT user_id, character, match_replay_id, snapshot_at, drive_impact, just_parry FROM play_stats_snapshots ORDER BY id DESC LIMIT 5"
```

Expected:

- At least 1 row where `match_replay_id IS NULL` (baseline)
- At least 1 row where `match_replay_id` matches a recent `matches.replay_id` (per-match)
- Values look plausible (`drive_impact` ~1.0-2.0, `just_parry` ≥ 0)

- [ ] **Step 4: Verify the dashboard renders**

In the GUI's `/stats` page:

1. User selector lists your tracked user
2. Character selector lists your favorite character (e.g. "JP")
3. KPI cards show current values + previous-delta indicators
4. Trend chart appears once there are ≥ 2 snapshots
5. Detail expand table reveals match rows alongside their snapshots

- [ ] **Step 5: T8 regression check**

Stop the app. Restart it, choose Tekken 8, run tracking for a moment. Verify:

- `/stats` page still accessible from sidebar
- Shows the empty SF6-only placeholder (because no SF6 snapshots exist for the T8 user)
- `slog` output contains no `play stats` errors

- [ ] **Step 6: Final test sweep**

```sh
cd app
mise exec -- go test ./...
```

Expected: all PASS.

```sh
cd app/gui
mise exec -- bun run tsc
mise exec -- bun run format:check
```

Expected: TS compile PASS (deprecation warning OK); prettier PASS.

---

## Spec Coverage Self-Review

- [x] §2 BattleStats / BaseInfo / ContentPlayTime → Task 2
- [x] §3 schema + indexes + no FK → Task 1
- [x] §3 matches JOIN snapshots → Task 7
- [x] §4 SF6 type assertion + 30-min baseline skip → Task 9
- [x] §4 error handling = warn only → Task 9
- [x] §5 sidebar visible always, page-level empty states → Tasks 14, 18
- [x] §5 KPI 6 cards + recharts + detail expand → Tasks 15, 16, 17
- [x] §6 storage APIs + command handlers → Tasks 5-7, 10
- [x] §6 Localization struct + 3 locale files + `task bind` → Tasks 11, 12
- [x] §6 testing (unmarshal + storage roundtrip) → Tasks 2, 5-7
- [x] §7 FK avoided, `m.character` placement → Tasks 1, 7

## Execution Handoff

Plan complete and saved to `specs/2026-05-22-play-stats-phase-a-plan.md`. Two execution options:

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks, fast iteration
2. **Inline Execution** — execute tasks in this session, batch with checkpoints

Which approach?
