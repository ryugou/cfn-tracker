# Play Stats Phase A — Design

- 日付: 2026-05-22
- ブランチ: `feat/play-stats-phase-a`
- 関連: 後続の Phase B (上級者ベンチマーク機能) を別 PR で計画

## 1. 目的とスコープ

SF6 の Buckler プロフィール `/play` ページから取得できる **過去 100 戦平均** の戦闘統計 (ドライブインパクト命中数、ジャストパリィ回数、ドライブゲージ使用率、投げ抜け回数、壁際時間 等) を **新マッチ検知のたびにスナップショットとして SQLite に蓄積**し、推移を GUI のダッシュボードで可視化する。

100 戦平均はスライディングウィンドウのため、隣接スナップショットの差分を取れば「直近 1 試合分の寄与」に近い量を推定できる (厳密には「ウィンドウが 1 試合スライドしたときの平均変化」)。スナップショットを全件残すことで、後段の分析 (Phase B でのベンチマーク、将来の Vegapunk 連携) のデータ基盤とする。

各スナップショットを **対応する match (勝敗・対戦相手・LP/MR 増減を持つ既存 `matches` 行)** に紐付けることで、`matches JOIN play_stats_snapshots` の SELECT で「その試合の勝ち負け × その時点の 100 戦平均」をペアで取り出せる構造とする。

### 含むもの

- `/play` ページのスクレイピング (新マッチ検知時 + トラッキング開始時の初回基準)
- スナップショット用 SQLite テーブル新設
- 統計推移ダッシュボード (新ページ `/stats`)
- 折れ線グラフ用ライブラリ追加 (`recharts`)
- SF6 専用機能としてのガード (T8 トラッキング中は取得しない・UI から非表示)

### 含まないもの (将来 Phase で対応)

- 他ユーザー (上級者) の `/play` 取得とベンチマーク表示 → **Phase B**
- Vegapunk への蓄積データ送信と分析レポート → **Phase C**
- 練習プラン自動生成 → **Phase D**
- Tekken 8 対応 (wavu.wiki に同等エンドポイントが無いため SF6 専用機能)

## 2. 取得対象データ (実 JSON より)

`/play` ページの `<script id="__NEXT_DATA__">` に埋め込まれた JSON のうち、本機能で扱うのは `props.pageProps.play` 配下のみ。実値サンプル:

### `play.battle_stats` (38 フィールド)

```json
{
  "battle_hub_match_play_count": 6,
  "casual_match_play_count": 0,
  "corner_time": 3,
  "cornered_time": 10,
  "custom_room_match_play_count": 11,
  "drive_impact": 1.2,
  "drive_impact_to_drive_impact": 0.1,
  "drive_parry": 0.6,
  "drive_reversal": 0.1,
  "gauge_rate_ca": 0.1803,
  "gauge_rate_drive_arts": 0.119,
  "gauge_rate_drive_guard": 0.0435,
  "gauge_rate_drive_impact": 0.2154,
  "gauge_rate_drive_other": 0.5994,
  "gauge_rate_drive_reversal": 0.0119,
  "gauge_rate_drive_rush_from_cancel": 0.0107,
  "gauge_rate_drive_rush_from_parry": 0,
  "gauge_rate_sa_lv1": 0.5081,
  "gauge_rate_sa_lv2": 0.0327,
  "gauge_rate_sa_lv3": 0.2786,
  "just_parry": 0,
  "punish_counter": 0.6,
  "rank_match_play_count": 59,
  "received_drive_impact": 1.9,
  "received_drive_impact_to_drive_impact": 0.2,
  "received_punish_counter": 1,
  "received_stun": 0,
  "received_throw_count": 2.2,
  "received_throw_drive_parry": 0,
  "rival_ai_achieved_challenge_count": 0,
  "rival_ai_highest_league_rank": 2,
  "rival_ai_highest_league_rank_txt": "Rookie 2",
  "stun": 0,
  "target_clear_count": 31,
  "throw_count": 0.8,
  "throw_drive_parry": 0.1,
  "throw_tech": 0.1,
  "total_all_character_play_point": 11425
}
```

