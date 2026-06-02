package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	"github.com/williamsjokvist/cfn-tracker/pkg/storage/sql"
)

const (
	vegapunkSchemaEnvKey        = "VEGAPUNK_SCHEMA"
	vegapunkGRPCTargetEnvKey    = "VEGAPUNK_GRPC_TARGET"
	vegapunkProtoEnvKey         = "VEGAPUNK_PROTO"
	vegapunkProtoImportEnvKey   = "VEGAPUNK_PROTO_IMPORT_PATH"
	vegapunkTokenEnvKey         = "VEGAPUNK_TOKEN"
	vegapunkGrowthSyncTimeout   = 5 * time.Minute
	vegapunkGrowthBackfillLimit = 200
	vegapunkSyncQueueBatchSize  = 20
)

var vegapunkSyncQueueMu sync.Mutex

type vegapunkNode struct {
	ID         string              `json:"id"`
	Type       string              `json:"type"`
	Attributes []vegapunkAttribute `json:"attributes"`
}

type vegapunkAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type vegapunkEdge struct {
	FromID     string              `json:"from_id"`
	ToID       string              `json:"to_id"`
	Type       string              `json:"type"`
	Attributes []vegapunkAttribute `json:"attributes,omitempty"`
}

type vegapunkVector struct {
	ID       string            `json:"id"`
	Vector   []float64         `json:"vector"`
	Metadata map[string]string `json:"metadata"`
}

type vegapunkGraph struct {
	Nodes   []vegapunkNode   `json:"nodes"`
	Edges   []vegapunkEdge   `json:"edges"`
	Vectors []vegapunkVector `json:"vectors,omitempty"`
}

type growthMetric struct {
	Key        string
	Label      string
	HigherGood bool
	Value      float64
}

type growthObservation struct {
	Key         string
	Label       string
	Direction   string
	Assessment  string
	HigherGood  bool
	HasPrevious bool
}

func (ch *CommandHandler) SyncVegapunkGrowthData(userId, character string) error {
	return ch.syncVegapunkGrowthData(context.Background(), userId, character)
}

func StartVegapunkSyncQueue(ctx context.Context, db *sql.Storage) {
	go processVegapunkSyncQueue(context.Background(), db)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			processVegapunkSyncQueue(context.Background(), db)
		case <-ctx.Done():
			return
		}
	}
}

func (ch *CommandHandler) syncVegapunkGrowthData(ctx context.Context, userId, character string) error {
	if !vegapunkConfigured() {
		return nil
	}
	matches, err := ch.sqlDb.GetMatches(ctx, 0, userId, vegapunkGrowthBackfillLimit, 0)
	if err != nil {
		return fmt.Errorf("load matches for vegapunk sync: %w", err)
	}
	stats, err := ch.sqlDb.GetPlayStatsHistory(ctx, userId, character, "", "", vegapunkGrowthBackfillLimit)
	if err != nil {
		return fmt.Errorf("load play stats for vegapunk sync: %w", err)
	}
	runs, err := ch.sqlDb.GetAdviceRuns(ctx, userId, character, 50)
	if err != nil {
		return fmt.Errorf("load advice runs for vegapunk sync: %w", err)
	}

	graph := vegapunkGraph{}
	playerID := vegapunkPlayerID(userId)
	graph.addNode("Player", playerID, map[string]string{
		"user_id": userId,
		"text":    fmt.Sprintf("CFN player %s", userId),
	})

	for _, match := range matches {
		if match == nil {
			continue
		}
		if character != "" && match.Character != character {
			continue
		}
		graph.addMatch(playerID, *match)
	}
	for i, snap := range stats {
		if snap == nil {
			continue
		}
		var previous *model.PlayStatsSnapshot
		if i > 0 {
			previous = stats[i-1]
		}
		graph.addPlayStatsSnapshot(playerID, *snap, previous)
	}
	for _, run := range runs {
		if run == nil {
			continue
		}
		graph.addAdviceRun(playerID, *run)
	}

	if len(graph.Nodes) == 0 {
		return nil
	}
	if err := enqueueVegapunkGraph(ctx, ch.sqlDb, "backfill", vegapunkDedupeKey("backfill", userId, character), &graph); err != nil {
		return fmt.Errorf("enqueue vegapunk growth data: %w", err)
	}
	return ch.processVegapunkSyncQueue(ctx)
}

