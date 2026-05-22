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
		t.Errorf("ContentPlayTimeList len = %d, want 9", len(pp.Play.BaseInfo.ContentPlayTimeList))
	}
	if pp.FighterBannerInfo.FavoriteCharacterName == "" {
		t.Errorf("FavoriteCharacterName is empty")
	}
}