### フィールドの意味と単位 (現状の理解)

- **`gauge_rate_*`** (合計 = 1.0): ドライブゲージ・SA ゲージの**使用先内訳の比率**。例: `0.2154` = 21.54% をドライブインパクトに使用
- **`drive_impact` / `just_parry` / `throw_count` 等の小数値**: **直近 100 ランクマ内での 1 試合平均回数**
- **`received_*`**: 相手から受けた回数の 1 試合平均
- **`corner_time` / `cornered_time`**: **秒数 (1 試合平均)**。例: `3` = 1 試合あたり平均 3 秒間相手を壁際に追い詰めている
- **`*_play_count`** (`rank_match_play_count`, `casual_match_play_count`, `battle_hub_match_play_count`, `custom_room_match_play_count`): **累計プレイ回数** (ウィンドウ内ではない)
- **`target_clear_count`** / **`total_all_character_play_point`** / **`rival_ai_achieved_challenge_count`**: 累計値
- **`rival_ai_highest_league_rank`** / `_txt`: V ライバル (CPU) で到達した最高ランク

なお `drive_impact = 1.2` は実値、`corner_time = 3` も実値で、それぞれの単位は上記推定。Phase A 実装時に DB に保存しておけば後で再解釈可能なので、現時点では生値をそのまま格納する方針。

### `play.base_info` (プレイ時間と Enjoy ポイント)

```json
{
  "content_play_time_list": [
    { "content_type": 1, "content_type_name": "World Tour",          "play_time": 59936 },
    { "content_type": 2, "content_type_name": "Ranked Matches",      "play_time": 4692 },
    { "content_type": 3, "content_type_name": "Casual Matches",      "play_time": 0 },
    { "content_type": 4, "content_type_name": "Custom Room Matches", "play_time": 2634 },
    { "content_type": 5, "content_type_name": "Battle Hub",          "play_time": 14401 },
    { "content_type": 6, "content_type_name": "Offline Matches",     "play_time": 0 },
    { "content_type": 7, "content_type_name": "Arcade",              "play_time": 8353 },
    { "content_type": 8, "content_type_name": "Practice",            "play_time": 159402 },
    { "content_type": 9, "content_type_name": "Extreme",             "play_time": 0 }
  ],
  "enjoy_fight_point": 3,
  "enjoy_total_point": 9,
  "enjoy_user_point": 6
}
```

`content_type` は固定 enum (1..9)。`play_time` は累計秒。

### キャラクタ判定 — 既存 `matches.character` に揃える

`fighter_banner_info.favorite_character_name` (例: `"JP"`、表示名) が**現在のお気に入りキャラ**を示し、`battle_stats` はそのキャラの値。

**既存の `matches.character` も `BattleLog.GetCharacter() = FavoriteCharacterName` を使っており表示名ベース** なので、Phase A では `play_stats_snapshots.character` も **表示名 (`favorite_character_name`)** に揃える。これにより `/sessions` の対戦履歴と `/stats` のキャラフィルタが同じキーで一致する。

将来 Phase B で他言語のクライアントに合わせて tool_name (`"jp"` 等) に正規化する余地はあるが、その場合は `matches.character` も同時に移行する別タスクとする。

## 3. データモデル

### 新規テーブル: `play_stats_snapshots`

