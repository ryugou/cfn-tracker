package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

const adviceInputWindow = 30

type adviceSignal struct {
	key         string
	label       string
	self        float64
	benchmark   float64
	trend       float64
	higherGood  bool
	severity    float64
	description string
}

func (ch *CommandHandler) GenerateAdviceComparison(userId, character string) (*model.AdviceRun, error) {
	ctx := context.Background()
	latest, err := ch.sqlDb.GetLatestPlayStatsSnapshot(ctx, userId)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	if latest == nil {
		return nil, model.WrapError(model.ErrGetPlayStats, fmt.Errorf("play stats are empty"))
	}

	history, err := ch.sqlDb.GetRecentPlayStatsSnapshots(ctx, userId, adviceInputWindow)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	players, err := ch.sqlDb.GetBenchmarkPlayers(ctx, userId, character)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	averages := benchmarkAverages(players)

	signals := adviceSignals(latest, history, averages)
	dbCandidate := buildDBOnlyAdvice(signals)
	graphCandidate := buildGraphRAGAdvice(signals, ch.searchVegapunkEvidence(ctx, character, dbCandidate.Theme, dbCandidate.Summary))

	run := &model.AdviceRun{
		UserId:      userId,
		Character:   character,
		InputWindow: adviceInputWindow,
		SnapshotAt:  latest.SnapshotAt,
		Candidates:  []*model.AdviceCandidate{dbCandidate, graphCandidate},
	}
	if err := ch.sqlDb.SaveAdviceRun(ctx, run); err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return run, nil
}

func (ch *CommandHandler) GetLatestAdviceRun(userId, character string) (*model.AdviceRun, error) {
	run, err := ch.sqlDb.GetLatestAdviceRun(context.Background(), userId, character)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return run, nil
}

func (ch *CommandHandler) SaveAdviceFeedback(runId int64, mode string, rating, specificity, usefulness, trust int, comment string) error {
	err := ch.sqlDb.SaveAdviceFeedback(context.Background(), model.AdviceFeedback{
		RunId:       runId,
		Mode:        model.AdviceMode(mode),
		Rating:      rating,
		Specificity: specificity,
		Usefulness:  usefulness,
		Trust:       trust,
		Comment:     comment,
	})
	if err != nil {
		return model.WrapError(model.ErrGetPlayStats, err)
	}
	return nil
}

func adviceSignals(
	latest *model.PlayStatsSnapshot,
	history []*model.PlayStatsSnapshot,
	averages []model.BenchmarkRankAverage,
) []adviceSignal {
	benchmark := map[int]*model.PlayStatsSnapshot{}
	for _, avg := range averages {
		benchmark[avg.RankOffset] = avg.Stats
	}
	next := benchmark[2]
	if next == nil {
		next = benchmark[1]
	}

	rows := []adviceSignal{
		makeSignal("received_drive_impact", "DI被弾", latest.ReceivedDriveImpact, avgValue(next, "received_drive_impact"), trend(history, func(s *model.PlayStatsSnapshot) float64 { return s.ReceivedDriveImpact }), false, "相手のドライブインパクトを受ける頻度"),
		makeSignal("just_parry", "ジャストパリィ", latest.JustParry, avgValue(next, "just_parry"), trend(history, func(s *model.PlayStatsSnapshot) float64 { return s.JustParry }), true, "防御時に精密な受けを作れている頻度"),
		makeSignal("throw_tech", "投げ抜け", latest.ThrowTech, avgValue(next, "throw_tech"), trend(history, func(s *model.PlayStatsSnapshot) float64 { return s.ThrowTech }), true, "近距離防御で投げに対応できている頻度"),
		makeSignal("cornered_time", "壁際に追い詰められた時間", latest.CorneredTime, avgValue(next, "cornered_time"), trend(history, func(s *model.PlayStatsSnapshot) float64 { return s.CorneredTime }), false, "守勢で画面端に留まっている時間"),
		makeSignal("received_punish_counter", "パニカン被弾", latest.ReceivedPunishCounter, avgValue(next, "received_punish_counter"), trend(history, func(s *model.PlayStatsSnapshot) float64 { return s.ReceivedPunishCounter }), false, "技振りや暴れを咎められている頻度"),
		makeSignal("throw_count", "投げ", latest.ThrowCount, avgValue(next, "throw_count"), trend(history, func(s *model.PlayStatsSnapshot) float64 { return s.ThrowCount }), true, "攻めで投げ択を見せられている頻度"),
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].severity > rows[j].severity
	})
	return rows
}

