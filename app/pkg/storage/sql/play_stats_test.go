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