func (ch *TrackingHandler) syncMatchToVegapunk(ctx context.Context, match model.Match) {
	graph := vegapunkGraph{}
	playerID := vegapunkPlayerID(match.UserId)
	graph.addNode("Player", playerID, map[string]string{
		"user_id": match.UserId,
		"text":    fmt.Sprintf("CFN player %s", match.UserId),
	})
	graph.addMatch(playerID, match)
	dedupe := vegapunkDedupeKey("match", match.UserId, firstNonEmpty(match.ReplayID, fmt.Sprintf("%d-%s-%s", match.SessionId, match.Date, match.Time)))
	if err := enqueueVegapunkGraph(ctx, ch.sqlDb, "match", dedupe, &graph); err != nil {
		slog.Warn("vegapunk match enqueue failed", slog.String("replay_id", match.ReplayID), slog.Any("error", err))
		return
	}
	go processVegapunkSyncQueue(context.Background(), ch.sqlDb)
}

func (ch *TrackingHandler) syncLatestPlayStatsToVegapunk(ctx context.Context, userId string) {
	rows, err := ch.sqlDb.GetRecentPlayStatsSnapshots(ctx, userId, 2)
	if err != nil {
		slog.Warn("vegapunk play stats sync lookup failed", slog.Any("error", err))
		return
	}
	if len(rows) == 0 || rows[len(rows)-1] == nil {
		return
	}
	var previous *model.PlayStatsSnapshot
	if len(rows) > 1 {
		previous = rows[len(rows)-2]
	}
	graph := vegapunkGraph{}
	playerID := vegapunkPlayerID(userId)
	graph.addNode("Player", playerID, map[string]string{
		"user_id": userId,
		"text":    fmt.Sprintf("CFN player %s", userId),
	})
	graph.addPlayStatsSnapshot(playerID, *rows[len(rows)-1], previous)
	latest := rows[len(rows)-1]
	dedupe := vegapunkDedupeKey("play_stats", userId, strconv.FormatInt(latest.Id, 10), latest.SnapshotAt)
	if err := enqueueVegapunkGraph(ctx, ch.sqlDb, "play_stats", dedupe, &graph); err != nil {
		slog.Warn("vegapunk play stats enqueue failed", slog.Any("error", err))
		return
	}
	go processVegapunkSyncQueue(context.Background(), ch.sqlDb)
}

func (ch *CommandHandler) syncAdviceRunToVegapunk(ctx context.Context, run *model.AdviceRun) {
	if run == nil {
		return
	}
	graph := vegapunkGraph{}
	playerID := vegapunkPlayerID(run.UserId)
	graph.addNode("Player", playerID, map[string]string{
		"user_id": run.UserId,
		"text":    fmt.Sprintf("CFN player %s", run.UserId),
	})
	graph.addAdviceRun(playerID, *run)
	dedupe := vegapunkDedupeKey("advice", run.UserId, run.Character, strconv.FormatInt(run.Id, 10))
	if err := enqueueVegapunkGraph(ctx, ch.sqlDb, "advice", dedupe, &graph); err != nil {
		slog.Warn("vegapunk advice enqueue failed", slog.Int64("run_id", run.Id), slog.Any("error", err))
		return
	}
	go ch.processVegapunkSyncQueue(context.Background())
}

func (ch *CommandHandler) processVegapunkSyncQueue(ctx context.Context) error {
	return processVegapunkSyncQueue(ctx, ch.sqlDb)
}

