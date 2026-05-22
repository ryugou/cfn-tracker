# Play Stats Phase A — Design

- 日付: 2026-05-22
- ブランチ: `feat/play-stats-phase-a`
- 関連: 後続の Phase B (上級者ベンチマーク機能) を別 PR で計画

## 1. 目的とスコープ

SF6 の Buckler プロフィール `/play` ページから取得できる **過去 100 戦平均** の戦闘統計 (ドライブインパクト命中数、ジャストパリィ回数、ドライブゲージ使用率、投げ抜け回数、壁際時間 等) を **新マッチ検知のたびにスナップショットとして SQLite に蓄積**し、推移を GUI のダッシュボードで可視化する。

100 戦平均はスライディングウィンドウのため、隣接スナップショットの差分を取れば 1 試合あたりの寄与を逆算できる。**スナップショットを全件残す**ことで後段の分析 (Phase B でのベンチマーク、将来の Vegapunk 連携) のデータ基盤とする。

### 含むもの

- `/play` ページのスクレイピング (新マッチ検知時に追加リクエスト)
- スナップショット用 SQLite テーブル新設
- 統計推移ダッシュボード (新ページ `/stats`)
- 折れ線グラフ用ライブラリ追加 (`recharts`)

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

### `play.base_info` (プレイ時間と Enjoy ポイント)

```json
{
  "content_play_time_list": [
    { "content_type": 1, "content_type_name": "World Tour", "play_time": 59936 },
    { "content_type": 2, "content_type_name": "Ranked Matches", "play_time": 4692 },
    { "content_type": 3, "content_type_name": "Casual Matches", "play_time": 0 },
    { "content_type": 4, "content_type_name": "Custom Room Matches", "play_time": 2634 },
    { "content_type": 5, "content_type_name": "Battle Hub", "play_time": 14401 },
    { "content_type": 6, "content_type_name": "Offline Matches", "play_time": 0 },
    { "content_type": 7, "content_type_name": "Arcade", "play_time": 8353 },
    { "content_type": 8, "content_type_name": "Practice", "play_time": 159402 },
    { "content_type": 9, "content_type_name": "Extreme", "play_time": 0 }
  ],
  "enjoy_fight_point": 3,
  "enjoy_total_point": 9,
  "enjoy_user_point": 6
}
```

`content_type` は固定 enum (1..9)。`play_time` は秒。

### キャラクタ判定

`fighter_banner_info.favorite_character_tool_name` (例: `"jp"`) が**現在のお気に入りキャラ**を示し、`battle_stats` はそのキャラの値。同一ユーザーがゲーム内でお気に入りキャラを変えると次の取得から `character` 列の値が変わる。

## 3. データモデル

### 新規テーブル: `play_stats_snapshots`

```sql
CREATE TABLE IF NOT EXISTS play_stats_snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL,
  character TEXT NOT NULL,
  match_replay_id TEXT,
  snapshot_at TEXT DEFAULT (DATETIME('NOW')) NOT NULL,

  -- battle_stats (over last 100 ranked matches)
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

  -- base_info content_play_time_list (seconds per content_type)
  world_tour_seconds INTEGER,
  ranked_match_seconds INTEGER,
  casual_match_seconds INTEGER,
  custom_room_seconds INTEGER,
  battle_hub_seconds INTEGER,
  offline_match_seconds INTEGER,
  arcade_seconds INTEGER,
  practice_seconds INTEGER,
  extreme_seconds INTEGER,

  FOREIGN KEY (user_id) REFERENCES users(code),
  FOREIGN KEY (match_replay_id) REFERENCES matches(replay_id)
);

CREATE INDEX IF NOT EXISTS idx_play_stats_user_char_at
  ON play_stats_snapshots(user_id, character, snapshot_at);
```

カラム合計 55 (meta 5 + battle_stats 38 + enjoy 3 + content_play_time 9)。

- `user_id` を Phase B で他ユーザーのスナップショットにも流用する前提
- `character` 単位で時系列を引けるよう INDEX を貼る
- `match_replay_id` で対応マッチに紐付け (重複防止には `UNIQUE(user_id, character, match_replay_id)` も検討するが、お気に入りキャラ切替時の重複保存を許容したいので一旦 INDEX のみ)

### マイグレーション

- `000004_add_play_stats_snapshots.up.sql` — 上記 CREATE 文
- `000004_add_play_stats_snapshots.down.sql` — `DROP TABLE play_stats_snapshots`

## 4. 取得ロジック

### 取得タイミング

既存の `cmd.TrackingHandler.StartTracking` ポーリングループ内で:

1. 30 秒ごとに `gameTracker.Poll` を実行 (既存)
2. `replay_id` が前回と異なる = 新マッチ検知 (既存)
3. **新規**: 同タイミングで `cfnClient.GetPlayStats(ctx, userId)` を実行
4. **新規**: `playStatsStorage.SavePlayStats(...)` で永続化
5. 既存の `eventEmitter("match", ...)` などは変更なし

### エラーハンドリング

`/play` 取得・保存が失敗しても **match 保存は続行**する (graceful degradation)。

```go
if playStats, err := ch.cfnClient.GetPlayStats(ctx, session.UserId); err != nil {
    slog.Warn("get play stats failed, skipping snapshot", slog.Any("error", err))
} else if err := ch.sqlDb.SavePlayStats(ctx, playStats, match.ReplayID); err != nil {
    slog.Warn("save play stats failed", slog.Any("error", err))
}
```

差分計算は隣接スナップショットでしか機能しないので、欠落がある時は 2 試合分まとめた寄与として読み取る (集計時のクライアント責務)。

### スクレイピング詳細

