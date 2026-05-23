package cmd

import (
	"fmt"
	"testing"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

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
		curr = applyRunningCounters(curr, prev)
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