```sql
CREATE TABLE IF NOT EXISTS play_stats_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  character TEXT NOT NULL,            -- 表示名 (FavoriteCharacterName)
  match_replay_id TEXT,               -- NULL = 初回基準スナップショット
  snapshot_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,

  -- battle_stats
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

  -- base_info enjoy points
  enjoy_fight_point INTEGER,
  enjoy_total_point INTEGER,
  enjoy_user_point INTEGER,

  -- base_info content_play_time_list (秒、固定 enum content_type)
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

カラム合計 55 (meta 5 + battle_stats 38 + enjoy 3 + content_play_time 9)。

### FK 制約は付けない

- `matches.replay_id` は `TEXT NOT NULL DEFAULT ''` で UNIQUE/PK 制約が無いため、SQLite の FK 仕様上 `REFERENCES matches(replay_id)` は成立しない (PRAGMA foreign_keys が ON でも制約破綻、OFF だと無意味)
- 同様に `users(code)` も既存 matches では FK 宣言があるが運用上ゆるい結合
- Phase A では FK を**付けない**。`user_id` / `match_replay_id` は **論理的な参照** として INDEX のみ用意。アプリ層で整合性を担保する

### 一意性

- 同じ `(user_id, character, match_replay_id)` が複数挿入されることは原則ないが、リトライ等で重複する可能性あり
- 重複は許容し、SELECT 側で `snapshot_at` 最新を取る方針。`UNIQUE` 制約は当面付けない (Phase A の段階では分析データを失う方を恐れる)

### マイグレーション

- `000004_add_play_stats_snapshots.up.sql` — 上記 CREATE 文 + INDEX
- `000004_add_play_stats_snapshots.down.sql` — `DROP INDEX` + `DROP TABLE play_stats_snapshots`

### `matches JOIN play_stats_snapshots` の使い方 (要件)

ダッシュボード側で、各試合の勝敗とその時点のスナップショットを 1 行で取り出せる:

```sql
SELECT
  m.replay_id,
  m.date, m.time,
  m.victory, m.opponent, m.opponent_character,
  m.lp, m.lp_gain, m.mr, m.mr_gain,
  ps.drive_impact, ps.received_drive_impact, ps.just_parry,
  ps.gauge_rate_sa_lv3, ps.corner_time, ps.cornered_time,
  ps.snapshot_at
FROM matches m
LEFT JOIN play_stats_snapshots ps
  ON ps.match_replay_id = m.replay_id
 AND ps.user_id = m.user_id
WHERE m.user_id = ?
  AND ps.character = ?
ORDER BY m.date DESC, m.time DESC;
```

これにより「勝った試合の前後で DI 命中はどう動いたか」「連敗中の壁際時間が短い」等の分析が後段で可能。

## 4. 取得ロジック

### 取得タイミング

`cmd.TrackingHandler.StartTracking` のポーリングループ内で、以下のタイミングに `/play` も取得する:

1. **トラッキング開始直後 (基準値の 1 回)**: `gameTracker.GetUser` 成功後、`session.Matches` の有無に関わらず一度取得。`match_replay_id = NULL` で保存し、後段の差分計算の起点とする
2. **新マッチ検知時**: `gameTracker.Poll` が新マッチを返した直後、SQL/txt の match 保存に成功した後で取得。`match_replay_id = match.ReplayID` で保存

30 秒のティック自体は既存と変わらない。**新マッチがない時 (replay_id 不変) は `/play` も叩かない**ので、リクエスト負荷は新マッチ時のみ 2 倍 (`/battlelog/rank` + `/play`)。

### レイヤー責務

抽象化を壊さないため `tracker.GameTracker` インターフェースは変更せず、SF6 専用の追加機能として `SF6Tracker` に直接メソッドを生やす:

```go
// pkg/tracker/sf6/track.go
type PlayStatsResult struct {
    Character string                    // FavoriteCharacterName (表示名)
    Stats     *cfn.BattleStats          // play.battle_stats
    BaseInfo  *cfn.BaseInfo             // play.base_info
}

