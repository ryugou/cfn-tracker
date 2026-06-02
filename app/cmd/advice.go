package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

const (
	adviceInputWindow           = 30
	defaultAnthropicBaseURL     = "https://api.anthropic.com/v1"
	defaultAnthropicVersion     = "2023-06-01"
	defaultAnthropicOpusModel   = "claude-opus-4-6"
	defaultAnthropicSonnetModel = "claude-sonnet-4-6"
	anthropicRequestTimeout     = 90 * time.Second
	anthropicOpusModelEnvKey    = "ADVICE_LLM_OPUS_MODEL"
	anthropicSonnetModelEnvKey  = "ADVICE_LLM_SONNET_MODEL"
	anthropicAPIKeyEnvKey       = "ANTHROPIC_API_KEY"
	anthropicBaseURLEnvKey      = "ANTHROPIC_BASE_URL"
	anthropicVersionEnvKey      = "ANTHROPIC_VERSION"
	anthropicAPIKeyOPRefEnvKey  = "ANTHROPIC_API_KEY_OP_REF"
	vegapunkSearchTimeoutEnvKey = "VEGAPUNK_SEARCH_TIMEOUT_SECONDS"
)

var adviceHTTPClient = http.DefaultClient
var adviceLLMConfigMu sync.Mutex

type adviceContext struct {
	UserID         string                  `json:"userId"`
	Character      string                  `json:"character"`
	InputWindow    int                     `json:"inputWindow"`
	SnapshotAt     string                  `json:"snapshotAt"`
	Signals        []adviceSignalForPrompt `json:"signals"`
	BenchmarkCount int                     `json:"benchmarkCount"`
}

type adviceSignalForPrompt struct {
	Key         string  `json:"key"`
	Label       string  `json:"label"`
	Description string  `json:"description"`
	Self        float64 `json:"self"`
	Benchmark   float64 `json:"benchmark"`
	Trend       float64 `json:"trend"`
	HigherGood  bool    `json:"higherGood"`
	Severity    float64 `json:"severity"`
}

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
	adviceCtx := buildAdviceContext(userId, character, latest, signals, len(players))
	sonnetModel := adviceLLMModel(anthropicSonnetModelEnvKey, defaultAnthropicSonnetModel)
	opusModel := adviceLLMModel(anthropicOpusModelEnvKey, defaultAnthropicOpusModel)
	apiKeyErr := InitializeAdviceLLMConfig(ctx)
	apiKey := strings.TrimSpace(os.Getenv(anthropicAPIKeyEnvKey))

	dbFallback := buildDBOnlyAdvice(signals)
	dbCandidate := ch.generateAdviceWithLLM(ctx, model.AdviceModeDBOnly, sonnetModel, apiKey, apiKeyErr, adviceCtx, nil, dbFallback)
	graphEvidence := ch.searchVegapunkEvidence(ctx, character, dbCandidate.Theme, dbCandidate.Summary)
	graphSonnetFallback := buildGraphRAGAdvice(signals, graphEvidence)
	graphSonnetCandidate := ch.generateAdviceWithLLM(ctx, model.AdviceModePunkRecordSonnet46, sonnetModel, apiKey, apiKeyErr, adviceCtx, graphEvidence, graphSonnetFallback)
	graphOpusFallback := buildGraphRAGAdvice(signals, graphEvidence)
	graphOpusCandidate := ch.generateAdviceWithLLM(ctx, model.AdviceModePunkRecordOpus46, opusModel, apiKey, apiKeyErr, adviceCtx, graphEvidence, graphOpusFallback)

	run := &model.AdviceRun{
		UserId:      userId,
		Character:   character,
		InputWindow: adviceInputWindow,
		SnapshotAt:  latest.SnapshotAt,
		Candidates:  []*model.AdviceCandidate{graphOpusCandidate, graphSonnetCandidate, dbCandidate},
	}
	if err := ch.sqlDb.SaveAdviceRun(ctx, run); err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	go ch.syncAdviceRunToVegapunk(context.Background(), run)
	return run, nil
}

