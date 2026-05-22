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