func enqueueVegapunkGraph(ctx context.Context, db *sql.Storage, kind, dedupeKey string, graph *vegapunkGraph) error {
	if db == nil || graph == nil || len(graph.Nodes) == 0 {
		return nil
	}
	graph.dedupe()
	payload, err := json.Marshal(graph)
	if err != nil {
		return fmt.Errorf("marshal vegapunk sync graph: %w", err)
	}
	if err := db.EnqueueVegapunkSyncJob(ctx, kind, dedupeKey, payload); err != nil {
		return err
	}
	return nil
}

func processVegapunkSyncQueue(ctx context.Context, db *sql.Storage) error {
	if db == nil {
		return nil
	}
	if !vegapunkConfigured() {
		return nil
	}
	if !vegapunkSyncQueueMu.TryLock() {
		return nil
	}
	defer vegapunkSyncQueueMu.Unlock()

	for {
		jobs, err := db.GetDueVegapunkSyncJobs(ctx, vegapunkSyncQueueBatchSize)
		if err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}
		for _, job := range jobs {
			if job == nil {
				continue
			}
			var graph vegapunkGraph
			if err := json.Unmarshal([]byte(job.PayloadJSON), &graph); err != nil {
				if markErr := db.MarkVegapunkSyncJobFailed(ctx, job.Id, job.Attempts+1, err.Error(), nextVegapunkRetryAt(job.Attempts+1)); markErr != nil {
					slog.Warn("mark malformed vegapunk sync job failed", slog.Int64("job_id", job.Id), slog.Any("error", markErr))
				}
				continue
			}
			if err := upsertVegapunkGrowthGraph(ctx, &graph); err != nil {
				attempts := job.Attempts + 1
				if markErr := db.MarkVegapunkSyncJobFailed(ctx, job.Id, attempts, err.Error(), nextVegapunkRetryAt(attempts)); markErr != nil {
					slog.Warn("mark vegapunk sync job failed", slog.Int64("job_id", job.Id), slog.Any("error", markErr))
				}
				slog.Warn("vegapunk sync job failed; queued for retry", slog.Int64("job_id", job.Id), slog.String("kind", job.Kind), slog.Int("attempts", attempts), slog.Any("error", err))
				continue
			}
			if err := db.MarkVegapunkSyncJobDone(ctx, job.Id); err != nil {
				return err
			}
		}
	}
}

func nextVegapunkRetryAt(attempts int) time.Time {
	if attempts < 1 {
		attempts = 1
	}
	delay := time.Duration(1<<min(attempts-1, 6)) * time.Minute
	return time.Now().Add(delay)
}

func vegapunkDedupeKey(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, ":")
}

func (g *vegapunkGraph) addMatch(playerID string, match model.Match) {
	matchKey := match.ReplayID
	if matchKey == "" {
		matchKey = fmt.Sprintf("%d-%s-%s", match.SessionId, match.Date, match.Time)
	}
	matchID := vegapunkID("match", match.UserId, matchKey)
	result := "loss"
	if match.Victory {
		result = "win"
	}
	text := fmt.Sprintf(
		"%s played %s ranked match on %s %s JST: %s vs %s %s. Numeric match facts remain in the local RDB; this node is a context pointer for narrative retrieval.",
		match.UserName, match.Character, match.Date, match.Time, result, match.Opponent, match.OpponentCharacter,
	)
	g.addNode("Match", matchID, map[string]string{
		"user_id":            match.UserId,
		"user_name":          match.UserName,
		"session_id":         strconv.Itoa(int(match.SessionId)),
		"replay_id":          match.ReplayID,
		"character":          match.Character,
		"opponent":           match.Opponent,
		"opponent_character": match.OpponentCharacter,
		"opponent_league":    match.OpponentLeague,
		"result":             result,
		"played_at":          strings.TrimSpace(match.Date + " " + match.Time),
		"rdb_ref":            firstNonEmpty(match.ReplayID, fmt.Sprintf("session:%d", match.SessionId)),
		"text":               text,
	})
	g.addEdge(playerID, matchID, "PLAYED", nil)
}

