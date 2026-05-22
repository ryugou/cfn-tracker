package sql

import (
	"context"
	stdsql "database/sql"
	"io/fs"
	"log"
	"os"
	"sort"
	"strings"
	"testing"

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

	// Three snapshots for the same user/character — we only need the count
	// to exceed our LIMIT; date filters use the inserted DEFAULT.
	for i := 0; i < 3; i++ {
		if err := store.SavePlayStats(ctx, sampleSnapshot("user-filter", "JP", "")); err != nil {
			t.Fatalf("SavePlayStats: %v", err)
		}
	}

	// 1) limit > 0 caps the result set
	limited, err := store.GetPlayStatsHistory(ctx, "user-filter", "JP", "", "", 2)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(limit=2): %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("limited rows = %d, want 2", len(limited))
	}

	// 2) "from" in the future excludes everything
	none, err := store.GetPlayStatsHistory(ctx, "user-filter", "JP", "2099-01-01", "", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(from=future): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("future-from rows = %d, want 0", len(none))
	}

	// 3) "to" in the far past excludes everything
	noneOld, err := store.GetPlayStatsHistory(ctx, "user-filter", "JP", "", "1970-01-01", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(to=past): %v", err)
	}
	if len(noneOld) != 0 {
		t.Errorf("past-to rows = %d, want 0", len(noneOld))
	}

	// 4) Filtering on a different user yields nothing
	other, err := store.GetPlayStatsHistory(ctx, "user-other", "JP", "", "", 0)
	if err != nil {
		t.Fatalf("GetPlayStatsHistory(other user): %v", err)
	}
	if len(other) != 0 {
		t.Errorf("other-user rows = %d, want 0", len(other))
	}
}

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
