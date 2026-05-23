package cmd_test

import (
	"fmt"
	"testing"

	"github.com/williamsjokvist/cfn-tracker/cmd"
	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

func TestTrackingSelectGame(t *testing.T) {
	tests := []struct {
		name      string
		gameType  model.GameType
		expErrMsg string
	}{
		{
			name:     "select tekken 8",
			gameType: model.GameTypeT8,
		},
		{
			name:      "select SF6",
			gameType:  model.GameTypeSF6,
			expErrMsg: "unauthenticated: browser not initialized",
		},
		{
			name:      "select game that doesn't exist",
			gameType:  "undefined",
			expErrMsg: "select game: game does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := testSuite.trackingHandler.SelectGame(tt.gameType)
			if err == nil && tt.expErrMsg != "" {
				t.Errorf("expected error: %q, got: nil", tt.expErrMsg)
			}
			if err != nil && err.Error() != tt.expErrMsg {
				t.Errorf("unexpected error: got: %q, want: %q", err, tt.expErrMsg)
			}
		})
	}
}

func TestApplyRunningCounters(t *testing.T) {
	// Simulate the failing progression from real data:
	// win, lose, lose, win -> wins/losses should monotonically grow per side.
	// Earlier bug: only the incremented side was set on each match, leaving
	// the other counter at 0. Asserts both sides carry forward correctly.
	type step struct {
		victory     bool
		wantWins    int
		wantLosses  int
		wantStreak  int
		wantWinRate int
	}
	steps := []step{
		{victory: true, wantWins: 1, wantLosses: 0, wantStreak: 1, wantWinRate: 100},
		{victory: false, wantWins: 1, wantLosses: 1, wantStreak: 0, wantWinRate: 50},
		{victory: false, wantWins: 1, wantLosses: 2, wantStreak: 0, wantWinRate: 33},
		{victory: true, wantWins: 2, wantLosses: 2, wantStreak: 1, wantWinRate: 50},
	}
	prev := model.Match{}
	for i, s := range steps {
		curr := model.Match{Victory: s.victory, ReplayID: fmt.Sprintf("r%d", i)}
		curr = cmd.ApplyRunningCounters(curr, prev)
		if curr.Wins != s.wantWins {
			t.Errorf("step %d wins = %d, want %d", i, curr.Wins, s.wantWins)
		}
		if curr.Losses != s.wantLosses {
			t.Errorf("step %d losses = %d, want %d", i, curr.Losses, s.wantLosses)
		}
		if curr.WinStreak != s.wantStreak {
			t.Errorf("step %d streak = %d, want %d", i, curr.WinStreak, s.wantStreak)
		}
		if curr.WinRate != s.wantWinRate {
			t.Errorf("step %d win rate = %d%%, want %d%%", i, curr.WinRate, s.wantWinRate)
		}
		prev = curr
	}
}