func (g *vegapunkGraph) addPlayStatsSnapshot(playerID string, snap model.PlayStatsSnapshot, previous *model.PlayStatsSnapshot) {
	snapshotKey := snap.SnapshotAt
	if snap.Id > 0 {
		snapshotKey = strconv.FormatInt(snap.Id, 10)
	}
	snapshotID := vegapunkID("snapshot", snap.UserId, snapshotKey)
	replayID := ""
	if snap.MatchReplayId.Valid {
		replayID = snap.MatchReplayId.String
	}
	text := fmt.Sprintf(
		"Play stats snapshot for user %s character %s at %s JST. Numeric stat values remain in the local RDB and should be read from the RDB/API when exact values are needed.",
		snap.UserId, snap.Character, snap.SnapshotAt,
	)
	g.addNode("PlayStatsSnapshot", snapshotID, map[string]string{
		"user_id":         snap.UserId,
		"character":       snap.Character,
		"match_replay_id": replayID,
		"snapshot_at":     snap.SnapshotAt,
		"rdb_ref":         strconv.FormatInt(snap.Id, 10),
		"text":            text,
	})
	g.addEdge(playerID, snapshotID, "HAS_SNAPSHOT", nil)
	if replayID != "" {
		g.addEdge(vegapunkID("match", snap.UserId, replayID), snapshotID, "HAS_PLAY_STATS", nil)
	}
	observations := growthObservations(snap, previous)
	if len(observations) > 0 {
		summaryID := vegapunkID("observation_summary", snap.UserId, snapshotKey)
		summaryText := playStatsObservationSummaryText(snap, previous, observations)
		g.addNode("ObservationSummary", summaryID, map[string]string{
			"user_id":     snap.UserId,
			"character":   snap.Character,
			"snapshot_at": snap.SnapshotAt,
			"rdb_ref":     strconv.FormatInt(snap.Id, 10),
			"title":       fmt.Sprintf("%s %s play-stat observation summary", snap.Character, snap.SnapshotAt),
			"text":        summaryText,
		})
		g.addEdge(snapshotID, summaryID, "SUMMARIZED_BY", nil)
	}
}

func (g *vegapunkGraph) addAdviceRun(playerID string, run model.AdviceRun) {
	runKey := strconv.FormatInt(run.Id, 10)
	if run.Id == 0 {
		runKey = run.CreatedAt
	}
	runID := vegapunkID("advice_run", run.UserId, runKey)
	text := fmt.Sprintf("Advice run %s for user %s character %s, created at %s. Numeric input windows and snapshot facts remain in the local RDB.", runKey, run.UserId, run.Character, run.CreatedAt)
	g.addNode("AdviceRun", runID, map[string]string{
		"user_id":     run.UserId,
		"character":   run.Character,
		"snapshot_at": run.SnapshotAt,
		"created_at":  run.CreatedAt,
		"rdb_ref":     runKey,
		"text":        text,
	})
	g.addEdge(playerID, runID, "REQUESTED_ADVICE", nil)
	for _, candidate := range run.Candidates {
		if candidate == nil {
			continue
		}
		if candidate.Mode == model.AdviceModeDBOnly {
			continue
		}
		candidateKey := strconv.FormatInt(candidate.Id, 10)
		if candidate.Id == 0 {
			candidateKey = fmt.Sprintf("%s-%s", runKey, candidate.Mode)
		}
		candidateID := vegapunkID("advice_candidate", run.UserId, candidateKey)
		candidateText := strings.Join(nonEmptyStrings(
			fmt.Sprintf("%s advice: %s", candidate.Mode, candidate.Theme),
			candidate.Action,
			candidate.WatchMetrics,
			candidate.Risks,
			"Exact DB metric values and benchmark deltas are intentionally omitted from PunkRecord; query the local RDB for numeric evidence.",
		), "\n")
		g.addNode("AdviceCandidate", candidateID, map[string]string{
			"user_id":       run.UserId,
			"character":     run.Character,
			"mode":          string(candidate.Mode),
			"priority":      candidate.Priority,
			"theme":         candidate.Theme,
			"action":        candidate.Action,
			"watch_metrics": candidate.WatchMetrics,
			"risks":         candidate.Risks,
			"created_at":    candidate.CreatedAt,
			"text":          candidateText,
		})
		g.addEdge(runID, candidateID, "GENERATED_CANDIDATE", map[string]string{"mode": string(candidate.Mode)})
	}
}

