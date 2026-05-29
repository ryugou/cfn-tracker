package model

type BenchmarkPlayer struct {
	Id                int64              `db:"id" json:"id"`
	SourceUserId      string             `db:"source_user_id" json:"sourceUserId"`
	TargetUserId      string             `db:"target_user_id" json:"targetUserId"`
	FighterId         string             `db:"fighter_id" json:"fighterId"`
	Character         string             `db:"character" json:"character"`
	CharacterToolName string             `db:"character_tool_name" json:"characterToolName"`
	RankOffset        int                `db:"rank_offset" json:"rankOffset"`
	LeagueRank        int                `db:"league_rank" json:"leagueRank"`
	LP                int                `db:"lp" json:"lp"`
	MR                int                `db:"mr" json:"mr"`
	MRRanking         int                `db:"mr_ranking" json:"mrRanking"`
	Wins              int                `db:"wins" json:"wins"`
	Losses            int                `db:"losses" json:"losses"`
	WinDiff           int                `db:"win_diff" json:"winDiff"`
	LastPlayAt        int64              `db:"last_play_at" json:"lastPlayAt"`
	FetchedAt         string             `db:"fetched_at" json:"fetchedAt"`
	StatsJSON         string             `db:"stats_json" json:"-"`
	Stats             *PlayStatsSnapshot `db:"-" json:"stats"`
	LastError         string             `db:"last_error" json:"lastError"`
	CreatedAt         string             `db:"created_at" json:"createdAt"`
	UpdatedAt         string             `db:"updated_at" json:"updatedAt"`
}

type BenchmarkComparison struct {
	Self         *PlayStatsSnapshot     `json:"self"`
	Players      []*BenchmarkPlayer     `json:"players"`
	RankAverages []BenchmarkRankAverage `json:"rankAverages"`
}

type BenchmarkRankAverage struct {
	RankOffset int                `json:"rankOffset"`
	Count      int                `json:"count"`
	Stats      *PlayStatsSnapshot `json:"stats"`
}