func makeSignal(key, label string, self, benchmark, trend float64, higherGood bool, description string) adviceSignal {
	gap := 0.0
	if benchmark > 0 {
		gap = (self - benchmark) / benchmark
		if higherGood {
			gap = -gap
		}
	}
	if benchmark == 0 {
		if higherGood {
			gap = -trend
		} else {
			gap = trend
		}
	}
	if gap < 0 {
		gap = 0
	}
	return adviceSignal{
		key: key, label: label, self: self, benchmark: benchmark, trend: trend,
		higherGood: higherGood, severity: gap, description: description,
	}
}

func buildDBOnlyAdvice(signals []adviceSignal) *model.AdviceCandidate {
	top := firstMeaningfulSignal(signals)
	c := baseAdvice(top)
	c.Mode = model.AdviceModeDBOnly
	c.Rationale = fmt.Sprintf(
		"現在の%sは %.2f、比較対象平均は %.2f です。直近%d件の推移では %.2f 変化しています。DB上の数値差分だけを見ると、この項目を優先して観察する価値があります。",
		top.label, top.self, top.benchmark, adviceInputWindow, top.trend,
	)
	c.Evidence = []model.AdviceEvidence{
		{Source: "db", Title: top.label, Text: fmt.Sprintf("%s: 自分 %.2f / 比較対象 %.2f / 推移 %.2f", top.description, top.self, top.benchmark, top.trend)},
	}
	return c
}

func buildGraphRAGAdvice(signals []adviceSignal, evidence []model.AdviceEvidence) *model.AdviceCandidate {
	top := firstMeaningfulSignal(signals)
	c := baseAdvice(top)
	c.Mode = model.AdviceModeGraphRAG
	c.Rationale = fmt.Sprintf(
		"DB上は%sが優先課題です。GraphRAG側では、この数値変化を攻略知識・副作用候補・過去施策の根拠と接続して検証します。根拠が不足する場合は断定せず、次回の監視指標を増やします。",
		top.label,
	)
	c.Risks = fmt.Sprintf("%sだけを追うと、別の行動量が落ちる可能性があります。投げ、パニカン被弾、壁際時間を副作用指標として同時に見ます。", top.label)
	c.Evidence = append([]model.AdviceEvidence{
		{Source: "db", Title: top.label, Text: fmt.Sprintf("自分 %.2f / 比較対象 %.2f / 推移 %.2f", top.self, top.benchmark, top.trend)},
	}, evidence...)
	return c
}

func baseAdvice(top adviceSignal) *model.AdviceCandidate {
	switch top.key {
	case "received_drive_impact":
		return &model.AdviceCandidate{
			Priority: "高", Theme: "DI被弾を減らす",
			Summary:         "相手のDIを受ける頻度が比較対象より高いため、端付近での技振りとDI返しを重点的に見ます。",
			Action:          "次の30試合は、端付近でキャンセル不能技を振る回数を減らし、相手がDIを押しそうな距離では様子見かキャンセル可能技を優先します。",
			Drill:           "トレモで相手DIを記録し、よく使う牽制からDI返しできる距離を5分確認してからランクマに入ります。",
			SuccessCriteria: "DI被弾を比較対象平均に近づける。投げ回数とパニカン被弾が同時に悪化していないことも確認します。",
			WatchMetrics:    "DI被弾, 投げ, パニカン被弾, 壁際に追い詰められた時間",
		}
	case "throw_tech":
		return &model.AdviceCandidate{
			Priority: "中", Theme: "近距離防御の投げ対応を戻す",
			Summary:         "投げ抜けが比較対象より低いため、守りでパリィやガードに寄りすぎていないかを確認します。",
			Action:          "次の30試合は、近距離で毎回パリィに逃げず、歩き投げが多い相手には遅らせ投げ抜けを選択肢に戻します。",
			Drill:           "打撃重ねと投げの2択を記録して、遅らせ投げ抜けを10分練習します。",
			SuccessCriteria: "投げ抜けを改善しつつ、DI被弾とパニカン被弾を悪化させない。",
			WatchMetrics:    "投げ抜け, 投げられ, DI被弾, パニカン被弾",
		}
	case "throw_count":
		return &model.AdviceCandidate{
			Priority: "中", Theme: "攻めで投げ択を増やす",
			Summary:         "投げ回数が比較対象より少ないため、端や有利状況で相手に投げを意識させる回数を増やします。",
			Action:          "端で有利を取ったら、最低1回は歩き投げを見せます。その後は下がり打撃でグラップ狩りも狙います。",
			Drill:           "端の有利状況から投げと下がり打撃の2択を練習します。",
			SuccessCriteria: "投げ回数を増やし、パニカン被弾とDI被弾が増えすぎていないことを確認します。",
			WatchMetrics:    "投げ, パニカン, DI被弾, 壁際追い詰め時間",
		}
	default:
		return &model.AdviceCandidate{
			Priority: "中", Theme: fmt.Sprintf("%sを改善する", top.label),
			Summary:         fmt.Sprintf("%sが比較対象との差分として目立っています。", top.label),
			Action:          fmt.Sprintf("次の30試合は%sを意識し、試合後に変化を確認します。", top.label),
			Drill:           fmt.Sprintf("%sに関連する状況をトレモで短時間確認してからランクマに入ります。", top.label),
			SuccessCriteria: fmt.Sprintf("%sが改善し、副作用指標が大きく悪化しないこと。", top.label),
			WatchMetrics:    fmt.Sprintf("%s, DI被弾, 投げ, パニカン被弾", top.label),
		}
	}
}