func (g *vegapunkGraph) addNode(nodeType, id string, attrs map[string]string) {
	if id == "" {
		return
	}
	nodeType = vegapunkSchemaNodeType(nodeType)
	attrs = vegapunkSchemaAttributes(nodeType, id, attrs)
	g.Nodes = append(g.Nodes, vegapunkNode{ID: id, Type: nodeType, Attributes: vegapunkAttributes(attrs)})
}

func (g *vegapunkGraph) addEdge(fromID, toID, edgeType string, attrs map[string]string) {
	if fromID == "" || toID == "" || edgeType == "" {
		return
	}
	edgeType = vegapunkSchemaEdgeType(edgeType)
	g.Edges = append(g.Edges, vegapunkEdge{FromID: fromID, ToID: toID, Type: edgeType, Attributes: vegapunkAttributes(attrs)})
}

func upsertVegapunkGrowthGraph(ctx context.Context, graph *vegapunkGraph) error {
	if graph == nil || len(graph.Nodes) == 0 {
		return nil
	}
	graph.dedupe()
	ctx, cancel := context.WithTimeout(ctx, vegapunkGrowthSyncTimeout)
	defer cancel()

	for _, node := range graph.Nodes {
		text := vegapunkNodeText(node)
		if text == "" {
			continue
		}
		vector, err := embedVegapunkText(ctx, text)
		if err != nil {
			slog.Warn("vegapunk embed failed; upserting graph without this vector", slog.String("node_id", node.ID), slog.Any("error", err))
			continue
		}
		graph.Vectors = append(graph.Vectors, vegapunkVector{
			ID:     node.ID,
			Vector: vector,
			Metadata: map[string]string{
				"node_id":      node.ID,
				"text":         text,
				"source_type":  "cfn-tracker-growth",
				"timestamp_ms": strconv.FormatInt(time.Now().UnixMilli(), 10),
				"schema":       vegapunkSchema(),
				"type":         node.Type,
			},
		})
	}
	if len(graph.Nodes) > 0 && len(graph.Vectors) == 0 {
		return fmt.Errorf("created 0 vectors for %d nodes", len(graph.Nodes))
	}

	body, err := json.Marshal(graph)
	if err != nil {
		return fmt.Errorf("marshal upsert graph request: %w", err)
	}
	out, err := runVegapunkGRPC(ctx, "graphrag.GraphRAGEngine/UpsertGraph", body)
	if err != nil {
		return err
	}
	var res struct {
		UpsertedNodes   int `json:"upsertedNodes"`
		UpsertedEdges   int `json:"upsertedEdges"`
		UpsertedVectors int `json:"upsertedVectors"`
	}
	if err := json.Unmarshal(out, &res); err == nil {
		slog.Info(
			"vegapunk growth graph upserted",
			slog.Int("nodes", res.UpsertedNodes),
			slog.Int("edges", res.UpsertedEdges),
			slog.Int("vectors", res.UpsertedVectors),
		)
		if len(graph.Vectors) > 0 && res.UpsertedVectors == 0 {
			return fmt.Errorf("upserted 0 vectors for %d vector entries", len(graph.Vectors))
		}
	}
	return nil
}

