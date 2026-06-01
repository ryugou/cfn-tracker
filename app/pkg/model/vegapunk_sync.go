package model

type VegapunkSyncJob struct {
	Id            int64  `db:"id" json:"id"`
	Kind          string `db:"kind" json:"kind"`
	DedupeKey     string `db:"dedupe_key" json:"dedupeKey"`
	PayloadJSON   string `db:"payload_json" json:"payloadJson"`
	Attempts      int    `db:"attempts" json:"attempts"`
	LastError     string `db:"last_error" json:"lastError"`
	NextAttemptAt string `db:"next_attempt_at" json:"nextAttemptAt"`
	ProcessedAt   string `db:"processed_at" json:"processedAt"`
	CreatedAt     string `db:"created_at" json:"createdAt"`
	UpdatedAt     string `db:"updated_at" json:"updatedAt"`
}
