package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/williamsjokvist/cfn-tracker/pkg/model"
	"github.com/williamsjokvist/cfn-tracker/pkg/tracker/sf6/official"
)

func (ch *CommandHandler) SyncSF6CharacterData(character, locale string) ([]model.SF6CharacterMove, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	character = official.NormalizeCharacterSlug(character)
	if character == "" {
		return nil, model.WrapError(model.ErrGetPlayStats, fmt.Errorf("character is empty"))
	}
	if locale == "" {
		locale = "ja-jp"
	}
	moves, err := official.NewClient().FetchCharacterData(ctx, character, locale)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	if err := ch.sqlDb.ReplaceSF6CharacterMoves(ctx, character, locale, moves); err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return ch.sqlDb.GetSF6CharacterMoves(ctx, character, locale, 500)
}

func (ch *CommandHandler) GetSF6CharacterMoves(character, locale string, limit int) ([]model.SF6CharacterMove, error) {
	character = official.NormalizeCharacterSlug(character)
	if locale == "" {
		locale = "ja-jp"
	}
	moves, err := ch.sqlDb.GetSF6CharacterMoves(context.Background(), character, locale, limit)
	if err != nil {
		return nil, model.WrapError(model.ErrGetPlayStats, err)
	}
	return moves, nil
}
