package sf6

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/cfn"
)

type SF6Tracker struct {
	cfnClient cfn.CFNClient
}

var _ tracker.GameTracker = (*SF6Tracker)(nil)

func NewSF6Tracker(cfnClient cfn.CFNClient) *SF6Tracker {
	return &SF6Tracker{
		cfnClient,
	}
}

func (t *SF6Tracker) GetUser(ctx context.Context, userCode string) (*model.User, error) {
	bl, err := t.cfnClient.GetBattleLog(ctx, userCode)
	if err != nil {
		return nil, fmt.Errorf("fetch battle log: %w", err)
	}
	return &model.User{
		DisplayName: bl.GetCFN(),
		Code:        userCode,
		LP:          bl.GetLP(),
		MR:          bl.GetMR(),
	}, nil
}

func (t *SF6Tracker) Poll(ctx context.Context, session *model.Session) (*model.Match, error) {
	bl, err := t.cfnClient.GetBattleLog(ctx, session.UserId)
	if err != nil {
		return nil, fmt.Errorf("cfn: get battle log: %w", err)
	}
	if bl == nil || len(bl.ReplayList) == 0 {
		return nil, nil
	}
	lastReplay := bl.ReplayList[0]
	var prevMatch model.Match
	if len(session.Matches) > 0 {
		prevMatch = getPreviousMatchForCharacter(session, bl.GetCharacter())
	}

	battleAt := time.Unix(lastReplay.UploadedAt, 0)
	if time.Since(battleAt).Minutes() >= 15 || prevMatch.ReplayID == lastReplay.ReplayID {
		return nil, nil
	}

	var opponent cfn.PlayerInfo
	if bl.GetCFN() == lastReplay.Player1Info.Player.FighterID {
		opponent = lastReplay.Player2Info
	} else {
		opponent = lastReplay.Player1Info
	}
	victory := !isVictory(opponent.RoundResults)
	wins := prevMatch.Wins
	losses := prevMatch.Losses
	winStreak := prevMatch.WinStreak
	if victory {
		wins++
		winStreak++
	} else {
		losses++
		winStreak = 0
	}

	return &model.Match{
		Character:         bl.GetCharacter(),
		LP:                bl.GetLP(),
		LeagueRank:        bl.GetLeagueRank(),
		MR:                bl.GetMR(),
		Opponent:          opponent.Player.FighterID,
		OpponentCharacter: opponent.CharacterName,
		OpponentLP:        opponent.LeaguePoint,
		OpponentLeague:    getLeagueFromLP(opponent.LeaguePoint),
		OpponentMR:        opponent.MasterRating,
		Victory:           victory,
		Wins:              wins,
		Losses:            losses,
		WinStreak:         winStreak,
		Date:              time.Now().Format(`2006-01-02`),
		Time:              time.Now().Format(`15:04`),
		LPGain:            prevMatch.LPGain + bl.GetLP() - session.LP,
		MRGain:            prevMatch.MRGain + bl.GetMR() - session.MR,
		WinRate:           int((float64(wins) / float64(wins+losses)) * 100),
		UserId:            session.UserId,
		UserName:          session.UserName,
		SessionId:         session.Id,
		ReplayID:          lastReplay.ReplayID,
	}, nil
}

func getPreviousMatchForCharacter(sesh *model.Session, character string) model.Match {
	for i := 0; i < len(sesh.Matches); i++ {
		match := sesh.Matches[i]
		if match.Character == character {
			return *match
		}
	}
	return model.Match{}
}

func isVictory(roundResults []int) bool {
	roundsPlayed := len(roundResults)
	losses := make([]int, 0, roundsPlayed)
	for _, result := range roundResults {
		if result == 0 {
			losses = append(losses, result)
		}
	}
	return (roundsPlayed == 3 && len(losses) == 1) || len(losses) == 0
}

func getLeagueFromLP(lp int) string {
	if lp >= 25000 {
		return `Master`
	} else if lp >= 20000 {
		return `Diamond`
	} else if lp >= 14000 {
		return `Platinum`
	} else if lp >= 9000 {
		return `Gold`
	} else if lp >= 5000 {
		return `Silver`
	} else if lp >= 3000 {
		return `Bronze`
	} else if lp >= 1000 {
		return `Iron`
	}

	return `Rookie`
}

func (t *SF6Tracker) Authenticate(ctx context.Context, email string, password string, statChan chan tracker.AuthStatus) {
	t.cfnClient.Authenticate(ctx, email, password, statChan)
}