func (ch *CommandHandler) GetLatestAdviceRun(userId, character string) (*model.AdviceRun, error) {
	run, err := ch.sqlDb.GetLatestAdviceRun(context.Background(), userId, character)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return run, nil
}

func (ch *CommandHandler) GetAdviceRuns(userId, character string, limit int) ([]*model.AdviceRun, error) {
	runs, err := ch.sqlDb.GetAdviceRuns(context.Background(), userId, character, limit)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return runs, nil
}

func (ch *CommandHandler) DeleteAdviceRun(runId int64) error {
	err := ch.sqlDb.DeleteAdviceRun(context.Background(), runId)
	if err != nil {
		return model.WrapError(model.ErrGetPlayStats, err)
	}
	return nil
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

func buildAdviceContext(
	userId, character string,
	latest *model.PlayStatsSnapshot,
	signals []adviceSignal,
	benchmarkCount int,
) adviceContext {
	out := adviceContext{
		UserID:         userId,
		Character:      character,
		InputWindow:    adviceInputWindow,
		SnapshotAt:     latest.SnapshotAt,
		BenchmarkCount: benchmarkCount,
		Signals:        make([]adviceSignalForPrompt, 0, len(signals)),
	}
	for _, signal := range signals {
		out.Signals = append(out.Signals, adviceSignalForPrompt{
			Key:         signal.key,
			Label:       signal.label,
			Description: signal.description,
			Self:        round2(signal.self),
			Benchmark:   round2(signal.benchmark),
			Trend:       round2(signal.trend),
			HigherGood:  signal.higherGood,
			Severity:    round2(signal.severity),
		})
	}
	return out
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
		"DB上は%sが優先課題です。PunkRecord側では、この数値変化を攻略知識・副作用候補・過去施策の根拠と接続して検証します。根拠が不足する場合は断定せず、次回の監視指標を増やします。",
		top.label,
	)
	c.Risks = fmt.Sprintf("%sだけを追うと、別の行動量が落ちる可能性があります。投げ、パニカン被弾、壁際時間を副作用指標として同時に見ます。", top.label)
	c.Evidence = append([]model.AdviceEvidence{
		{Source: "db", Title: top.label, Text: fmt.Sprintf("自分 %.2f / 比較対象 %.2f / 推移 %.2f", top.self, top.benchmark, top.trend)},
	}, evidence...)
	return c
}

func (ch *CommandHandler) generateAdviceWithLLM(
	ctx context.Context,
	mode model.AdviceMode,
	modelName string,
	apiKey string,
	apiKeyErr error,
	adviceCtx adviceContext,
	graphEvidence []model.AdviceEvidence,
	fallback *model.AdviceCandidate,
) *model.AdviceCandidate {
	fallback.Mode = mode
	candidate, err := requestAdviceLLM(ctx, mode, modelName, apiKey, apiKeyErr, adviceCtx, graphEvidence)
	if err != nil {
		fallback.Evidence = append(fallback.Evidence, model.AdviceEvidence{
			Source: "llm",
			Title:  "LLM結果使用不可",
			Text:   err.Error(),
		})
		return fallback
	}
	candidate.Mode = mode
	if len(candidate.Evidence) == 0 {
		candidate.Evidence = fallback.Evidence
	}
	return candidate
}

