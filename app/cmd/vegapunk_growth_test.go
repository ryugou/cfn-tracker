package cmd

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

func TestVegapunkMatchOmitsNumericFacts(t *testing.T) {
	graph := vegapunkGraph{}
	playerID := vegapunkPlayerID("u-1")
	graph.addMatch(playerID, model.Match{
		UserId:            "u-1",
		UserName:          "ryugo",
		SessionId:         12,
		ReplayID:          "ABC123",
		Character:         "JP",
		Opponent:          "opponent",
		OpponentCharacter: "Ryu",
		OpponentLeague:    "IRON",
		Victory:           true,
		Date:              "2026-06-02",
		Time:              "20:10:00",
		LP:                1800,
		LPGain:            50,
		MR:                1500,
		MRGain:            -10,
		Wins:              11,
		Losses:            9,
		WinRate:           55,
	})

	match := findVegapunkNode(t, graph, "match")
	attrs := vegapunkAttrMap(match)
	for _, forbidden := range []string{"lp", "lp_gain", "mr", "mr_gain", "wins", "losses", "win_rate"} {
		if _, ok := attrs[forbidden]; ok {
			t.Fatalf("match node should not store numeric fact %q: %#v", forbidden, attrs)
		}
	}
	if strings.Contains(attrs["text"], "1800") || strings.Contains(attrs["text"], "55") {
		t.Fatalf("match text leaked numeric facts: %s", attrs["text"])
	}
	if attrs["rdb_ref"] != "ABC123" {
		t.Fatalf("rdb_ref = %q", attrs["rdb_ref"])
	}
}

func TestVegapunkPlayStatsSnapshotUsesNarrativeSummaryOnly(t *testing.T) {
	graph := vegapunkGraph{}
	playerID := vegapunkPlayerID("u-1")
	previous := model.PlayStatsSnapshot{
		Id:                         1,
		UserId:                     "u-1",
		Character:                  "JP",
		SnapshotAt:                 "2026-06-01 20:00:00",
		ReceivedDriveImpact:        2.5,
		JustParry:                  0.1,
		ThrowTech:                  0.2,
		CorneredTime:               10.0,
		ReceivedPunishCounter:      1.0,
		ThrowCount:                 1.0,
		MatchReplayId:              sql.NullString{String: "REPLAY1", Valid: true},
		RankMatchPlayCount:         20,
		TotalAllCharacterPlayPoint: 9999,
	}
	current := model.PlayStatsSnapshot{
		Id:                    2,
		UserId:                "u-1",
		Character:             "JP",
		SnapshotAt:            "2026-06-02 20:00:00",
		ReceivedDriveImpact:   1.5,
		JustParry:             0.2,
		ThrowTech:             0.3,
		CorneredTime:          11.0,
		ReceivedPunishCounter: 0.8,
		ThrowCount:            1.4,
		MatchReplayId:         sql.NullString{String: "REPLAY2", Valid: true},
		RankMatchPlayCount:    21,
	}
	graph.addPlayStatsSnapshot(playerID, current, &previous)

	joinedText := strings.Join(vegapunkGraphText(graph), "\n")
	for _, forbidden := range []string{"1.50", "2.50", "11.00", "10.00", "9999", "ranked matches 21"} {
		if strings.Contains(joinedText, forbidden) {
			t.Fatalf("vegapunk graph leaked numeric fact %q in:\n%s", forbidden, joinedText)
		}
	}

	for _, node := range graph.Nodes {
		if strings.Contains(node.ID, "metric_observation") || strings.Contains(node.ID, "metric_delta") {
			t.Fatalf("unexpected metric fact node: %#v", node)
		}
	}

	summary := findVegapunkNode(t, graph, "observation_summary")
	attrs := vegapunkAttrMap(summary)
	if attrs["rdb_ref"] != "2" {
		t.Fatalf("summary rdb_ref = %q", attrs["rdb_ref"])
	}
	if !strings.Contains(attrs["text"], "Exact metric values, counts, and deltas are intentionally not stored") {
		t.Fatalf("summary should state numeric source-of-truth boundary: %s", attrs["text"])
	}
	if !strings.Contains(attrs["text"], "DI被弾: decreased, assessed as improved") {
		t.Fatalf("summary should keep qualitative trend hints: %s", attrs["text"])
	}
}

func TestVegapunkAdviceRunSkipsDBNumericEvidence(t *testing.T) {
	graph := vegapunkGraph{}
	playerID := vegapunkPlayerID("u-1")
	graph.addAdviceRun(playerID, model.AdviceRun{
		Id:          10,
		UserId:      "u-1",
		Character:   "JP",
		InputWindow: 30,
		SnapshotAt:  "2026-06-02 20:00:00",
		CreatedAt:   "2026-06-02 20:05:00",
		Candidates: []*model.AdviceCandidate{{
			Id:              20,
			Mode:            model.AdviceModePunkRecordOpus46,
			Priority:        "高",
			Theme:           "投げ択を増やす",
			Summary:         "投げが自分1.00、比較対象2.25で不足している。",
			Action:          "密着有利時に投げ択を混ぜる。",
			SuccessCriteria: "直近30件で1.80以上を目指す。",
			WatchMetrics:    "投げ、パニカン被弾",
			Risks:           "前歩きが増えると被弾する可能性。",
			Evidence: []model.AdviceEvidence{
				{Source: "db", Title: "投げ", Text: "自分 1.00 / 比較対象 2.25 / 推移 0.10"},
				{Source: "vegapunk", Title: "投げ択不足", Text: "過去にも投げ択不足が課題として扱われた。"},
			},
		}},
	})

	joinedText := strings.Join(vegapunkGraphText(graph), "\n")
	for _, forbidden := range []string{"自分 1.00", "比較対象 2.25", "1.80以上"} {
		if strings.Contains(joinedText, forbidden) {
			t.Fatalf("advice sync leaked DB numeric fact %q in:\n%s", forbidden, joinedText)
		}
	}

	candidate := findVegapunkNode(t, graph, "advice_candidate")
	attrs := vegapunkAttrMap(candidate)
	for _, forbiddenAttr := range []string{"summary", "success_criteria"} {
		if _, ok := attrs[forbiddenAttr]; ok {
			t.Fatalf("candidate should not store numeric-prone attr %q: %#v", forbiddenAttr, attrs)
		}
	}
	if !strings.Contains(joinedText, "過去にも投げ択不足が課題として扱われた") {
		t.Fatalf("non-DB vegapunk evidence should be retained:\n%s", joinedText)
	}
}

func findVegapunkNode(t *testing.T, graph vegapunkGraph, idPart string) vegapunkNode {
	t.Helper()
	for _, node := range graph.Nodes {
		if strings.Contains(node.ID, idPart) {
			return node
		}
	}
	t.Fatalf("node containing %q not found: %#v", idPart, graph.Nodes)
	return vegapunkNode{}
}

func vegapunkAttrMap(node vegapunkNode) map[string]string {
	out := map[string]string{}
	for _, attr := range node.Attributes {
		out[attr.Key] = attr.Value
	}
	return out
}

func vegapunkGraphText(graph vegapunkGraph) []string {
	out := []string{}
	for _, node := range graph.Nodes {
		attrs := vegapunkAttrMap(node)
		for key, value := range attrs {
			out = append(out, key+"="+value)
		}
	}
	return out
}