func (g *vegapunkGraph) dedupe() {
	nodesByID := map[string]vegapunkNode{}
	nodeOrder := make([]string, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		if node.ID == "" {
			continue
		}
		if _, ok := nodesByID[node.ID]; !ok {
			nodeOrder = append(nodeOrder, node.ID)
		}
		nodesByID[node.ID] = node
	}
	g.Nodes = make([]vegapunkNode, 0, len(nodeOrder))
	for _, id := range nodeOrder {
		g.Nodes = append(g.Nodes, nodesByID[id])
	}

	edgesByID := map[string]vegapunkEdge{}
	edgeOrder := make([]string, 0, len(g.Edges))
	for _, edge := range g.Edges {
		if edge.FromID == "" || edge.ToID == "" || edge.Type == "" {
			continue
		}
		key := edge.FromID + "\x00" + edge.Type + "\x00" + edge.ToID
		if _, ok := edgesByID[key]; !ok {
			edgeOrder = append(edgeOrder, key)
		}
		edgesByID[key] = edge
	}
	g.Edges = make([]vegapunkEdge, 0, len(edgeOrder))
	for _, key := range edgeOrder {
		g.Edges = append(g.Edges, edgesByID[key])
	}

	vectorsByID := map[string]vegapunkVector{}
	vectorOrder := make([]string, 0, len(g.Vectors))
	for _, vector := range g.Vectors {
		if vector.ID == "" {
			continue
		}
		if _, ok := vectorsByID[vector.ID]; !ok {
			vectorOrder = append(vectorOrder, vector.ID)
		}
		vectorsByID[vector.ID] = vector
	}
	g.Vectors = make([]vegapunkVector, 0, len(vectorOrder))
	for _, id := range vectorOrder {
		g.Vectors = append(g.Vectors, vectorsByID[id])
	}
}

