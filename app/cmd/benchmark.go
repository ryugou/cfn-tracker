package cmd

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/cfn"
)

const (
	benchmarkPlayersPerRank = 5
	benchmarkSearchPages    = 3
)

func (ch *CommandHandler) RefreshBenchmarkPlayers(userId, character string) ([]*model.BenchmarkPlayer, error) {
	ctx := context.Background()
	if ch.cfnClient == nil {
		return nil, model.WrapError(model.ErrGetPlayStats, fmt.Errorf("cfn client is not configured"))
	}

	bl, err := ch.cfnClient.GetBattleLog(ctx, userId)
	if err != nil {
		return nil, model.WrapError(model.ErrGetUser, err)
	}
	if bl == nil {
		return nil, model.WrapError(model.ErrGetUser, fmt.Errorf("battle log is empty"))
	}

	characterName := character
	if characterName == "" {
		characterName = bl.GetCharacter()
	}
	characterToolName := characterToolName(characterName)
	if characterName == bl.GetCharacter() {
		characterToolName = bl.GetCharacterToolName()
	}

	candidatesByOffset, err := ch.findBenchmarkCandidates(ctx, bl, characterToolName)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}

	players := make([]*model.BenchmarkPlayer, 0, benchmarkPlayersPerRank*2)
	for _, offset := range []int{1, 2} {
		for _, candidate := range candidatesByOffset[offset] {
			player := benchmarkPlayerFromCandidate(userId, characterName, offset, candidate)
			pp, err := ch.cfnClient.GetPlayStats(ctx, player.TargetUserId)
			if err != nil {
				player.LastError = err.Error()
				players = append(players, player)
				continue
			}
			res := &sf6.PlayStatsResult{
				Character: pp.FighterBannerInfo.FavoriteCharacterName,
				Stats:     &pp.Play.BattleStats,
				BaseInfo:  &pp.Play.BaseInfo,
			}
			stats := buildSnapshot(player.TargetUserId, res, "")
			player.Stats = &stats
			players = append(players, player)
		}
	}

	if err := ch.sqlDb.SaveBenchmarkPlayers(ctx, userId, characterName, players); err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return ch.sqlDb.GetBenchmarkPlayers(ctx, userId, characterName)
}

func (ch *CommandHandler) GetBenchmarkPlayers(userId, character string) ([]*model.BenchmarkPlayer, error) {
	rows, err := ch.sqlDb.GetBenchmarkPlayers(context.Background(), userId, character)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return rows, nil
}

func (ch *CommandHandler) GetBenchmarkComparison(userId, character string) (*model.BenchmarkComparison, error) {
	ctx := context.Background()
	players, err := ch.sqlDb.GetBenchmarkPlayers(ctx, userId, character)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	self, err := ch.sqlDb.GetLatestPlayStatsSnapshot(ctx, userId)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return &model.BenchmarkComparison{
		Self:         self,
		Players:      players,
		RankAverages: benchmarkAverages(players),
	}, nil
}

func (ch *CommandHandler) findBenchmarkCandidates(
	ctx context.Context,
	bl *cfn.BattleLog,
	characterToolName string,
) (map[int][]cfn.FighterBanner, error) {
	selfId := bl.GetUserCode()
	selfRank := bl.GetLeagueRank()
	selfMR := bl.GetMR()

	if selfRank >= 36 {
		candidates, err := ch.searchBenchmarkRank(ctx, characterToolName, 36)
		if err != nil {
			return nil, err
		}
		filtered := make([]cfn.FighterBanner, 0, len(candidates))
		for _, candidate := range candidates {
			if strconv.FormatInt(candidate.PersonalInfo.ShortID, 10) == selfId {
				continue
			}
			if candidate.FavoriteCharacterLeagueInfo.MasterRating > selfMR {
				filtered = append(filtered, candidate)
			}
		}
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].FavoriteCharacterLeagueInfo.MasterRating <
				filtered[j].FavoriteCharacterLeagueInfo.MasterRating
		})
		return map[int][]cfn.FighterBanner{
			1: takeFighters(filtered, 0, benchmarkPlayersPerRank),
			2: takeFighters(filtered, benchmarkPlayersPerRank, benchmarkPlayersPerRank),
		}, nil
	}

	out := map[int][]cfn.FighterBanner{}
	used := map[string]bool{selfId: true}
	for _, offset := range []int{1, 2} {
		targetRank := selfRank + offset
		if targetRank > 36 {
			targetRank = 36
		}
		candidates, err := ch.searchBenchmarkRank(ctx, characterToolName, targetRank)
		if err != nil {
			return nil, err
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].FavoriteCharacterLeagueInfo.LeaguePoint == candidates[j].FavoriteCharacterLeagueInfo.LeaguePoint {
				return candidates[i].FavoriteCharacterLeagueInfo.MasterRating > candidates[j].FavoriteCharacterLeagueInfo.MasterRating
			}
			return candidates[i].FavoriteCharacterLeagueInfo.LeaguePoint > candidates[j].FavoriteCharacterLeagueInfo.LeaguePoint
		})
		selected := make([]cfn.FighterBanner, 0, benchmarkPlayersPerRank)
		for _, candidate := range candidates {
			id := strconv.FormatInt(candidate.PersonalInfo.ShortID, 10)
			if used[id] {
				continue
			}
			selected = append(selected, candidate)
			used[id] = true
			if len(selected) >= benchmarkPlayersPerRank {
				break
			}
		}
		out[offset] = selected
	}
	return out, nil
}