func requestAdviceLLM(
	ctx context.Context,
	mode model.AdviceMode,
	modelName string,
	apiKey string,
	apiKeyErr error,
	adviceCtx adviceContext,
	graphEvidence []model.AdviceEvidence,
) (*model.AdviceCandidate, error) {
	if apiKeyErr != nil {
		return nil, apiKeyErr
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("%s is empty", anthropicAPIKeyEnvKey)
	}
	if strings.HasPrefix(strings.TrimSpace(apiKey), "op://") {
		return nil, fmt.Errorf("%s is still a 1Password reference; startup resolution has not completed or failed", anthropicAPIKeyEnvKey)
	}
	system := adviceSystemPrompt(mode)
	user, err := adviceUserPrompt(mode, adviceCtx, graphEvidence)
	if err != nil {
		return nil, err
	}
	reqBody := map[string]any{
		"model":       modelName,
		"max_tokens":  3200,
		"temperature": 0.2,
		"system":      system,
		"messages": []map[string]string{
			{"role": "user", "content": user},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal llm request: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, anthropicRequestTimeout)
	defer cancel()
	baseURL := strings.TrimRight(os.Getenv(anthropicBaseURLEnvKey), "/")
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	anthropicVersion := strings.TrimSpace(os.Getenv(anthropicVersionEnvKey))
	if anthropicVersion == "" {
		anthropicVersion = defaultAnthropicVersion
	}
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := adviceHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call llm: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read llm response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("llm status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse llm response: %w", err)
	}
	content := strings.Builder{}
	for _, block := range parsed.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}
	if strings.TrimSpace(content.String()) == "" {
		return nil, fmt.Errorf("llm response is empty")
	}
	if parsed.StopReason == "max_tokens" {
		return nil, fmt.Errorf("llm response was truncated at max_tokens")
	}
	candidate, err := parseAdviceCandidateJSON(content.String())
	if err != nil {
		return nil, err
	}
	candidate.Evidence = append(dbEvidenceFromContext(adviceCtx), graphEvidence...)
	candidate.Evidence = append(candidate.Evidence, model.AdviceEvidence{
		Source: "llm",
		Title:  modelName,
		Text:   "Anthropic Messages APIで生成しました。",
	})
	return candidate, nil
}

func InitializeAdviceLLMConfig(ctx context.Context) error {
	adviceLLMConfigMu.Lock()
	defer adviceLLMConfigMu.Unlock()

	apiKey := strings.TrimSpace(os.Getenv(anthropicAPIKeyEnvKey))
	if apiKey != "" && !strings.HasPrefix(apiKey, "op://") {
		return nil
	}
	opRef := strings.TrimSpace(os.Getenv(anthropicAPIKeyOPRefEnvKey))
	if strings.HasPrefix(apiKey, "op://") {
		opRef = apiKey
	}
	if opRef == "" {
		return nil
	}
	cmd := exec.CommandContext(ctx, "op", "read", opRef)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("1Password reference %q could not be read: %w: %s", opRef, err, detail)
		}
		return fmt.Errorf("1Password reference %q could not be read: %w", opRef, err)
	}
	apiKey = strings.TrimSpace(string(out))
	if apiKey == "" {
		return fmt.Errorf("1Password reference %q returned an empty Anthropic API key", opRef)
	}
	return os.Setenv(anthropicAPIKeyEnvKey, apiKey)
}

func adviceLLMModel(envKey, fallback string) string {
	if modelName := strings.TrimSpace(os.Getenv(envKey)); modelName != "" {
		return modelName
	}
	return fallback
}

func adviceSystemPrompt(mode model.AdviceMode) string {
	modeInstruction := "DB観測データだけを根拠にして、攻略知識を推測で広げすぎないでください。"
	if usesPunkRecordEvidence(mode) {
		modeInstruction = "DB観測データを主根拠にし、PunkRecord/GraphRAG evidenceを追加根拠として使ってください。矛盾する場合はDB観測データを優先してください。"
	}
	return strings.Join([]string{
		"あなたはStreet Fighter 6の分析コーチです。",
		"目的は、プレイヤーの現在値、直近推移、ベンチマーク差分から、次に実行する施策カードを1つ作ることです。",
		"観測事実と推定を分け、因果を断定しすぎないでください。",
		"「因果的に連動」「証拠となる」「証明する」など、因果証明を示す表現は禁止です。「関連している可能性」「改善を示唆する」に留めてください。",
		"短期間で言うことを変えすぎず、成功条件と副作用として監視する指標を必ず含めてください。",
		modeInstruction,
		"返答はJSONのみ。Markdownや説明文は不要です。",
		"JSON keys: priority, theme, summary, rationale, action, drill, successCriteria, watchMetrics, risks",
		"各JSON valueは文字列にしてください。配列やオブジェクトは使わず、複数項目は改行区切りの文字列にしてください。",
	}, "\n")
}

