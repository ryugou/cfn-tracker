package model

import "database/sql"

// PlayStatsSnapshot is one row of the play_stats_snapshots table. Field tags
// align with both DB column names (db:) and JSON keys emitted to the
// frontend (json:). MatchReplayId is nullable because the baseline snapshot
// captured at tracking start has no associated match yet.
type PlayStatsSnapshot struct {
	Id            int64          `db:"id" json:"id"`
	UserId        string         `db:"user_id" json:"userId"`
	Character     string         `db:"character" json:"character"`
	MatchReplayId sql.NullString `db:"match_replay_id" json:"matchReplayId"`
	SnapshotAt    string         `db:"snapshot_at" json:"snapshotAt"`

	// battle_stats
	BattleHubMatchPlayCount          int     `db:"battle_hub_match_play_count" json:"battleHubMatchPlayCount"`
	CasualMatchPlayCount             int     `db:"casual_match_play_count" json:"casualMatchPlayCount"`
	CornerTime                       float64 `db:"corner_time" json:"cornerTime"`
	CorneredTime                     float64 `db:"cornered_time" json:"corneredTime"`
	CustomRoomMatchPlayCount         int     `db:"custom_room_match_play_count" json:"customRoomMatchPlayCount"`
	DriveImpact                      float64 `db:"drive_impact" json:"driveImpact"`
	DriveImpactToDriveImpact         float64 `db:"drive_impact_to_drive_impact" json:"driveImpactToDriveImpact"`
	DriveParry                       float64 `db:"drive_parry" json:"driveParry"`
	DriveReversal                    float64 `db:"drive_reversal" json:"driveReversal"`
	GaugeRateCA                      float64 `db:"gauge_rate_ca" json:"gaugeRateCA"`
	GaugeRateDriveArts               float64 `db:"gauge_rate_drive_arts" json:"gaugeRateDriveArts"`
	GaugeRateDriveGuard              float64 `db:"gauge_rate_drive_guard" json:"gaugeRateDriveGuard"`
	GaugeRateDriveImpact             float64 `db:"gauge_rate_drive_impact" json:"gaugeRateDriveImpact"`
	GaugeRateDriveOther              float64 `db:"gauge_rate_drive_other" json:"gaugeRateDriveOther"`
	GaugeRateDriveReversal           float64 `db:"gauge_rate_drive_reversal" json:"gaugeRateDriveReversal"`
	GaugeRateDriveRushFromCancel     float64 `db:"gauge_rate_drive_rush_from_cancel" json:"gaugeRateDriveRushFromCancel"`
	GaugeRateDriveRushFromParry      float64 `db:"gauge_rate_drive_rush_from_parry" json:"gaugeRateDriveRushFromParry"`
	GaugeRateSALv1                   float64 `db:"gauge_rate_sa_lv1" json:"gaugeRateSALv1"`
	GaugeRateSALv2                   float64 `db:"gauge_rate_sa_lv2" json:"gaugeRateSALv2"`
	GaugeRateSALv3                   float64 `db:"gauge_rate_sa_lv3" json:"gaugeRateSALv3"`
	JustParry                        float64 `db:"just_parry" json:"justParry"`
	PunishCounter                    float64 `db:"punish_counter" json:"punishCounter"`
	RankMatchPlayCount               int     `db:"rank_match_play_count" json:"rankMatchPlayCount"`
	ReceivedDriveImpact              float64 `db:"received_drive_impact" json:"receivedDriveImpact"`
	ReceivedDriveImpactToDriveImpact float64 `db:"received_drive_impact_to_drive_impact" json:"receivedDriveImpactToDriveImpact"`
	ReceivedPunishCounter            float64 `db:"received_punish_counter" json:"receivedPunishCounter"`
	ReceivedStun                     float64 `db:"received_stun" json:"receivedStun"`
	ReceivedThrowCount               float64 `db:"received_throw_count" json:"receivedThrowCount"`
	ReceivedThrowDriveParry          float64 `db:"received_throw_drive_parry" json:"receivedThrowDriveParry"`
	RivalAIAchievedChallengeCount    int     `db:"rival_ai_achieved_challenge_count" json:"rivalAIAchievedChallengeCount"`
	RivalAIHighestLeagueRank         int     `db:"rival_ai_highest_league_rank" json:"rivalAIHighestLeagueRank"`
	RivalAIHighestLeagueRankTxt      string  `db:"rival_ai_highest_league_rank_txt" json:"rivalAIHighestLeagueRankTxt"`
	Stun                             float64 `db:"stun" json:"stun"`
	TargetClearCount                 int     `db:"target_clear_count" json:"targetClearCount"`
	ThrowCount                       float64 `db:"throw_count" json:"throwCount"`
	ThrowDriveParry                  float64 `db:"throw_drive_parry" json:"throwDriveParry"`
	ThrowTech                        float64 `db:"throw_tech" json:"throwTech"`
	TotalAllCharacterPlayPoint       int     `db:"total_all_character_play_point" json:"totalAllCharacterPlayPoint"`

	// base_info enjoy
	EnjoyFightPoint int `db:"enjoy_fight_point" json:"enjoyFightPoint"`
	EnjoyTotalPoint int `db:"enjoy_total_point" json:"enjoyTotalPoint"`
	EnjoyUserPoint  int `db:"enjoy_user_point" json:"enjoyUserPoint"`

	// base_info content_play_time_list (seconds per content_type)
	WorldTourSeconds    int `db:"world_tour_seconds" json:"worldTourSeconds"`
	RankedMatchSeconds  int `db:"ranked_match_seconds" json:"rankedMatchSeconds"`
	CasualMatchSeconds  int `db:"casual_match_seconds" json:"casualMatchSeconds"`
	CustomRoomSeconds   int `db:"custom_room_seconds" json:"customRoomSeconds"`
	BattleHubSeconds    int `db:"battle_hub_seconds" json:"battleHubSeconds"`
	OfflineMatchSeconds int `db:"offline_match_seconds" json:"offlineMatchSeconds"`
	ArcadeSeconds       int `db:"arcade_seconds" json:"arcadeSeconds"`
	PracticeSeconds     int `db:"practice_seconds" json:"practiceSeconds"`
	ExtremeSeconds      int `db:"extreme_seconds" json:"extremeSeconds"`
}

// MatchWithStats joins one Match row with its corresponding play stats
// snapshot (if any). Stats is nil when no snapshot exists for the match.
type MatchWithStats struct {
	Match Match              `json:"match"`
	Stats *PlayStatsSnapshot `json:"stats"`
}