func (ch *CommandHandler) searchBenchmarkRank(ctx context.Context, characterToolName string, rank int) ([]cfn.FighterBanner, error) {
	out := make([]cfn.FighterBanner, 0, benchmarkSearchPages*100)
	for page := 1; page <= benchmarkSearchPages; page++ {
		res, err := ch.cfnClient.SearchFighters(ctx, cfn.FighterSearchParams{
			CharacterToolName: characterToolName,
			LeagueRankMin:     rank,
			LeagueRankMax:     rank,
			Page:              page,
		})
		if err != nil {
			return nil, err
		}
		if res == nil || len(res.FighterBannerList) == 0 {
			break
		}
		out = append(out, res.FighterBannerList...)
		if len(res.FighterBannerList) < 100 {
			break
		}
	}
	return out, nil
}

func benchmarkPlayerFromCandidate(sourceUserId, character string, rankOffset int, candidate cfn.FighterBanner) *model.BenchmarkPlayer {
	league := candidate.FavoriteCharacterLeagueInfo
	return &model.BenchmarkPlayer{
		SourceUserId:      sourceUserId,
		TargetUserId:      strconv.FormatInt(candidate.PersonalInfo.ShortID, 10),
		FighterId:         candidate.PersonalInfo.FighterID,
		Character:         character,
		CharacterToolName: candidate.FavoriteCharacterToolName,
		RankOffset:        rankOffset,
		LeagueRank:        league.LeagueRank,
		LP:                league.LeaguePoint,
		MR:                league.MasterRating,
		MRRanking:         league.MasterRatingRanking,
		LastPlayAt:        candidate.LastPlayAt,
	}
}

func benchmarkAverages(players []*model.BenchmarkPlayer) []model.BenchmarkRankAverage {
	out := make([]model.BenchmarkRankAverage, 0, 2)
	for _, offset := range []int{1, 2} {
		stats := make([]*model.PlayStatsSnapshot, 0, benchmarkPlayersPerRank)
		for _, player := range players {
			if player != nil && player.RankOffset == offset && player.Stats != nil {
				stats = append(stats, player.Stats)
			}
		}
		out = append(out, model.BenchmarkRankAverage{
			RankOffset: offset,
			Count:      len(stats),
			Stats:      averagePlayStats(stats),
		})
	}
	return out
}

func averagePlayStats(rows []*model.PlayStatsSnapshot) *model.PlayStatsSnapshot {
	if len(rows) == 0 {
		return nil
	}
	avg := &model.PlayStatsSnapshot{}
	dst := reflect.ValueOf(avg).Elem()
	for i := 0; i < dst.NumField(); i++ {
		field := dst.Field(i)
		if !field.CanSet() {
			continue
		}
		var totalFloat float64
		var totalInt int64
		for _, row := range rows {
			src := reflect.ValueOf(row).Elem().Field(i)
			switch field.Kind() {
			case reflect.Float64:
				totalFloat += src.Float()
			case reflect.Int, reflect.Int64:
				totalInt += src.Int()
			}
		}
		switch field.Kind() {
		case reflect.Float64:
			field.SetFloat(totalFloat / float64(len(rows)))
		case reflect.Int, reflect.Int64:
			field.SetInt(totalInt / int64(len(rows)))
		}
	}
	return avg
}

func takeFighters(in []cfn.FighterBanner, start, count int) []cfn.FighterBanner {
	if start >= len(in) {
		return []cfn.FighterBanner{}
	}
	end := start + count
	if end > len(in) {
		end = len(in)
	}
	return in[start:end]
}

func characterToolName(character string) string {
	key := strings.ToLower(strings.ReplaceAll(character, " ", ""))
	key = strings.ReplaceAll(key, ".", "")
	key = strings.ReplaceAll(key, "-", "")
	names := map[string]string{
		"edmondhonda": "honda",
		"ehonda":      "honda",
		"deeJay":      "deejay",
		"deejay":      "deejay",
		"chunli":      "chunli",
		"akuma":       "gouki",
		"gouki":       "gouki",
		"mbison":      "vega",
		"vega":        "vega",
		"aki":         "aki",
		"aviper":      "cviper",
		"cviper":      "cviper",
	}
	if v, ok := names[key]; ok {
		return v
	}
	return key
}