func embedVegapunkText(ctx context.Context, text string) ([]float64, error) {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, err
	}
	out, err := runVegapunkGRPC(ctx, "graphrag.GraphRAGEngine/Embed", body)
	if err != nil {
		return nil, err
	}
	var res struct {
		Vector []float64 `json:"vector"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("parse embed response: %w", err)
	}
	if len(res.Vector) == 0 {
		return nil, fmt.Errorf("empty embed vector")
	}
	return res.Vector, nil
}

func runVegapunkGRPC(ctx context.Context, method string, body []byte) ([]byte, error) {
	target := strings.TrimSpace(os.Getenv(vegapunkGRPCTargetEnvKey))
	if target == "" {
		target = "vegapunk.local:6840"
	}
	protoPath := strings.TrimSpace(os.Getenv(vegapunkProtoEnvKey))
	if protoPath == "" {
		protoPath = "/Users/ryugo/Developer/src/AI-Project/vegapunk/proto/graphrag.proto"
	}
	protoImportPath := strings.TrimSpace(os.Getenv(vegapunkProtoImportEnvKey))
	if protoImportPath == "" {
		protoImportPath = filepath.Dir(protoPath)
	}
	args := []string{
		"-plaintext",
		"-import-path", protoImportPath,
		"-proto", filepath.Base(protoPath),
		"-d", "@",
	}
	if token := strings.TrimSpace(os.Getenv(vegapunkTokenEnvKey)); token != "" {
		args = append(args, "-H", "authorization: Bearer "+token)
	}
	args = append(args, target, method)
	cmd := exec.CommandContext(ctx, "grpcurl", args...)
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = err.Error()
		}
		return nil, fmt.Errorf("%s: %s", method, text)
	}
	return out, nil
}

func vegapunkConfigured() bool {
	token := strings.TrimSpace(os.Getenv(vegapunkTokenEnvKey))
	return token != "" && !strings.HasPrefix(token, "op://")
}

func vegapunkSchema() string {
	schema := strings.TrimSpace(os.Getenv(vegapunkSchemaEnvKey))
	if schema == "" {
		return "sf6-advice"
	}
	return schema
}

func vegapunkPlayerID(userId string) string {
	return vegapunkID("player", userId)
}

func vegapunkID(parts ...string) string {
	cleaned := make([]string, 0, len(parts)+1)
	cleaned = append(cleaned, vegapunkSchema())
	cleaned = append(cleaned, "gen1")
	for _, part := range parts {
		cleaned = append(cleaned, sanitizeVegapunkIDPart(part))
	}
	return strings.Join(cleaned, ":")
}

var vegapunkIDPartPattern = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func sanitizeVegapunkIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = vegapunkIDPartPattern.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "unknown"
	}
	return strings.ToLower(value)
}

func vegapunkAttributes(attrs map[string]string) []vegapunkAttribute {
	out := make([]vegapunkAttribute, 0, len(attrs))
	for key, value := range attrs {
		out = append(out, vegapunkAttribute{Key: key, Value: value})
	}
	return out
}

func vegapunkNodeText(node vegapunkNode) string {
	for _, attr := range node.Attributes {
		if attr.Key == "text" {
			return strings.TrimSpace(attr.Value)
		}
	}
	return strings.TrimSpace(node.ID)
}

func growthMetrics(snap model.PlayStatsSnapshot) []growthMetric {
	return []growthMetric{
		{Key: "received_drive_impact", Label: "DI被弾", HigherGood: false, Value: snap.ReceivedDriveImpact},
		{Key: "just_parry", Label: "ジャストパリィ", HigherGood: true, Value: snap.JustParry},
		{Key: "throw_tech", Label: "投げ抜け", HigherGood: true, Value: snap.ThrowTech},
		{Key: "cornered_time", Label: "壁際に追い詰められた時間", HigherGood: false, Value: snap.CorneredTime},
		{Key: "received_punish_counter", Label: "パニカン被弾", HigherGood: false, Value: snap.ReceivedPunishCounter},
		{Key: "throw_count", Label: "投げ", HigherGood: true, Value: snap.ThrowCount},
	}
}

func growthObservations(snap model.PlayStatsSnapshot, previous *model.PlayStatsSnapshot) []growthObservation {
	if previous == nil {
		return []growthObservation{{
			Key:         "baseline",
			Label:       "初回観測",
			Direction:   "baseline",
			Assessment:  "baseline",
			HasPrevious: false,
		}}
	}
	metrics := growthMetrics(snap)
	out := make([]growthObservation, 0, len(metrics))
	for _, metric := range metrics {
		prevValue := avgValue(previous, metric.Key)
		delta := metric.Value - prevValue
		direction := "unchanged"
		if delta > 0 {
			direction = "increased"
		} else if delta < 0 {
			direction = "decreased"
		}
		assessment := "neutral"
		if delta != 0 {
			improved := (metric.HigherGood && delta > 0) || (!metric.HigherGood && delta < 0)
			if improved {
				assessment = "improved"
			} else {
				assessment = "worsened"
			}
		}
		out = append(out, growthObservation{
			Key:         metric.Key,
			Label:       metric.Label,
			Direction:   direction,
			Assessment:  assessment,
			HigherGood:  metric.HigherGood,
			HasPrevious: true,
		})
	}
	return out
}

func playStatsObservationSummaryText(
	snap model.PlayStatsSnapshot,
	previous *model.PlayStatsSnapshot,
	observations []growthObservation,
) string {
	lines := []string{
		fmt.Sprintf("Observation summary for user %s character %s at %s JST.", snap.UserId, snap.Character, snap.SnapshotAt),
		"Exact metric values, counts, and deltas are intentionally not stored in PunkRecord; the local RDB is the source of truth for numeric analysis.",
	}
	if previous == nil {
		lines = append(lines, "This snapshot establishes a baseline observation. Use the RDB/API for exact current values.")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("Compared with the previous RDB snapshot at %s, qualitative movement was observed:", previous.SnapshotAt))
	for _, obs := range observations {
		if !obs.HasPrevious {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s, assessed as %s for this metric.", obs.Label, obs.Direction, obs.Assessment))
	}
	lines = append(lines, "Treat these as retrieval hints only, not as causal proof or numeric fact storage.")
	return strings.Join(lines, "\n")
}

func vegapunkSchemaNodeType(nodeType string) string {
	switch nodeType {
	case "Metric", "Evidence", "Symptom", "CauseHypothesis", "AdviceAction", "Drill", "SuccessCriterion", "SideEffect", "AdviceOutcome":
		return nodeType
	case "MetricObservation", "MetricDelta", "ObservationSummary", "PlayStatsSnapshot", "Match":
		return "Evidence"
	case "AdviceCandidate":
		return "AdviceAction"
	default:
		return "Evidence"
	}
}

func vegapunkSchemaAttributes(nodeType, id string, attrs map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range attrs {
		out[key] = value
	}
	text := strings.TrimSpace(out["text"])
	if text == "" {
		text = id
		out["text"] = text
	}
	title := strings.TrimSpace(out["title"])
	if title == "" {
		title = strings.TrimSpace(out["label"])
	}
	if title == "" {
		title = firstLine(text)
	}
	if title == "" {
		title = id
	}
	switch nodeType {
	case "Metric":
		if strings.TrimSpace(out["key"]) == "" {
			out["key"] = firstNonEmpty(out["metric_key"], sanitizeVegapunkIDPart(title))
		}
		if strings.TrimSpace(out["label"]) == "" {
			out["label"] = title
		}
	case "Evidence":
		if strings.TrimSpace(out["source"]) == "" {
			out["source"] = "cfn-tracker"
		}
		if strings.TrimSpace(out["title"]) == "" {
			out["title"] = title
		}
	case "AdviceAction":
		if strings.TrimSpace(out["title"]) == "" {
			out["title"] = title
		}
		if strings.TrimSpace(out["instruction"]) == "" {
			out["instruction"] = firstNonEmpty(out["action"], out["summary"], text)
		}
	case "Symptom", "CauseHypothesis", "SideEffect":
		if strings.TrimSpace(out["title"]) == "" {
			out["title"] = title
		}
		if strings.TrimSpace(out["description"]) == "" {
			out["description"] = firstNonEmpty(out["summary"], text)
		}
	case "Drill":
		if strings.TrimSpace(out["title"]) == "" {
			out["title"] = title
		}
		if strings.TrimSpace(out["procedure"]) == "" {
			out["procedure"] = firstNonEmpty(out["summary"], text)
		}
	case "SuccessCriterion":
		if strings.TrimSpace(out["title"]) == "" {
			out["title"] = title
		}
	case "AdviceOutcome":
		if strings.TrimSpace(out["advice_run_id"]) == "" {
			out["advice_run_id"] = id
		}
		if strings.TrimSpace(out["mode"]) == "" {
			out["mode"] = "unknown"
		}
		if strings.TrimSpace(out["summary"]) == "" {
			out["summary"] = firstNonEmpty(out["title"], firstLine(text))
		}
		if strings.TrimSpace(out["created_at"]) == "" {
			out["created_at"] = time.Now().Format(time.RFC3339)
		}
	}
	return out
}

func vegapunkSchemaEdgeType(edgeType string) string {
	switch edgeType {
	case "EXPECTED_TO_IMPROVE", "HAS_SUCCESS_CRITERION", "IMPROVED_BY", "INDICATES", "MAY_BE_CAUSED_BY", "MAY_WORSEN", "MEASURED_BY", "PRACTICED_BY", "SUPPORTS", "WATCHED_BY":
		return edgeType
	default:
		return "RELATES_TO"
	}
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
