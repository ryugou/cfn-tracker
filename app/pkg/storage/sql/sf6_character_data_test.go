package sql

import (
	"context"
	"testing"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
)

func TestFindSF6CharacterMovesMatchesAnyTerm(t *testing.T) {
	store, err := newTestStorage()
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	ctx := context.Background()
	moves := []model.SF6CharacterMove{
		{
			Character: "ingrid",
			Locale:    "ja-jp",
			Source:    "frame",
			Category:  "共通システム",
			Name:      "ドライブインパクト（トゥインクルキック）",
			Command:   "強強",
			Startup:   "26",
			Remarks:   "アーマー判定",
		},
		{
			Character: "ingrid",
			Locale:    "ja-jp",
			Source:    "frame",
			Category:  "通常技",
			Name:      "しゃがみ中K（ミルキーウェイ）",
			Command:   "中",
			Startup:   "8",
			Cancel:    "C",
		},
		{
			Character: "ingrid",
			Locale:    "ja-jp",
			Source:    "frame",
			Category:  "通常投げ",
			Name:      "ストレンジナックル",
			Command:   "弱弱",
			Startup:   "3",
		},
	}
	if err := store.ReplaceSF6CharacterMoves(ctx, "ingrid", "ja-jp", moves); err != nil {
		t.Fatalf("ReplaceSF6CharacterMoves: %v", err)
	}

	found, err := store.FindSF6CharacterMoves(ctx, "ingrid", "ja-jp", []string{"ドライブインパクト", "キャンセル"}, 10)
	if err != nil {
		t.Fatalf("FindSF6CharacterMoves: %v", err)
	}
	seen := map[string]bool{}
	for _, move := range found {
		seen[move.Name] = true
	}
	if !seen["ドライブインパクト（トゥインクルキック）"] {
		t.Fatalf("missing DI row: %#v", found)
	}
	if !seen["しゃがみ中K（ミルキーウェイ）"] {
		t.Fatalf("missing cancelable row: %#v", found)
	}
	if seen["ストレンジナックル"] {
		t.Fatalf("unexpected throw row: %#v", found)
	}
}