- URL: `https://www.streetfighter.com/6/buckler/profile/<user_code>/play`
- 抽出: `#__NEXT_DATA__` の textContent を JSON.parse、`props.pageProps.play` を取り出す
- 失敗判定: `common.statusCode != 200` または `play == nil`

## 5. UI ダッシュボード

### 新ページ `/stats`

サイドバーに「実績推移」リンクを追加し、ルート `/stats` を新設。

### レイアウト概要

```
┌─────────────────────────────────────────────────────┐
│ ヘッダ: ユーザー [選択▼] / キャラ [選択▼] / 期間 [選択▼]│
├─────────────────────────────────────────────────────┤
│ KPI カード 6 枚 (現在値 + 前回からの増減 ↑↓)          │
│  ┌─────────┐┌─────────┐┌─────────┐                  │
│  │ DI 命中 ││ DI 被弾 ││ ジャパリ│ ...               │
│  │  1.2回  ││  1.9回  ││  0.0回  │                  │
│  │ +0.3 ↑  ││ -0.2 ↓  ││ ─       │                  │
│  └─────────┘└─────────┘└─────────┘                  │
├─────────────────────────────────────────────────────┤
│ [折れ線グラフ] 主要 KPI の時系列推移 (重ね描き)        │
├─────────────────────────────────────────────────────┤
│ [▼ 全項目を見る] (clickで展開)                        │
│   全 55 列のスナップショット時系列テーブル              │
└─────────────────────────────────────────────────────┘
```

### 主要 KPI (KPI カード + グラフ対象)

実装者判断で以下を初期セットとし、使いながら調整する:

1. **drive_impact** (DI 命中) — 攻撃起点指標
2. **received_drive_impact** (DI 被弾) — DI 確認力
3. **just_parry** (ジャストパリィ) — 上級者度
4. **throw_tech** (投げ抜け) — 防御力
5. **corner_time** (壁際追い詰め秒数) — 場所取り
6. **gauge_rate_sa_lv3** (SA Lv3 使用率) — リソース管理判断

### グラフライブラリ

`recharts` を採用 (React 19 互換、TypeScript ネイティブ、`bun add recharts`)。

## 6. 実装ステップ

1. **データモデル**
   - `pkg/tracker/sf6/cfn/model.go` に `PlayPage` / `BattleStats` / `BaseInfo` 構造体追加
   - `pkg/model/play_stats.go` 新設 (DB マッピング、`db:` タグ付き)
2. **取得層**
   - `pkg/tracker/sf6/cfn/client.go` に `GetPlayStats(ctx, cfn) (*BattleStats, *BaseInfo, error)` を追加
   - 戻り値はキャラ名を `fighter_banner_info.favorite_character_tool_name` から取得して合わせる
3. **永続化層**
   - SQL マイグレーション `000004_add_play_stats_snapshots.{up,down}.sql`
   - `pkg/storage/sql/play_stats.go` (`SavePlayStats`, `GetPlayStatsHistory(userId, character, limit uint16)`)
4. **トラッキング統合**
   - `pkg/tracker/sf6/track.go` に新メソッド `PollPlayStats(ctx, session) (*BattleStats, *BaseInfo, string, error)` を追加 (キャラ名も返す)。既存 `Poll` シグネチャと `tracker.GameTracker` インターフェースは変更しない (T8 側を巻き込まないため)
   - `cmd/tracking.go` の `StartTracking` ループで、新マッチ保存に成功した直後に `PollPlayStats` を呼び出して保存。失敗時は warning ログのみで継続
5. **Wails Bind**
   - `cmd/cmd.go` に `GetPlayStatsHistory(userId, character string, limit uint16)` を追加 (Read API)
6. **GUI**
   - `bun add recharts`
   - `app/gui/src/pages/stats.tsx` 新設
   - `app/gui/src/main/router.tsx` に `/stats` ルート追加
   - `app/gui/src/main/app-sidebar.tsx` にリンク追加
   - `en-GB.json` / `ja-JP.json` / `fr-FR.json` に新規 i18n キー追加 (`stats`, `kpiDriveImpact`, `kpiJustParry`, 等)
7. **クリーンアップ**
   - `app/tools/dump-play-stats/` をリポに含めるか判断 (Phase B でも使う予定なので残す方向)
   - `play-stats-sample.json` は gitignore に追加してローカル限定

## 7. テスト

- `pkg/tracker/sf6/cfn/client_test.go` 新設: `BattleStats` JSON unmarshal の正当性テスト (固定 JSON フィクスチャ)
- `pkg/storage/sql/play_stats_test.go` 新設: in-memory SQLite で SavePlayStats → GetPlayStatsHistory のラウンドトリップ
- 既存テストは変更なし

## 8. リスク・前提

- Capcom 側 `/play` ページの DOM 構造 (`#__NEXT_DATA__` パターン) が将来変わると壊れる
- `fighter_banner_info.favorite_character_tool_name` がお気に入りキャラ依存なので、ユーザーがゲーム内で頻繁に切り替えるとスナップショットがキャラ別に分散する。`character` 列で識別すれば分析可能
- 既存 `Poll` 1 回ごとにリクエストが `/battlelog/rank` + `/play` の 2 倍に増えるが、新マッチ検知時のみなので実質ほぼ問題なし
- `play_stats_snapshots.match_replay_id` は FK 制約あり。match 保存が成功した後に play stats を保存する順序を守る

## 9. Phase B プレビュー (実装は別 PR)

- 設定画面で「ベンチマーク対象 CFN コード」を複数登録できるようにする
- 1 日 1 回など低頻度で他ユーザーの `/play` を取得して同じ `play_stats_snapshots` テーブルに保存
- `/stats` 画面で「自分 vs 平均」をカード/グラフに重ね表示
- `user_id` 列で自分以外も区別できるので、本 Phase のスキーマで対応可能
