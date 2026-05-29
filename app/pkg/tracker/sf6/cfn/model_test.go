package cfn

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPlayPageUnmarshal(t *testing.T) {
	data, err := os.ReadFile("testdata/play-stats-sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var pp PlayPageProps
	if err := json.Unmarshal(data, &pp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if pp.Common.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", pp.Common.StatusCode)
	}
	if pp.Play.BattleStats.DriveImpact <= 0 {
		t.Errorf("BattleStats.DriveImpact = %v, want > 0", pp.Play.BattleStats.DriveImpact)
	}
	if got := pp.Play.BattleStats.GaugeRateSALv1 + pp.Play.BattleStats.GaugeRateSALv2 +
		pp.Play.BattleStats.GaugeRateSALv3 + pp.Play.BattleStats.GaugeRateCA; got <= 0 {
		t.Errorf("SA gauge rates sum = %v, want > 0", got)
	}
	if len(pp.Play.BaseInfo.ContentPlayTimeList) != 9 {
		t.Fatalf("ContentPlayTimeList len = %d, want 9", len(pp.Play.BaseInfo.ContentPlayTimeList))
	}
	if pp.FighterBannerInfo.FavoriteCharacterName == "" {
		t.Errorf("FavoriteCharacterName is empty")
	}
	if len(pp.Play.CharacterWinRates) == 0 {
		t.Fatalf("CharacterWinRates is empty")
	}

	// Equality assertions against known fixture values catch json-tag regressions.
	if pp.Play.BattleStats.RankMatchPlayCount != 59 {
		t.Errorf("RankMatchPlayCount = %d, want 59", pp.Play.BattleStats.RankMatchPlayCount)
	}
	if pp.Play.BattleStats.RivalAIHighestLeagueRankTxt != "Rookie 2" {
		t.Errorf("RivalAIHighestLeagueRankTxt = %q, want %q", pp.Play.BattleStats.RivalAIHighestLeagueRankTxt, "Rookie 2")
	}
	if pp.Play.BaseInfo.ContentPlayTimeList[0].ContentType != ContentTypeWorldTour {
		t.Errorf("ContentPlayTimeList[0].ContentType = %d, want %d (WorldTour)", pp.Play.BaseInfo.ContentPlayTimeList[0].ContentType, ContentTypeWorldTour)
	}
	if pp.Play.BaseInfo.ContentPlayTimeList[7].PlayTime != 159402 {
		t.Errorf("ContentPlayTimeList[7].PlayTime = %d, want 159402 (Practice seconds)", pp.Play.BaseInfo.ContentPlayTimeList[7].PlayTime)
	}
}

func TestPlayPageDocUnmarshal(t *testing.T) {
	inner, err := os.ReadFile("testdata/play-stats-sample.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	// Wrap the existing pageProps-level fixture as a full __NEXT_DATA__ doc.
	wrapped := []byte(`{"props":{"pageProps":` + string(inner) + `}}`)

	var doc PlayPageDoc
	if err := json.Unmarshal(wrapped, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Props.PageProps.Common.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", doc.Props.PageProps.Common.StatusCode)
	}
	if doc.Props.PageProps.Play.BattleStats.RankMatchPlayCount <= 0 {
		t.Errorf("RankMatchPlayCount = %d, want > 0", doc.Props.PageProps.Play.BattleStats.RankMatchPlayCount)
	}
}
