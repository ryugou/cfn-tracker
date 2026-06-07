package sql

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"io/fs"
	"log"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

var store *Storage

// newTestStorage builds an in-memory SQLite storage with all embedded
// migrations applied. NewStorage(true) on its own currently only migrates the
// on-disk DB at getDataSource(), leaving the in-memory connection schemaless,
// so tests bootstrap the schema directly here.
func newTestStorage() (*Storage, error) {
	db, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(migrationsFs, "migrations")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		buf, err := fs.ReadFile(migrationsFs, "migrations/"+name)
		if err != nil {
			return nil, err
		}
		if _, err := db.Exec(string(buf)); err != nil {
			return nil, err
		}
	}
	return &Storage{db: db}, nil
}

func TestVegapunkSyncQueueLifecycle(t *testing.T) {
	ctx := context.Background()
	s, err := newTestStorage()
	if err != nil {
		t.Fatalf("newTestStorage: %v", err)
	}

	if err := s.EnqueueVegapunkSyncJob(ctx, "match", "match:u:replay-1", []byte(`{"nodes":[{"id":"n1"}]}`)); err != nil {
		t.Fatalf("EnqueueVegapunkSyncJob: %v", err)
	}
	jobs, err := s.GetDueVegapunkSyncJobs(ctx, 10)
	if err != nil {
		t.Fatalf("GetDueVegapunkSyncJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	if jobs[0].Attempts != 0 || jobs[0].ProcessedAt != "" {
		t.Fatalf("unexpected initial job state: %+v", jobs[0])
	}

	next := time.Now().Add(time.Hour)
	if err := s.MarkVegapunkSyncJobFailed(ctx, jobs[0].Id, 1, "temporary failure", next); err != nil {
		t.Fatalf("MarkVegapunkSyncJobFailed: %v", err)
	}
	jobs, err = s.GetDueVegapunkSyncJobs(ctx, 10)
	if err != nil {
		t.Fatalf("GetDueVegapunkSyncJobs after failure: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs after future retry = %d, want 0", len(jobs))
	}

	if err := s.EnqueueVegapunkSyncJob(ctx, "match", "match:u:replay-1", []byte(`{"nodes":[{"id":"n2"}]}`)); err != nil {
		t.Fatalf("re-enqueue: %v", err)
	}
	jobs, err = s.GetDueVegapunkSyncJobs(ctx, 10)
	if err != nil {
		t.Fatalf("GetDueVegapunkSyncJobs after re-enqueue: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("jobs after re-enqueue = %d, want 1", len(jobs))
	}
	if jobs[0].Attempts != 0 || jobs[0].LastError != "" || !strings.Contains(jobs[0].PayloadJSON, "n2") {
		t.Fatalf("re-enqueue did not reset job: %+v", jobs[0])
	}

	if err := s.MarkVegapunkSyncJobDone(ctx, jobs[0].Id); err != nil {
		t.Fatalf("MarkVegapunkSyncJobDone: %v", err)
	}
	jobs, err = s.GetDueVegapunkSyncJobs(ctx, 10)
	if err != nil {
		t.Fatalf("GetDueVegapunkSyncJobs after done: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs after done = %d, want 0", len(jobs))
	}
}

func TestMain(m *testing.M) {
	s, err := newTestStorage()
	if err != nil {
		log.Fatalf("init sql storage: %v", err)
	}
	store = s
	os.Exit(m.Run())
}

func sampleSnapshot(userId, character, replayId string) model.PlayStatsSnapshot {
	snap := model.PlayStatsSnapshot{
		UserId:                      userId,
		Character:                   character,
		DriveImpact:                 1.2,
		ReceivedDriveImpact:         1.9,
		JustParry:                   0,
		ThrowTech:                   0.1,
		CornerTime:                  3,
		CorneredTime:                10,
		GaugeRateSALv3:              0.2786,
		RankMatchPlayCount:          59,
		WorldTourSeconds:            59936,
		RankedMatchSeconds:          4692,
		PracticeSeconds:             159402,
		RivalAIHighestLeagueRankTxt: "Rookie 2",
	}
	if replayId != "" {
		snap.MatchReplayId = stdsql.NullString{String: replayId, Valid: true}
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

func TestGetPlayStatsHistoryFiltersAndLimit(t *testing.T) {
	ctx := context.Background()

	// Mix character tags on the saved snapshots — GetPlayStatsHistory must
	// ignore the `character` argument and treat snapshots as user-wide.
	for _, char := range []string{"JP", "Ken", "JP"} {
		if err := store.SavePlayStats(ctx, sampleSnapshot("user-filter", char, "")); err != nil {
			t.Fatalf("SavePlayStats: %v", err)
		}
	}

	// 0) character argument is ignored: passing a value that no snapshot
	//    was saved with must still return all of the user's snapshots.
	all, err := store.GetPlayStatsHistory(ctx, "user-filter", "Ryu", "", "", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(character ignored): %v", err)
	}
	if len(all) != 3 {
		t.Errorf("character-ignored rows = %d, want 3", len(all))
	}

	// 1) limit > 0 caps the result set
	limited, err := store.GetPlayStatsHistory(ctx, "user-filter", "", "", "", 2)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(limit=2): %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("limited rows = %d, want 2", len(limited))
	}

	// 2) "from" in the future excludes everything
	none, err := store.GetPlayStatsHistory(ctx, "user-filter", "", "2099-01-01", "", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(from=future): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("future-from rows = %d, want 0", len(none))
	}

	// 3) "to" in the far past excludes everything
	noneOld, err := store.GetPlayStatsHistory(ctx, "user-filter", "", "", "1970-01-01", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(to=past): %v", err)
	}
	if len(noneOld) != 0 {
		t.Errorf("past-to rows = %d, want 0", len(noneOld))
	}

	// 4) Filtering on a different user yields nothing
	other, err := store.GetPlayStatsHistory(ctx, "user-other", "", "", "", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(other user): %v", err)
	}
	if len(other) != 0 {
		t.Errorf("other-user rows = %d, want 0", len(other))
	}
}

func TestGetMatchCharactersForUser(t *testing.T) {
	ctx := context.Background()
	if err := store.SaveUser(ctx, model.User{Code: "char-list-u", DisplayName: "clu"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "char-list-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// Two JP matches + one Ken match for char-list-u; plus a Ryu match for
	// a different user that must not leak into the result.
	for _, m := range []model.Match{
		{UserId: "char-list-u", UserName: "clu", SessionId: sesh.Id, ReplayID: "cl-jp-1", Character: "JP", Date: "2026-05-23", Time: "10:00"},
		{UserId: "char-list-u", UserName: "clu", SessionId: sesh.Id, ReplayID: "cl-jp-2", Character: "JP", Date: "2026-05-23", Time: "10:01"},
		{UserId: "char-list-u", UserName: "clu", SessionId: sesh.Id, ReplayID: "cl-ken-1", Character: "Ken", Date: "2026-05-23", Time: "10:02"},
	} {
		if err := store.SaveMatch(ctx, m); err != nil {
			t.Fatalf("SaveMatch: %v", err)
		}
	}
	if err := store.SaveUser(ctx, model.User{Code: "char-list-other", DisplayName: "other"}); err != nil {
		t.Fatalf("SaveUser other: %v", err)
	}
	otherSesh, err := store.CreateSession(ctx, "char-list-other")
	if err != nil {
		t.Fatalf("CreateSession other: %v", err)
	}
	if err := store.SaveMatch(ctx, model.Match{
		UserId: "char-list-other", UserName: "other", SessionId: otherSesh.Id,
		ReplayID: "cl-ryu-1", Character: "Ryu",
		Date: "2026-05-23", Time: "10:00",
	}); err != nil {
		t.Fatalf("SaveMatch other: %v", err)
	}

	got, err := store.GetMatchCharactersForUser(ctx, "char-list-u")
	if err != nil {
		t.Fatalf("GetMatchCharactersForUser: %v", err)
	}
	// Expect DISTINCT + alphabetical: ["JP", "Ken"]; "Ryu" belongs to the other user.
	if len(got) != 2 || got[0] != "JP" || got[1] != "Ken" {
		t.Errorf("characters = %v, want [JP Ken]", got)
	}
}

func TestSaveMatchIfNewReportsPrimaryKeyCollision(t *testing.T) {
	ctx := context.Background()

	if err := store.SaveUser(ctx, model.User{Code: "collide-u", DisplayName: "bf"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "collide-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	base := model.Match{
		UserId: "collide-u", UserName: "bf", SessionId: sesh.Id,
		Character: "JP", Date: "2026-05-23", Time: "10:00",
	}

	first := base
	first.ReplayID = "collide-first"
	inserted, err := store.SaveMatchIfNew(ctx, first)
	if err != nil {
		t.Fatalf("SaveMatchIfNew first: %v", err)
	}
	if !inserted {
		t.Fatalf("first insert should report inserted=true")
	}

	dup := base
	dup.ReplayID = "collide-second"
	inserted, err = store.SaveMatchIfNew(ctx, dup)
	if err != nil {
		t.Fatalf("SaveMatchIfNew dup: %v", err)
	}
	if inserted {
		t.Errorf("PK collision should report inserted=false, replay_id=%q", dup.ReplayID)
	}
}

func TestGetMatchReplayIDsForUser(t *testing.T) {
	ctx := context.Background()

	if err := store.SaveUser(ctx, model.User{Code: "backfill-u", DisplayName: "bf"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "backfill-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	for _, rid := range []string{"r1", "r2", "r3"} {
		err := store.SaveMatch(ctx, model.Match{
			UserId: "backfill-u", UserName: "bf", SessionId: sesh.Id,
			ReplayID: rid, Character: "JP",
			Date: "2026-05-23", Time: "10:0" + rid[1:],
		})
		if err != nil {
			t.Fatalf("SaveMatch(%s): %v", rid, err)
		}
	}

	got, err := store.GetMatchReplayIDsForUser(ctx, "backfill-u")
	if err != nil {
		t.Fatalf("GetMatchReplayIDsForUser: %v", err)
	}
	for _, want := range []string{"r1", "r2", "r3"} {
		if !got[want] {
			t.Errorf("missing replay id %q in result %v", want, got)
		}
	}
	if got["r4"] {
		t.Errorf("unexpected r4 in result")
	}
}

func TestGetLatestMatchForUser(t *testing.T) {
	ctx := context.Background()

	if err := store.SaveUser(ctx, model.User{Code: "latest-u", DisplayName: "lu"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "latest-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	older := model.Match{
		UserId: "latest-u", UserName: "lu", SessionId: sesh.Id,
		ReplayID: "old-r", Character: "JP",
		Date: "2026-05-20", Time: "10:00",
		Wins: 5, Losses: 2,
	}
	newer := model.Match{
		UserId: "latest-u", UserName: "lu", SessionId: sesh.Id,
		ReplayID: "new-r", Character: "JP",
		Date: "2026-05-23", Time: "14:30",
		Wins: 6, Losses: 2,
	}
	if err := store.SaveMatch(ctx, older); err != nil {
		t.Fatalf("SaveMatch older: %v", err)
	}
	if err := store.SaveMatch(ctx, newer); err != nil {
		t.Fatalf("SaveMatch newer: %v", err)
	}

	got, err := store.GetLatestMatchForUser(ctx, "latest-u")
	if err != nil {
		t.Fatalf("GetLatestMatchForUser: %v", err)
	}
	if got == nil {
		t.Fatal("expected a match, got nil")
	}
	if got.ReplayID != "new-r" {
		t.Errorf("ReplayID = %q, want new-r", got.ReplayID)
	}
	if got.Wins != 6 {
		t.Errorf("Wins = %d, want 6", got.Wins)
	}

	none, err := store.GetLatestMatchForUser(ctx, "no-such-user")
	if err != nil {
		t.Fatalf("GetLatestMatchForUser(no user): %v", err)
	}
	if none != nil {
		t.Errorf("expected nil for unknown user, got %+v", none)
	}
}

func TestSaveAndGetBenchmarkPlayers(t *testing.T) {
	ctx := context.Background()
	stats := sampleSnapshot("bench-target", "JP", "")
	players := []*model.BenchmarkPlayer{
		{
			SourceUserId:      "bench-source",
			TargetUserId:      "bench-target",
			FighterId:         "Benchmark",
			Character:         "JP",
			CharacterToolName: "jp",
			RankOffset:        1,
			LeagueRank:        36,
			LP:                30000,
			MR:                1600,
			MRRanking:         1000,
			Wins:              11,
			Losses:            4,
			WinDiff:           7,
			LastPlayAt:        1779859033,
			Stats:             &stats,
		},
	}

	if err := store.SaveBenchmarkPlayers(ctx, "bench-source", "JP", players); err != nil {
		t.Fatalf("SaveBenchmarkPlayers: %v", err)
	}
	got, err := store.GetBenchmarkPlayers(ctx, "bench-source", "JP")
	if err != nil {
		t.Fatalf("GetBenchmarkPlayers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("benchmark rows = %d, want 1", len(got))
	}
	if got[0].TargetUserId != "bench-target" {
		t.Errorf("TargetUserId = %q, want bench-target", got[0].TargetUserId)
	}
	if got[0].Stats == nil {
		t.Fatal("Stats should be unmarshaled")
	}
	if got[0].Stats.DriveImpact != stats.DriveImpact {
		t.Errorf("DriveImpact = %v, want %v", got[0].Stats.DriveImpact, stats.DriveImpact)
	}
	if got[0].FetchedAt == "" {
		t.Errorf("FetchedAt should be populated")
	}
}

func TestSaveAndListAdviceRuns(t *testing.T) {
	ctx := context.Background()
	first := &model.AdviceRun{
		UserId:      "advice-u",
		Character:   "JP",
		InputWindow: 30,
		SnapshotAt:  "2026-05-29 10:00:00",
		Candidates: []*model.AdviceCandidate{
			{
				Mode:            model.AdviceModeDBOnly,
				Priority:        "高",
				Theme:           "DI被弾を減らす",
				Summary:         "summary",
				Rationale:       "rationale",
				Action:          "action",
				Drill:           "drill",
				SuccessCriteria: "success",
				WatchMetrics:    "DI被弾",
				Evidence: []model.AdviceEvidence{
					{Source: "db", Title: "DI被弾", Text: "evidence"},
				},
			},
		},
	}
	second := &model.AdviceRun{
		UserId:      "advice-u",
		Character:   "JP",
		InputWindow: 30,
		SnapshotAt:  "2026-05-29 11:00:00",
		Candidates: []*model.AdviceCandidate{
			{Mode: model.AdviceModeGraphRAG, Theme: "投げ択を増やす", Action: "action"},
		},
	}

	if err := store.SaveAdviceRun(ctx, first); err != nil {
		t.Fatalf("SaveAdviceRun(first): %v", err)
	}
	if first.CreatedAt == "" {
		t.Fatal("first CreatedAt should be populated after save")
	}
	if first.Candidates[0].CreatedAt == "" {
		t.Fatal("first candidate CreatedAt should be populated after save")
	}
	if err := store.SaveAdviceRun(ctx, second); err != nil {
		t.Fatalf("SaveAdviceRun(second): %v", err)
	}
	if second.CreatedAt == "" {
		t.Fatal("second CreatedAt should be populated after save")
	}
	runs, err := store.GetAdviceRuns(ctx, "advice-u", "JP", 20)
	if err != nil {
		t.Fatalf("GetAdviceRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("advice runs = %d, want 2", len(runs))
	}
	if runs[0].Id != second.Id {
		t.Fatalf("latest run id = %d, want %d", runs[0].Id, second.Id)
	}
	if len(runs[1].Candidates) != 1 {
		t.Fatalf("first candidates = %d, want 1", len(runs[1].Candidates))
	}
	if runs[1].Candidates[0].Evidence[0].Text != "evidence" {
		t.Fatalf("evidence did not roundtrip: %+v", runs[1].Candidates[0].Evidence)
	}

	if err := store.SaveAdviceFeedback(ctx, model.AdviceFeedback{
		RunId:       second.Id,
		Mode:        model.AdviceModeGraphRAG,
		Rating:      4,
		Specificity: 4,
		Usefulness:  4,
		Trust:       4,
	}); err != nil {
		t.Fatalf("SaveAdviceFeedback: %v", err)
	}
	if err := store.DeleteAdviceRun(ctx, second.Id); err != nil {
		t.Fatalf("DeleteAdviceRun: %v", err)
	}
	runs, err = store.GetAdviceRuns(ctx, "advice-u", "JP", 20)
	if err != nil {
		t.Fatalf("GetAdviceRuns after delete: %v", err)
	}
	if len(runs) != 1 || runs[0].Id != first.Id {
		t.Fatalf("runs after delete = %+v, want only first run", runs)
	}
}

func TestGetBenchmarkPlayersReturnsTopFivePerRankOffset(t *testing.T) {
	ctx := context.Background()
	players := make([]*model.BenchmarkPlayer, 0, 14)
	for offset := 1; offset <= 2; offset++ {
		for i := 0; i < 7; i++ {
			stats := sampleSnapshot("bench-top-target", "JP", "")
			wins := 20 + i
			losses := 10
			if i == 6 {
				wins = 9
				losses = 10
			}
			players = append(players, &model.BenchmarkPlayer{
				SourceUserId:      "bench-top-source",
				TargetUserId:      fmt.Sprintf("bench-top-%d-%d", offset, i),
				FighterId:         fmt.Sprintf("Benchmark%d%d", offset, i),
				Character:         "JP",
				CharacterToolName: "jp",
				RankOffset:        offset,
				LeagueRank:        20 + offset,
				LP:                30000 + i,
				Wins:              wins,
				Losses:            losses,
				WinDiff:           wins - losses,
				LastPlayAt:        1779859033,
				Stats:             &stats,
			})
		}
	}

	if err := store.SaveBenchmarkPlayers(ctx, "bench-top-source", "JP", players); err != nil {
		t.Fatalf("SaveBenchmarkPlayers: %v", err)
	}
	got, err := store.GetBenchmarkPlayers(ctx, "bench-top-source", "JP")
	if err != nil {
		t.Fatalf("GetBenchmarkPlayers: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("benchmark rows = %d, want 10", len(got))
	}
	counts := map[int]int{}
	for _, row := range got {
		counts[row.RankOffset]++
		if row.Wins <= row.Losses {
			t.Fatalf("returned losing row: %+v", row)
		}
	}
	if counts[1] != 5 || counts[2] != 5 {
		t.Fatalf("rank offset counts = %v, want map[1:5 2:5]", counts)
	}
	if got[0].WinDiff < got[1].WinDiff || got[5].WinDiff < got[6].WinDiff {
		t.Fatalf("rows are not sorted by win diff within rank: %v", got)
	}
}

func TestGetLatestMatchForUserAndCharacter(t *testing.T) {
	ctx := context.Background()

	if err := store.SaveUser(ctx, model.User{Code: "latest-char-u", DisplayName: "lcu"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "latest-char-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// JP has an older and newer match; Ken has a single newest-of-all match.
	// Querying for JP must return the newer JP match (not the absolute latest).
	matches := []model.Match{
		{
			UserId: "latest-char-u", UserName: "lcu", SessionId: sesh.Id,
			ReplayID: "lc-jp-old", Character: "JP",
			Date: "2026-05-20", Time: "10:00",
			Wins: 3, Losses: 1,
		},
		{
			UserId: "latest-char-u", UserName: "lcu", SessionId: sesh.Id,
			ReplayID: "lc-jp-new", Character: "JP",
			Date: "2026-05-22", Time: "12:00",
			Wins: 4, Losses: 1,
		},
		{
			UserId: "latest-char-u", UserName: "lcu", SessionId: sesh.Id,
			ReplayID: "lc-ken-latest", Character: "Ken",
			Date: "2026-05-23", Time: "09:00",
			Wins: 1, Losses: 0,
		},
	}
	for _, m := range matches {
		if err := store.SaveMatch(ctx, m); err != nil {
			t.Fatalf("SaveMatch %s: %v", m.ReplayID, err)
		}
	}

	got, err := store.GetLatestMatchForUserAndCharacter(ctx, "latest-char-u", "JP")
	if err != nil {
		t.Fatalf("GetLatestMatchForUserAndCharacter: %v", err)
	}
	if got == nil {
		t.Fatal("expected JP match, got nil")
	}
	if got.ReplayID != "lc-jp-new" {
		t.Errorf("ReplayID = %q, want lc-jp-new (newer JP, even though Ken is overall newer)", got.ReplayID)
	}
	if got.Wins != 4 {
		t.Errorf("Wins = %d, want 4", got.Wins)
	}

	none, err := store.GetLatestMatchForUserAndCharacter(ctx, "latest-char-u", "Ryu")
	if err != nil {
		t.Fatalf("GetLatestMatchForUserAndCharacter(unknown char): %v", err)
	}
	if none != nil {
		t.Errorf("expected nil for unplayed character, got %+v", none)
	}
}

func TestGetLatestPlayStatsSnapshot(t *testing.T) {
	ctx := context.Background()

	// No snapshots
	got, err := store.GetLatestPlayStatsSnapshot(ctx, "no-such-user")
	if err != nil {
		t.Fatalf("GetLatestPlayStatsSnapshot (empty): %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown user, got %+v", got)
	}

	// snapshot_at is server-set on insert (CURRENT_TIMESTAMP default in the
	// schema), so to assert that ORDER BY snapshot_at DESC wins we override
	// the timestamps with raw UPDATEs after insert. rB gets the newer
	// snapshot_at; rA gets the older one.
	if err := store.SavePlayStats(ctx, sampleSnapshot("latest-u", "JP", "rA")); err != nil {
		t.Fatalf("SavePlayStats rA: %v", err)
	}
	if err := store.SavePlayStats(ctx, sampleSnapshot("latest-u", "JP", "rB")); err != nil {
		t.Fatalf("SavePlayStats rB: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE play_stats_snapshots SET snapshot_at = ? WHERE user_id = ? AND match_replay_id = ?`,
		"2026-01-01 00:00:00", "latest-u", "rA",
	); err != nil {
		t.Fatalf("update rA snapshot_at: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE play_stats_snapshots SET snapshot_at = ? WHERE user_id = ? AND match_replay_id = ?`,
		"2026-06-01 12:00:00", "latest-u", "rB",
	); err != nil {
		t.Fatalf("update rB snapshot_at: %v", err)
	}

	got, err = store.GetLatestPlayStatsSnapshot(ctx, "latest-u")
	if err != nil {
		t.Fatalf("GetLatestPlayStatsSnapshot: %v", err)
	}
	if got == nil {
		t.Fatal("expected a snapshot, got nil")
	}
	if got.MatchReplayId.String != "rB" {
		t.Errorf("MatchReplayId = %q, want rB (newest by snapshot_at)", got.MatchReplayId.String)
	}

	// user-wide semantics: even after a snapshot tagged with a different
	// character is inserted, the user-level lookup must still return the
	// most recent snapshot regardless of character.
	if err := store.SavePlayStats(ctx, sampleSnapshot("latest-u", "Ken", "rC")); err != nil {
		t.Fatalf("SavePlayStats rC: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE play_stats_snapshots SET snapshot_at = ? WHERE user_id = ? AND match_replay_id = ?`,
		"2026-12-01 00:00:00", "latest-u", "rC",
	); err != nil {
		t.Fatalf("update rC snapshot_at: %v", err)
	}
	got, err = store.GetLatestPlayStatsSnapshot(ctx, "latest-u")
	if err != nil {
		t.Fatalf("GetLatestPlayStatsSnapshot (cross-char): %v", err)
	}
	if got == nil || got.MatchReplayId.String != "rC" {
		t.Errorf("cross-character latest = %+v, want rC", got)
	}

	// Tiebreak when snapshot_at is identical: id DESC wins.
	if err := store.SavePlayStats(ctx, sampleSnapshot("tie-u", "JP", "tie-A")); err != nil {
		t.Fatalf("SavePlayStats tie-A: %v", err)
	}
	if err := store.SavePlayStats(ctx, sampleSnapshot("tie-u", "JP", "tie-B")); err != nil {
		t.Fatalf("SavePlayStats tie-B: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE play_stats_snapshots SET snapshot_at = '2026-03-01 09:00:00' WHERE user_id = 'tie-u'`,
	); err != nil {
		t.Fatalf("update tie snapshot_at: %v", err)
	}
	got, err = store.GetLatestPlayStatsSnapshot(ctx, "tie-u")
	if err != nil {
		t.Fatalf("GetLatestPlayStatsSnapshot (tiebreak): %v", err)
	}
	if got == nil || got.MatchReplayId.String != "tie-B" {
		t.Errorf("tiebreak latest = %+v, want tie-B (higher id)", got)
	}
}

func TestGetMatchesWithPlayStatsDuplicateSnapshotPicksLatest(t *testing.T) {
	ctx := context.Background()

	if err := store.SaveUser(ctx, model.User{Code: "dup-u", DisplayName: "du"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "dup-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := store.SaveMatch(ctx, model.Match{
		UserId: "dup-u", UserName: "du", SessionId: sesh.Id,
		ReplayID: "dup-replay", Character: "JP",
		Date: "2026-05-25", Time: "12:00",
	}); err != nil {
		t.Fatalf("SaveMatch: %v", err)
	}

	// Two snapshots for the same (user, match_replay_id) — should not happen
	// in production, but we want the lookup to be deterministic if it does.
	// Insert in order, then key UPDATEs by the auto-increment id (the older
	// row has the lower id) so the timestamp assignment doesn't depend on
	// fragile float-value matching.
	older := sampleSnapshot("dup-u", "JP", "dup-replay")
	older.DriveImpact = 0.111
	if err := store.SavePlayStats(ctx, older); err != nil {
		t.Fatalf("SavePlayStats older: %v", err)
	}
	newer := sampleSnapshot("dup-u", "JP", "dup-replay")
	newer.DriveImpact = 0.999
	if err := store.SavePlayStats(ctx, newer); err != nil {
		t.Fatalf("SavePlayStats newer: %v", err)
	}

	var olderId, newerId int64
	if err := store.db.GetContext(ctx, &olderId,
		`SELECT MIN(id) FROM play_stats_snapshots WHERE user_id = 'dup-u' AND match_replay_id = 'dup-replay'`,
	); err != nil {
		t.Fatalf("select older id: %v", err)
	}
	if err := store.db.GetContext(ctx, &newerId,
		`SELECT MAX(id) FROM play_stats_snapshots WHERE user_id = 'dup-u' AND match_replay_id = 'dup-replay'`,
	); err != nil {
		t.Fatalf("select newer id: %v", err)
	}
	// Guard against a future refactor that accidentally collapses both
	// inserts into a single row — the rest of the test would silently pass
	// on garbage assertions if older and newer pointed at the same row.
	if olderId == newerId {
		t.Fatalf("expected two distinct snapshot rows, both ids = %d", olderId)
	}
	// Pin snapshot_at so "newer" really is newer than "older".
	if _, err := store.db.ExecContext(ctx,
		`UPDATE play_stats_snapshots SET snapshot_at = '2026-05-25 11:00:00' WHERE id = ?`, olderId,
	); err != nil {
		t.Fatalf("update older snapshot_at: %v", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`UPDATE play_stats_snapshots SET snapshot_at = '2026-05-25 13:00:00' WHERE id = ?`, newerId,
	); err != nil {
		t.Fatalf("update newer snapshot_at: %v", err)
	}

	got, err := store.GetMatchesWithPlayStats(ctx, "dup-u", "JP", 0, 0)
	if err != nil {
		t.Fatalf("GetMatchesWithPlayStats: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 match, got %d", len(got))
	}
	if got[0].Stats == nil {
		t.Fatal("expected stats attached, got nil")
	}
	if got := got[0].Stats.DriveImpact; got < 88.79 || got > 88.81 {
		t.Errorf("DriveImpact = %v, want about 88.80 (newest computed delta wins)", got)
	}
}

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
	baseline := sampleSnapshot("user-5", "JP", "")
	baseline.DriveImpact = 1.00
	baseline.ReceivedDriveImpact = 1.00
	baseline.ThrowTech = 0.10
	baseline.CornerTime = 3.00
	if err := store.SavePlayStats(ctx, baseline); err != nil {
		t.Fatalf("SavePlayStats baseline: %v", err)
	}
	replayStats := sampleSnapshot("user-5", "JP", "replay-with-stats")
	replayStats.DriveImpact = 1.03
	replayStats.ReceivedDriveImpact = 1.07
	replayStats.ThrowTech = 0.11
	replayStats.CornerTime = 2.95
	if err := store.SavePlayStats(ctx, replayStats); err != nil {
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
	} else {
		if got := byReplay["replay-with-stats"].Stats.DriveImpact; got < 2.99 || got > 3.01 {
			t.Errorf("DriveImpact delta = %v, want about 3.00", got)
		}
		if got := byReplay["replay-with-stats"].Stats.ReceivedDriveImpact; got < 6.99 || got > 7.01 {
			t.Errorf("ReceivedDriveImpact delta = %v, want about 7.00", got)
		}
	}
	if byReplay["replay-no-stats"].Stats != nil {
		t.Errorf("expected nil stats for replay-no-stats, got %+v", byReplay["replay-no-stats"].Stats)
	}
}

func TestGetMatchesWithPlayStatsFiltersCharacterInSQL(t *testing.T) {
	ctx := context.Background()
	if err := store.SaveUser(ctx, model.User{Code: "char-u", DisplayName: "cu"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "char-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// Save 1 JP match (older) + 2 Ken matches (newer)
	matches := []model.Match{
		{UserId: "char-u", UserName: "cu", SessionId: sesh.Id, ReplayID: "char-jp-1", Character: "JP", Date: "2026-05-23", Time: "10:00"},
		{UserId: "char-u", UserName: "cu", SessionId: sesh.Id, ReplayID: "char-ken-1", Character: "Ken", Date: "2026-05-23", Time: "11:00"},
		{UserId: "char-u", UserName: "cu", SessionId: sesh.Id, ReplayID: "char-ken-2", Character: "Ken", Date: "2026-05-23", Time: "12:00"},
	}
	for _, m := range matches {
		if err := store.SaveMatch(ctx, m); err != nil {
			t.Fatalf("SaveMatch: %v", err)
		}
	}

	// limit=2: with the old Go-side filter this would have returned 0 JP rows
	// (latest 2 are Ken). New SQL-side filter must return the JP row.
	got, err := store.GetMatchesWithPlayStats(ctx, "char-u", "JP", 2, 0)
	if err != nil {
		t.Fatalf("GetMatchesWithPlayStats: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 JP match, got %d", len(got))
	}
	if got[0].Match.Character != "JP" {
		t.Errorf("character = %q, want JP", got[0].Match.Character)
	}
}

func TestGetMatchesWithPlayStatsAllCharacters(t *testing.T) {
	ctx := context.Background()
	if err := store.SaveUser(ctx, model.User{Code: "all-char-u", DisplayName: "acu"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "all-char-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	matches := []model.Match{
		{UserId: "all-char-u", UserName: "acu", SessionId: sesh.Id, ReplayID: "ac-jp-1", Character: "JP", Date: "2026-05-23", Time: "10:00"},
		{UserId: "all-char-u", UserName: "acu", SessionId: sesh.Id, ReplayID: "ac-ken-1", Character: "Ken", Date: "2026-05-23", Time: "11:00"},
	}
	for _, m := range matches {
		if err := store.SaveMatch(ctx, m); err != nil {
			t.Fatalf("SaveMatch: %v", err)
		}
	}

	// Empty character must include both JP and Ken matches.
	got, err := store.GetMatchesWithPlayStats(ctx, "all-char-u", "", 0, 0)
	if err != nil {
		t.Fatalf("GetMatchesWithPlayStats(all chars): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 matches across all chars, got %d", len(got))
	}
	seen := map[string]bool{}
	for _, m := range got {
		seen[m.Match.Character] = true
	}
	if !seen["JP"] || !seen["Ken"] {
		t.Errorf("missing character in result: seen=%v", seen)
	}
}

func TestGetMatchesWithPlayStatsPagination(t *testing.T) {
	ctx := context.Background()
	if err := store.SaveUser(ctx, model.User{Code: "page-u", DisplayName: "pu"}); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}
	sesh, err := store.CreateSession(ctx, "page-u")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	// Two JP matches: newer first by ORDER BY date DESC, time DESC.
	matches := []model.Match{
		{UserId: "page-u", UserName: "pu", SessionId: sesh.Id, ReplayID: "page-jp-1", Character: "JP", Date: "2026-05-22", Time: "10:00"},
		{UserId: "page-u", UserName: "pu", SessionId: sesh.Id, ReplayID: "page-jp-2", Character: "JP", Date: "2026-05-23", Time: "12:00"},
	}
	for _, m := range matches {
		if err := store.SaveMatch(ctx, m); err != nil {
			t.Fatalf("SaveMatch: %v", err)
		}
	}

	// limit=1 offset=0 → newest JP match (page-jp-2)
	first, err := store.GetMatchesWithPlayStats(ctx, "page-u", "JP", 1, 0)
	if err != nil {
		t.Fatalf("GetMatchesWithPlayStats(0): %v", err)
	}
	if len(first) != 1 || first[0].Match.ReplayID != "page-jp-2" {
		t.Fatalf("page 0: want [page-jp-2], got %+v", first)
	}

	// limit=1 offset=1 → second JP match (page-jp-1)
	second, err := store.GetMatchesWithPlayStats(ctx, "page-u", "JP", 1, 1)
	if err != nil {
		t.Fatalf("GetMatchesWithPlayStats(1): %v", err)
	}
	if len(second) != 1 || second[0].Match.ReplayID != "page-jp-1" {
		t.Fatalf("page 1: want [page-jp-1], got %+v", second)
	}
}