func adviceUserPrompt(mode model.AdviceMode, adviceCtx adviceContext, graphEvidence []model.AdviceEvidence) (string, error) {
	payload := map[string]any{
		"mode":        mode,
		"db_context":  adviceCtx,
		"output_note": "全フィールドは日本語で、実行可能な具体性を持たせてください。",
	}
	if usesPunkRecordEvidence(mode) {
		payload["graph_rag_evidence"] = graphEvidence
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal advice prompt context: %w", err)
	}
	return string(b), nil
}

func usesPunkRecordEvidence(mode model.AdviceMode) bool {
	return mode == model.AdviceModeGraphRAG ||
		mode == model.AdviceModePunkRecordOpus46 ||
		mode == model.AdviceModePunkRecordSonnet46
}

func parseAdviceCandidateJSON(content string) (*model.AdviceCandidate, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var out struct {
		Priority        adviceJSONText `json:"priority"`
		Theme           adviceJSONText `json:"theme"`
		Summary         adviceJSONText `json:"summary"`
		Rationale       adviceJSONText `json:"rationale"`
		Action          adviceJSONText `json:"action"`
		Drill           adviceJSONText `json:"drill"`
		SuccessCriteria adviceJSONText `json:"successCriteria"`
		WatchMetrics    adviceJSONText `json:"watchMetrics"`
		Risks           adviceJSONText `json:"risks"`
	}
	if err := json.Unmarshal([]byte(content), &out); err != nil {
		return nil, fmt.Errorf("parse advice json: %w", err)
	}
	if out.Theme.String() == "" || out.Action.String() == "" {
		return nil, fmt.Errorf("advice json is missing required fields")
	}
	return &model.AdviceCandidate{
		Priority:        out.Priority.String(),
		Theme:           out.Theme.String(),
		Summary:         out.Summary.String(),
		Rationale:       out.Rationale.String(),
		Action:          out.Action.String(),
		Drill:           out.Drill.String(),
		SuccessCriteria: out.SuccessCriteria.String(),
		WatchMetrics:    out.WatchMetrics.String(),
		Risks:           out.Risks.String(),
	}, nil
}

type adviceJSONText string

func (t *adviceJSONText) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*t = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*t = adviceJSONText(s)
		return nil
	}
	var n json.Number
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&n); err == nil {
		*t = adviceJSONText(n.String())
		return nil
	}
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		*t = adviceJSONText(fmt.Sprintf("%t", b))
		return nil
	}
	text, err := adviceJSONValueToText(data)
	if err != nil {
		return err
	}
	*t = adviceJSONText(text)
	return nil
}

func (t adviceJSONText) String() string {
	return strings.TrimSpace(string(t))
}

func adviceJSONValueToText(data []byte) (string, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("expected JSON value, got %s", strings.TrimSpace(string(data)))
	}
	return strings.TrimSpace(formatAdviceJSONValue(value, 0)), nil
}

func formatAdviceJSONValue(value any, depth int) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return fmt.Sprintf("%t", v)
	case []any:
		lines := make([]string, 0, len(v))
		for _, item := range v {
			text := strings.TrimSpace(formatAdviceJSONValue(item, depth+1))
			if text != "" {
				lines = append(lines, "- "+text)
			}
		}
		return strings.Join(lines, "\n")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			text := strings.TrimSpace(formatAdviceJSONValue(v[key], depth+1))
			if text != "" {
				lines = append(lines, fmt.Sprintf("%s: %s", key, text))
			}
		}
		return strings.Join(lines, "\n")
	default:
		return fmt.Sprint(v)
	}
}