func (t *SF6Tracker) PollPlayStats(ctx context.Context, userCode string) (*PlayStatsResult, error)
```

`cmd.TrackingHandler` 側では、型アサーションで SF6 のときだけ呼ぶ:

```go
// cmd/tracking.go (StartTracking 内、新マッチ保存後)
if sf6, ok := ch.gameTracker.(*sf6.SF6Tracker); ok {
    if res, err := sf6.PollPlayStats(ctx, session.UserId); err != nil {
        slog.Warn("get play stats failed, skipping snapshot",
            slog.Any("error", err))
    } else {
        snap := model.PlayStatsSnapshot{
            UserId:        session.UserId,
            Character:     res.Character,
            MatchReplayId: sql.NullString{String: match.ReplayID, Valid: true},
            // ... 全 55 カラムを res.Stats / res.BaseInfo から詰める
        }
        if err := ch.sqlDb.SavePlayStats(ctx, snap); err != nil {
            slog.Warn("save play stats failed", slog.Any("error", err))
        }
    }
}
```

T8 (`*t8.T8Tracker`) のときは型アサーションに失敗するのでスキップされ、影響なし。トラッキング開始直後の初回保存も同じ型アサーションで囲む (`MatchReplayId` は `sql.NullString{Valid: false}`)。

### エラーハンドリング

`/play` 取得・保存が失敗しても **match 保存は続行**する (graceful degradation)。詳細は上記コードのとおり、`slog.Warn` で記録するのみで、ポーリングループの中断には繋げない。

差分計算は隣接スナップショットでしか機能しないので、欠落がある時は 2 試合分まとめた寄与として読み取る (集計時のクライアント責務、UI 側にも注記)。

### スクレイピング詳細

- URL: `https://www.streetfighter.com/6/buckler/profile/<user_code>/play`
- 抽出: `#__NEXT_DATA__` の textContent を JSON.parse、`props.pageProps.play.battle_stats` と `.base_info` を取り出す
- 失敗判定: `common.statusCode != 200` または `play == nil`

## 5. UI ダッシュボード

### 新ページ `/stats` (SF6 専用)

サイドバーに「実績推移」リンクを追加し、ルート `/stats` を新設。

T8 トラッキング中は **サイドバーから本リンクを非表示**にし、`/stats` を直接開いた場合は「SF6 限定の機能です」というプレースホルダを表示する。判定は `AuthMachineContext` の `context.game` を参照。

### レイアウト概要

```
┌─────────────────────────────────────────────────────┐
│ ヘッダ: ユーザー [選択▼] / キャラ [選択▼] / 期間 [選択▼]│
├─────────────────────────────────────────────────────┤
│ KPI カード 6 枚 (現在値 + 前回比 ↑↓ / —)              │
│  ┌─────────┐┌─────────┐┌─────────┐                  │
│  │ DI 命中 ││ DI 被弾 ││ ジャパリ│ ...               │
│  │  1.2回  ││  1.9回  ││  0.0回  │                  │
│  │ 前回比  ││ 前回比  ││ ─       │                  │
│  │ +0.3 ↑  ││ -0.2 ↓  ││         │                  │
│  └─────────┘└─────────┘└─────────┘                  │
├─────────────────────────────────────────────────────┤
│ [折れ線グラフ] 主要 KPI の時系列推移 (重ね描き)        │
├─────────────────────────────────────────────────────┤
│ [▼ 全項目を見る] (clickで展開)                        │
│   全 55 列のスナップショット時系列テーブル              │
│   (matches.victory・lp_gain・mr_gain と JOIN 表示)    │
└─────────────────────────────────────────────────────┘
```

### KPI 差分の解釈注記

「前回比」は 100 戦平均値の差分であり、**直前 1 試合の純粋な寄与とは限らない** (前後の試合間隔次第ではウィンドウから複数試合が出入りしている可能性あり)。UI ラベルは「前回比」「変化」程度に留め、「+0.3 = 1 試合で +0.3 をマーク」と誤読されないよう ツールチップで「100 戦平均の前回スナップショットからの差分」と説明する。

### 空状態

- スナップショット **0 件**: 「まだデータがありません。トラッキングを開始すると 1 試合ごとに記録されます」のプレースホルダ
- **1 件のみ**: KPI カードは現在値のみ表示 (前回比は ─)、グラフは点 1 つで描画

### 主要 KPI (KPI カード + グラフ対象)

実装者判断で以下を初期セットとし、使いながら調整する:

1. **drive_impact** (DI 命中) — 攻撃起点指標
2. **received_drive_impact** (DI 被弾) — DI 確認力
3. **just_parry** (ジャストパリィ) — 上級者度
4. **throw_tech** (投げ抜け) — 防御力
5. **corner_time** (壁際追い詰め秒数) — 場所取り
6. **gauge_rate_sa_lv3** (SA Lv3 使用率) — リソース管理判断

数値フォーマット:

