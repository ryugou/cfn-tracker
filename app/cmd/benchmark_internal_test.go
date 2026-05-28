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

func TestFindBenchmarkCandidatesSelectsTwoUpAndNextLeague(t *testing.T) {
	ch := &CommandHandler{
		cfnClient: fakeBenchmarkCFNClient{
			search: map[int][]cfn.FighterBanner{
				10: {
					fighter("rank10-third", 103, 10, 2700, 0),
					fighter("rank10-top", 101, 10, 2900, 0),
					fighter("rank10-second", 102, 10, 2800, 0),
					fighter("rank10-fourth", 104, 10, 2600, 0),
					fighter("rank10-fifth", 105, 10, 2500, 0),
				},
				11: {
					fighter("rank11-low", 111, 11, 3000, 0),
					fighter("rank11-top", 112, 11, 3400, 0),
					fighter("rank11-mid", 113, 11, 3200, 0),
					fighter("rank11-second", 114, 11, 3300, 0),
					fighter("rank11-fifth", 115, 11, 3100, 0),
					fighter("rank11-fourth", 116, 11, 3150, 0),
				},
			},
		},
	}

	got, err := ch.findBenchmarkCandidates(context.Background(), battleLog("self", 42, 8, 2000, 0), "jp")
	if err != nil {
		t.Fatalf("findBenchmarkCandidates: %v", err)
	}

	assertShortIDs(t, got[1], []int64{101, 102, 103, 104, 105})
	assertShortIDs(t, got[2], []int64{112, 114, 113, 116, 115})
}

func TestFindBenchmarkCandidatesSelectsMasterPlayersAroundTargetMR(t *testing.T) {
	ch := &CommandHandler{
		cfnClient: fakeBenchmarkCFNClient{
			search: map[int][]cfn.FighterBanner{
				36: {
					fighter("too-low", 1, 36, 25000, 1499),
					fighter("self", 42, 36, 25000, 1500),
					fighter("mr-1590", 1590, 36, 25000, 1590),
					fighter("mr-1600", 1600, 36, 25000, 1600),
					fighter("mr-1610", 1610, 36, 25000, 1610),
					fighter("mr-1580", 1580, 36, 25000, 1580),
					fighter("mr-1620", 1620, 36, 25000, 1620),
					fighter("mr-1700", 1700, 36, 25000, 1700),
					fighter("mr-1690", 1690, 36, 25000, 1690),
					fighter("mr-1710", 1710, 36, 25000, 1710),
					fighter("mr-1680", 1680, 36, 25000, 1680),
					fighter("mr-1720", 1720, 36, 25000, 1720),
				},
			},
		},
	}

	got, err := ch.findBenchmarkCandidates(context.Background(), battleLog("self", 42, 36, 25000, 1500), "jp")
	if err != nil {
		t.Fatalf("findBenchmarkCandidates: %v", err)
	}

	assertShortIDs(t, got[1], []int64{1600, 1610, 1590, 1620, 1580})
	assertShortIDs(t, got[2], []int64{1700, 1710, 1690, 1720, 1680})
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