// BackfillMatches scans /battlelog/rank pagination starting at page 1, returning
// matches not already in knownReplayIds. Stops as soon as a page yields zero
// new replays. Returns matches in chronological (oldest → newest) order so the
// caller can apply running win/loss/streak counters in sequence.
//
// session.UserId and session.UserName are used to populate Match fields.
// LP / MR / character on each Match come from the replay snapshot itself, not
// the current battle banner — so a backfilled match reflects the in-game state
// at that time.
func (t *SF6Tracker) BackfillMatches(
	ctx context.Context,
	session *model.Session,
	knownReplayIds map[string]bool,
) ([]model.Match, error) {
	const maxPages = 20 // safety cap; ~200 replays per cap

	collected := make([]model.Match, 0, 32)
	for page := 1; page <= maxPages; page++ {
		bl, err := t.cfnClient.GetBattleLogPage(ctx, session.UserId, page)
		if err != nil {
			return nil, fmt.Errorf("fetch page %d: %w", page, err)
		}
		if bl == nil || len(bl.ReplayList) == 0 {
			break
		}
		newOnThisPage := 0
		for _, r := range bl.ReplayList {
			if knownReplayIds[r.ReplayID] {
				continue
			}
			m := replayToMatch(bl, r, session)
			collected = append(collected, m)
			newOnThisPage++
		}
		if newOnThisPage == 0 {
			// caught up — every replay on this page already in DB
			break
		}
		if bl.TotalPage > 0 && page >= bl.TotalPage {
			break
		}
		// Surface silent truncation when Capcom reports more pages than our safety cap.
		if page == maxPages && bl.TotalPage > maxPages {
			slog.Warn(
				"backfill: hit page cap before exhausting capcom pagination",
				slog.Int("max_pages", maxPages),
				slog.Int("total_pages", bl.TotalPage),
			)
		}
	}

	// Reverse: collected is newest-first per Capcom; backfill needs oldest-first
	// so downstream win/loss/streak accumulation matches real play order.
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return collected, nil
}

// replayToMatch builds a Match from a single Replay without running counters.
// The TrackingHandler accumulates wins/losses/streak/LPGain/MRGain in the order
// matches are returned by BackfillMatches.
func replayToMatch(bl *cfn.BattleLog, r cfn.Replay, session *model.Session) model.Match {
	// Determine which side is the tracked user. The user's CFN ID is bl.GetCFN().
	userCfn := bl.GetCFN()
	var me, opp cfn.PlayerInfo
	if r.Player1Info.Player.FighterID == userCfn {
		me = r.Player1Info
		opp = r.Player2Info
	} else {
		me = r.Player2Info
		opp = r.Player1Info
	}
	victory := !isVictory(opp.RoundResults)

	battleAt := time.Unix(r.UploadedAt, 0)
	return model.Match{
		UserId:            session.UserId,
		UserName:          session.UserName,
		SessionId:         session.Id,
		ReplayID:          r.ReplayID,
		Character:         me.CharacterName,
		LP:                me.LeaguePoint,
		LeagueRank:        me.LeagueRank,
		MR:                me.MasterRating,
		Opponent:          opp.Player.FighterID,
		OpponentCharacter: opp.CharacterName,
		OpponentLP:        opp.LeaguePoint,
		OpponentLeague:    getLeagueFromLP(opp.LeaguePoint),
		OpponentMR:        opp.MasterRating,
		Victory:           victory,
		Date:              battleAt.Format("2006-01-02"),
		Time:              battleAt.Format("15:04"),
	}
}

// PlayStatsResult bundles the parsed /play data for storage. Character is
// the display name (FavoriteCharacterName) to match the existing
// matches.character column.
type PlayStatsResult struct {
	Character string
	Stats     *cfn.BattleStats
	BaseInfo  *cfn.BaseInfo
}

// PollPlayStats fetches a single /play snapshot for the given CFN user code.
// It is intentionally not part of the tracker.GameTracker interface — only
// SF6 has this concept — so callers must type-assert to *SF6Tracker.
func (t *SF6Tracker) PollPlayStats(ctx context.Context, userCode string) (*PlayStatsResult, error) {
	pp, err := t.cfnClient.GetPlayStats(ctx, userCode)
	if err != nil {
		return nil, fmt.Errorf("cfn: get play stats: %w", err)
	}
	return &PlayStatsResult{
		Character: pp.FighterBannerInfo.FavoriteCharacterName,
		Stats:     &pp.Play.BattleStats,
		BaseInfo:  &pp.Play.BaseInfo,
	}, nil
}
