package model

type AdviceMode string

const (
	AdviceModeDBOnly   AdviceMode = "db_only"
	AdviceModeGraphRAG AdviceMode = "graph_rag"
)

type AdviceRun struct {
	Id          int64              `db:"id" json:"id"`
	UserId      string             `db:"user_id" json:"userId"`
	Character   string             `db:"character" json:"character"`
	InputWindow int                `db:"input_window" json:"inputWindow"`
	SnapshotAt  string             `db:"snapshot_at" json:"snapshotAt"`
	CreatedAt   string             `db:"created_at" json:"createdAt"`
	Candidates  []*AdviceCandidate `db:"-" json:"candidates"`
}

type AdviceCandidate struct {
	Id              int64            `db:"id" json:"id"`
	RunId           int64            `db:"run_id" json:"runId"`
	Mode            AdviceMode       `db:"mode" json:"mode"`
	Priority        string           `db:"priority" json:"priority"`
	Theme           string           `db:"theme" json:"theme"`
	Summary         string           `db:"summary" json:"summary"`
	Rationale       string           `db:"rationale" json:"rationale"`
	Action          string           `db:"action" json:"action"`
	Drill           string           `db:"drill" json:"drill"`
	SuccessCriteria string           `db:"success_criteria" json:"successCriteria"`
	WatchMetrics    string           `db:"watch_metrics" json:"watchMetrics"`
	Risks           string           `db:"risks" json:"risks"`
	EvidenceJSON    string           `db:"evidence_json" json:"-"`
	Evidence        []AdviceEvidence `db:"-" json:"evidence"`
	CreatedAt       string           `db:"created_at" json:"createdAt"`
}

type AdviceEvidence struct {
	Source string  `json:"source"`
	Title  string  `json:"title"`
	Text   string  `json:"text"`
	Score  float64 `json:"score,omitempty"`
}

type AdviceFeedback struct {
	Id          int64      `db:"id" json:"id"`
	RunId       int64      `db:"run_id" json:"runId"`
	Mode        AdviceMode `db:"mode" json:"mode"`
	Rating      int        `db:"rating" json:"rating"`
	Specificity int        `db:"specificity" json:"specificity"`
	Usefulness  int        `db:"usefulness" json:"usefulness"`
	Trust       int        `db:"trust" json:"trust"`
	Comment     string     `db:"comment" json:"comment"`
	CreatedAt   string     `db:"created_at" json:"createdAt"`
}
