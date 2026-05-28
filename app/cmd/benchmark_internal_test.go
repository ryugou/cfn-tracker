package cmd

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/williamsjokvist/cfn-tracker/pkg/tracker"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/cfn"
)

type fakeBenchmarkCFNClient struct {
	search map[int][]cfn.FighterBanner
}

func (f fakeBenchmarkCFNClient) GetBattleLog(context.Context, string) (*cfn.BattleLog, error) {
	return nil, nil
}

func (f fakeBenchmarkCFNClient) GetBattleLogPage(context.Context, string, int) (*cfn.BattleLog, error) {
	return nil, nil
}

func (f fakeBenchmarkCFNClient) GetPlayStats(context.Context, string) (*cfn.PlayPageProps, error) {
	return nil, nil
}

func (f fakeBenchmarkCFNClient) SearchFighters(_ context.Context, params cfn.FighterSearchParams) (*cfn.FighterSearchPageProps, error) {
	if params.Page > 1 {
		return &cfn.FighterSearchPageProps{}, nil
	}
	return &cfn.FighterSearchPageProps{
		Common:            cfn.CommonProps{StatusCode: 200},
		FighterBannerList: f.search[params.LeagueRankMin],
		Page:              params.Page,
	}, nil
}

func (f fakeBenchmarkCFNClient) Authenticate(context.Context, string, string, chan tracker.AuthStatus) {
}

func TestFindBenchmarkCandidatesSelectsTopPlayersForNextRanks(t *testing.T) {
	ch := &CommandHandler{
		cfnClient: fakeBenchmarkCFNClient{
			search: map[int][]cfn.FighterBanner{
				9: {
					fighter("rank9-low", 91, 9, 2100, 0),
					fighter("rank9-top", 92, 9, 2500, 0),
					fighter("rank9-mid", 93, 9, 2300, 0),
					fighter("rank9-second", 94, 9, 2400, 0),
					fighter("rank9-fifth", 95, 9, 2200, 0),
					fighter("rank9-fourth", 96, 9, 2250, 0),
				},
				10: {
					fighter("rank10-third", 103, 10, 2700, 0),
					fighter("rank10-top", 101, 10, 2900, 0),
					fighter("rank10-second", 102, 10, 2800, 0),
					fighter("rank10-fourth", 104, 10, 2600, 0),
					fighter("rank10-fifth", 105, 10, 2500, 0),
				},
			},
		},
	}

	got, err := ch.findBenchmarkCandidates(context.Background(), battleLog("self", 42, 8, 2000, 0), "jp")
	if err != nil {
		t.Fatalf("findBenchmarkCandidates: %v", err)
	}

	assertShortIDs(t, got[1], []int64{92, 94, 93, 96, 95})
	assertShortIDs(t, got[2], []int64{101, 102, 103, 104, 105})
}

func TestFindBenchmarkCandidatesSplitsMasterPlayersByClosestHigherMR(t *testing.T) {
	ch := &CommandHandler{
		cfnClient: fakeBenchmarkCFNClient{
			search: map[int][]cfn.FighterBanner{
				36: {
					fighter("too-low", 1, 36, 25000, 1499),
					fighter("self", 42, 36, 25000, 1500),
					fighter("mr-1600", 1600, 36, 25000, 1600),
					fighter("mr-1510", 1510, 36, 25000, 1510),
					fighter("mr-1550", 1550, 36, 25000, 1550),
					fighter("mr-1520", 1520, 36, 25000, 1520),
					fighter("mr-1530", 1530, 36, 25000, 1530),
					fighter("mr-1540", 1540, 36, 25000, 1540),
					fighter("mr-1560", 1560, 36, 25000, 1560),
					fighter("mr-1570", 1570, 36, 25000, 1570),
					fighter("mr-1580", 1580, 36, 25000, 1580),
					fighter("mr-1590", 1590, 36, 25000, 1590),
				},
			},
		},
	}

	got, err := ch.findBenchmarkCandidates(context.Background(), battleLog("self", 42, 36, 25000, 1500), "jp")
	if err != nil {
		t.Fatalf("findBenchmarkCandidates: %v", err)
	}

	assertShortIDs(t, got[1], []int64{1510, 1520, 1530, 1540, 1550})
	assertShortIDs(t, got[2], []int64{1560, 1570, 1580, 1590, 1600})
}

func fighter(name string, shortID int64, leagueRank, lp, mr int) cfn.FighterBanner {
	var f cfn.FighterBanner
	f.FavoriteCharacterName = "JP"
	f.FavoriteCharacterToolName = "jp"
	f.FavoriteCharacterLeagueInfo.LeagueRank = leagueRank
	f.FavoriteCharacterLeagueInfo.LeaguePoint = lp
	f.FavoriteCharacterLeagueInfo.MasterRating = mr
	f.PersonalInfo.FighterID = name
	f.PersonalInfo.ShortID = shortID
	f.LastPlayAt = 1779860000 + shortID
	return f
}

func battleLog(name string, shortID int64, leagueRank, lp, mr int) *cfn.BattleLog {
	var bl cfn.BattleLog
	bl.FighterBannerInfo.FavoriteCharacterName = "JP"
	bl.FighterBannerInfo.FavoriteCharacterToolName = "jp"
	bl.FighterBannerInfo.FavoriteCharacterLeagueInfo.LeagueRank = leagueRank
	bl.FighterBannerInfo.FavoriteCharacterLeagueInfo.LeaguePoint = lp
	bl.FighterBannerInfo.FavoriteCharacterLeagueInfo.MasterRating = mr
	bl.FighterBannerInfo.PersonalInfo.FighterID = name
	bl.FighterBannerInfo.PersonalInfo.ShortID = shortID
	return &bl
}

func assertShortIDs(t *testing.T, got []cfn.FighterBanner, want []int64) {
	t.Helper()
	ids := make([]int64, 0, len(got))
	for _, f := range got {
		ids = append(ids, f.PersonalInfo.ShortID)
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("short IDs = %v, want %v", ids, want)
	}
	for _, id := range ids {
		if id == 42 {
			t.Fatalf("self short ID %s should not be selected", strconv.FormatInt(id, 10))
		}
	}
	if len(ids) > benchmarkPlayersPerRank {
		t.Fatalf("selected %d players, want at most %d", len(ids), benchmarkPlayersPerRank)
	}
	for i, id := range ids {
		if id == 0 {
			t.Fatalf("short ID at index %d is zero", i)
		}
		if got[i].PersonalInfo.FighterID == "" {
			t.Fatalf("fighter ID for %s is empty", fmt.Sprint(id))
		}
	}
}