func firstMeaningfulSignal(signals []adviceSignal) adviceSignal {
	for _, signal := range signals {
		if signal.severity > 0.05 {
			return signal
		}
	}
	if len(signals) > 0 {
		return signals[0]
	}
	return adviceSignal{key: "received_drive_impact", label: "DI被弾", higherGood: false}
}

func avgValue(stats *model.PlayStatsSnapshot, key string) float64 {
	if stats == nil {
		return 0
	}
	switch key {
	case "received_drive_impact":
		return stats.ReceivedDriveImpact
	case "just_parry":
		return stats.JustParry
	case "throw_tech":
		return stats.ThrowTech
	case "cornered_time":
		return stats.CorneredTime
	case "received_punish_counter":
		return stats.ReceivedPunishCounter
	case "throw_count":
		return stats.ThrowCount
	default:
		return 0
	}
}

func trend(rows []*model.PlayStatsSnapshot, pick func(*model.PlayStatsSnapshot) float64) float64 {
	if len(rows) < 2 {
		return 0
	}
	return pick(rows[len(rows)-1]) - pick(rows[0])
}

func (ch *CommandHandler) searchVegapunkEvidence(ctx context.Context, character, theme, summary string) []model.AdviceEvidence {
	target := os.Getenv("VEGAPUNK_GRPC_TARGET")
	if target == "" {
		target = "vegapunk:6840"
	}
	schema := os.Getenv("VEGAPUNK_SCHEMA")
	if schema == "" {
		schema = "sf6-advice"
	}
	query := fmt.Sprintf("SF6 %s %s %s", character, theme, summary)
	req := map[string]any{
		"text":   query,
		"mode":   "hybrid",
		"schema": schema,
		"top_k":  5,
		"limit":  5,
	}
	body, _ := json.Marshal(req)
	args := []string{"-plaintext", "-d", string(body)}
	if token := os.Getenv("VEGAPUNK_TOKEN"); token != "" {
		args = append(args, "-H", "authorization: Bearer "+token)
	}
	args = append(args, target, "graphrag.GraphRAGEngine/Search")

	cmdCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "grpcurl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = err.Error()
		}
		return []model.AdviceEvidence{{Source: "vegapunk", Title: "GraphRAG検索未接続", Text: text}}
	}
	var res struct {
		Results []struct {
			Type    string  `json:"type"`
			Text    string  `json:"text"`
			Summary string  `json:"summary"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return []model.AdviceEvidence{{Source: "vegapunk", Title: "GraphRAG応答解析失敗", Text: err.Error()}}
	}
	evidence := make([]model.AdviceEvidence, 0, len(res.Results))
	for _, row := range res.Results {
		text := row.Text
		if text == "" {
			text = row.Summary
		}
		evidence = append(evidence, model.AdviceEvidence{Source: "vegapunk", Title: row.Type, Text: text, Score: row.Score})
	}
	if len(evidence) == 0 {
		evidence = append(evidence, model.AdviceEvidence{Source: "vegapunk", Title: "GraphRAG検索結果なし", Text: query})
	}
	return evidence
}