func dbEvidenceFromContext(adviceCtx adviceContext) []model.AdviceEvidence {
	evidence := make([]model.AdviceEvidence, 0, len(adviceCtx.Signals))
	for _, signal := range adviceCtx.Signals {
		evidence = append(evidence, model.AdviceEvidence{
			Source: "db",
			Title:  signal.Label,
			Text: fmt.Sprintf(
				"%s: 自分 %.2f / 比較対象 %.2f / 直近%d件の推移 %.2f",
				signal.Description, signal.Self, signal.Benchmark, adviceCtx.InputWindow, signal.Trend,
			),
		})
	}
	return evidence
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

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func (ch *CommandHandler) searchVegapunkEvidence(ctx context.Context, character, theme, summary string) []model.AdviceEvidence {
	target := os.Getenv("VEGAPUNK_GRPC_TARGET")
	if target == "" {
		target = "vegapunk.local:6840"
	}
	schema := os.Getenv("VEGAPUNK_SCHEMA")
	if schema == "" {
		schema = "sf6-advice"
	}
	protoPath := os.Getenv("VEGAPUNK_PROTO")
	if protoPath == "" {
		protoPath = "/Users/ryugo/Developer/src/AI-Project/vegapunk/proto/graphrag.proto"
	}
	protoImportPath := os.Getenv("VEGAPUNK_PROTO_IMPORT_PATH")
	if protoImportPath == "" {
		protoImportPath = filepath.Dir(protoPath)
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
	args := []string{"-plaintext", "-import-path", protoImportPath, "-proto", filepath.Base(protoPath), "-d", string(body)}
	if token := os.Getenv("VEGAPUNK_TOKEN"); token != "" {
		args = append(args, "-H", "authorization: Bearer "+token)
	}
	args = append(args, target, "graphrag.GraphRAGEngine/Search")

	cmdCtx, cancel := context.WithTimeout(ctx, vegapunkSearchTimeout())
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
		if cmdCtx.Err() == context.DeadlineExceeded {
			text = fmt.Sprintf("vegapunk search timed out after %s", vegapunkSearchTimeout())
		}
		if strings.Contains(text, "schema error: schema") && strings.Contains(text, "not found") {
			return []model.AdviceEvidence{{
				Source: "vegapunk",
				Title:  "PunkRecord schema未作成",
				Text:   fmt.Sprintf("VEGAPUNK_SCHEMA=%s はまだvegapunk側に作成されていません。SF6攻略知識をingestするschema作成後に検索結果が使われます。", schema),
			}}
		}
		return []model.AdviceEvidence{{Source: "vegapunk", Title: "GraphRAG検索未接続", Text: text}}
	}
	var res struct {
		Results []struct {
			Type    string  `json:"type"`
			ID      string  `json:"id"`
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
		if isPunkRecordSearchNoise(row.ID, row.Type, text) {
			continue
		}
		title := row.Type
		if title == "" || title == "message" {
			title = firstLine(text)
		}
		evidence = append(evidence, model.AdviceEvidence{Source: "vegapunk", Title: title, Text: text, Score: row.Score})
	}
	if len(evidence) == 0 {
		evidence = append(evidence, model.AdviceEvidence{Source: "vegapunk", Title: "GraphRAG検索結果なし", Text: query})
	}
	return evidence
}

func isPunkRecordSearchNoise(id, nodeType, text string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	nodeType = strings.ToLower(strings.TrimSpace(nodeType))
	text = strings.TrimSpace(text)
	if strings.Contains(id, ":advice_evidence:") ||
		strings.Contains(id, ":advice_candidate:") ||
		strings.Contains(id, ":advice_run:") ||
		strings.Contains(id, ":player:") {
		return true
	}
	if nodeType == "evidence" {
		return strings.HasPrefix(text, "Advice run ") || strings.HasPrefix(text, "CFN player ")
	}
	return false
}

func vegapunkSearchTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv(vegapunkSearchTimeoutEnvKey))
	if value == "" {
		return 30 * time.Second
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(seconds) * time.Second
}