- 比率系 (`gauge_rate_*`): 小数 → パーセント (`0.2154` → `21.5%`)
- 秒数系 (`corner_time`, `cornered_time`, `*_seconds`): 整数秒 (>= 3600 は `1h 23m` 表示)
- 回数系 (`drive_impact`, `throw_count` 等): 小数 1 桁 (`1.2 回`)
- 累計系 (`rank_match_play_count`, `target_clear_count` 等): 整数

### グラフライブラリ

`recharts` を採用 (React 19 互換、TypeScript ネイティブ、`bun add recharts`)。

6 系列の重ね描きはツールチップ・凡例・色の見やすさを実装時に検証。`Tooltip` には「100 戦平均値」「前回比」を併記。

## 6. 実装ステップ

### 6.1 バックエンド (Go)

1. **JSON 構造の型定義**
   - `pkg/tracker/sf6/cfn/model.go` に追加:
     - `PlayPage` (ルート構造、ProfilePage と並列)
     - `PlayProps` / `BattleStats` (38 フィールド) / `BaseInfo` / `ContentPlayTime`
2. **DB マッピング型**
   - `pkg/model/play_stats.go` 新設
   - `PlayStatsSnapshot` 構造体 (`db:` タグ + `json:` タグ、55 カラム + `MatchReplayId sql.NullString`)
3. **スクレイピング**
   - `pkg/tracker/sf6/cfn/client.go` に `GetPlayStats(ctx, cfn) (*PlayPage, error)` を追加。既存 `GetBattleLog` と同パターンで `/play` を navigate、`#__NEXT_DATA__` から JSON 抽出
4. **トラッカー層**
   - `pkg/tracker/sf6/track.go` に `PollPlayStats(ctx, userCode) (*PlayStatsResult, error)` を追加
   - `tracker.GameTracker` インターフェースには **追加しない** (SF6 専用扱い)
5. **永続化層**
   - `pkg/storage/sql/migrations/000004_add_play_stats_snapshots.{up,down}.sql`
   - `pkg/storage/sql/play_stats.go`:
     - `SavePlayStats(ctx, PlayStatsSnapshot) error`
     - `GetPlayStatsHistory(ctx, userId, character string, from, to string, limit uint16) ([]*PlayStatsSnapshot, error)`
       - `from` / `to` は空文字で無視 (`YYYY-MM-DD` 形式、`snapshot_at` でフィルタ)
       - `limit` は 0 で無制限
     - `GetPlayStatsCharacters(ctx, userId string) ([]string, error)` — DISTINCT character
     - `GetMatchesWithPlayStats(ctx, userId, character string, limit uint8, offset uint16) ([]*MatchWithStats, error)` — matches LEFT JOIN play_stats_snapshots、全項目を返す (詳細展開テーブル用)
6. **トラッキング統合**
   - `cmd/tracking.go` の `StartTracking`:
     - 起動直後の `GetUser`/`SaveUser` 成功後、SF6 のとき初回スナップショットを取得して保存 (`match_replay_id = NULL`)
     - 既存マッチ保存ループ内で、新マッチ保存成功後に同じく `PollPlayStats` + `SavePlayStats`
     - どちらも `sf6.SF6Tracker` 型アサーションで囲む
7. **Wails Bind (Read API)**
   - `cmd/cmd.go` に追加し `main.go` の `Bind` 経由でフロントへ:
     - `GetPlayStatsHistory(userId, character, from, to string, limit uint16) ([]*PlayStatsSnapshot, error)`
     - `GetPlayStatsCharacters(userId string) ([]string, error)`
     - `GetMatchesWithPlayStats(userId, character string, limit uint8, offset uint16) ([]*MatchWithStats, error)`
8. **i18n 構造体更新**
   - `pkg/model/i18n.go` の `Localization` 構造体に新規キーを追加 (`Stats`, `KpiDriveImpact`, `KpiReceivedDriveImpact`, `KpiJustParry`, `KpiThrowTech`, `KpiCornerTime`, `KpiSaLv3`, `PreviousDelta`, `Period`, `Last7Days`, `Last30Days`, `AllTime`, `NoSnapshotsYet`, `SnapshotTooltip`, `ExpandAllFields`, `Sf6OnlyFeature` 等)
   - `pkg/i18n/locales/{en-GB,ja-JP,fr-FR}.json` 全てに同キーを追加
