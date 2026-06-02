package model

type SF6CharacterMove struct {
	Id                   int64  `db:"id" json:"id"`
	Character            string `db:"character" json:"character"`
	Locale               string `db:"locale" json:"locale"`
	Source               string `db:"source" json:"source"`
	Category             string `db:"category" json:"category"`
	Name                 string `db:"name" json:"name"`
	Command              string `db:"command" json:"command"`
	Description          string `db:"description" json:"description"`
	Startup              string `db:"startup" json:"startup"`
	Active               string `db:"active" json:"active"`
	Recovery             string `db:"recovery" json:"recovery"`
	HitAdvantage         string `db:"hit_advantage" json:"hitAdvantage"`
	BlockAdvantage       string `db:"block_advantage" json:"blockAdvantage"`
	Cancel               string `db:"cancel" json:"cancel"`
	Damage               string `db:"damage" json:"damage"`
	ComboScaling         string `db:"combo_scaling" json:"comboScaling"`
	DriveGaugeGainHit    string `db:"drive_gauge_gain_hit" json:"driveGaugeGainHit"`
	DriveGaugeLossBlock  string `db:"drive_gauge_loss_block" json:"driveGaugeLossBlock"`
	DriveGaugeLossPunish string `db:"drive_gauge_loss_punish" json:"driveGaugeLossPunish"`
	SAGaugeGain          string `db:"sa_gauge_gain" json:"saGaugeGain"`
	Attribute            string `db:"attribute" json:"attribute"`
	Remarks              string `db:"remarks" json:"remarks"`
	RawText              string `db:"raw_text" json:"rawText"`
	SourceURL            string `db:"source_url" json:"sourceUrl"`
	FetchedAt            string `db:"fetched_at" json:"fetchedAt"`
	CreatedAt            string `db:"created_at" json:"createdAt"`
	UpdatedAt            string `db:"updated_at" json:"updatedAt"`
}