9. **TS バインディング再生成**
   - `cd app && task bind` (= `wails generate module`) を流して `app/gui/wailsjs/go/models.ts` / `cmd/*` を更新

### 6.2 フロントエンド (React)

10. **依存追加**: `cd app/gui && bun add recharts`
11. **新ページ `app/gui/src/pages/stats.tsx`**:
    - ユーザー / キャラ / 期間セレクタ
    - KPI カード 6 枚 (現在値 + 前回比、tooltip で「100戦平均の前回スナップショット差分」)
    - 折れ線グラフ (recharts)
    - 詳細展開 (全 55 列テーブル、matches 結果と JOIN 表示)
    - 空状態 (0 件 / 1 件) ハンドリング
12. **ルート追加**: `app/gui/src/main/router.tsx` に `/stats` を追加
13. **サイドバー**: `app/gui/src/main/app-sidebar.tsx` に「実績推移」リンクを追加。`AuthMachineContext` の `game === GameType.SF6` のときだけ表示
14. **SF6 限定プレースホルダ**: `/stats` を T8 で開いた場合の表示

### 6.3 テスト

15. **`pkg/tracker/sf6/cfn/client_test.go`** 新設:
    - `BattleStats` / `BaseInfo` の JSON unmarshal テスト (`play-stats-sample.json` を testdata に切り出し、機密性なし)
    - エラー JSON (statusCode != 200) のハンドリングテスト
16. **`pkg/storage/sql/play_stats_test.go`** 新設:
    - in-memory SQLite で `SavePlayStats` → `GetPlayStatsHistory` ラウンドトリップ
    - `from` / `to` / `limit` の境界
    - `GetPlayStatsCharacters` の DISTINCT 動作
    - `GetMatchesWithPlayStats` の LEFT JOIN 挙動 (play_stats 欠落時に NULL カラム)
17. **既存テストは変更なし** (T8 経路への regression が出ないことを `go test ./...` で確認)

## 7. リスク・前提

- Capcom 側 `/play` ページの DOM 構造 (`#__NEXT_DATA__` パターン) が将来変わると壊れる
- `fighter_banner_info.favorite_character_name` がお気に入りキャラ依存。ユーザーがゲーム内で頻繁に切り替えるとスナップショットが `character` 列で分散するが、INDEX で時系列はキャラ別に引ける
- 1 マッチごとに `/battlelog/rank` + `/play` の 2 リクエストになるが、**新マッチ検知時のみ** で 30 秒ごとに必ず増えるわけではない
- `play_stats_snapshots.match_replay_id` は FK 制約を**付けない** (`matches.replay_id` が PK/UNIQUE でないため SQLite 制約上不成立)。アプリ層で整合性担保
- `matches.replay_id` が空文字 `''` のレコードと JOIN すると曖昧になり得るため、JOIN 時は `match_replay_id != '' AND match_replay_id IS NOT NULL` でフィルタする

## 8. Phase B プレビュー (実装は別 PR)

- 設定画面で「ベンチマーク対象 CFN コード」を複数登録できるようにする
- 1 日 1 回など低頻度で他ユーザーの `/play` を取得して同じ `play_stats_snapshots` テーブルに保存
- `/stats` 画面で「自分 vs 平均」をカード/グラフに重ね表示
- `user_id` 列で自分以外も区別できるので、本 Phase のスキーマで対応可能

### Phase B 開始時の users テーブル方針

`users(code)` テーブルは現状「自分のトラッキング履歴の登録ユーザー」だが、Phase B で他ユーザーをベンチマーク対象に追加する際は **取得時に `SaveUser` で登録**する方針 (matches 側のリレーション運用と一貫)。Phase A のスキーマには影響しない。

## 9. 残された判断 (実装時に決める軽量項目)

- KPI カード 6 個の最終ライナップ (使いながら調整)
- 折れ線グラフの色・凡例・ツールチップ詳細
- 期間セレクタのデフォルト値 (推奨: 直近 30 日)
- 詳細展開テーブルの初期表示行数 (推奨: 50 行 + ページネーション)
