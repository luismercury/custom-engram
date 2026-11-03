package store

import "time"

type Project struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	RootPath  string    `json:"root_path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Session struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	Goal      string     `json:"goal,omitempty"`
	Summary   string     `json:"summary,omitempty"`
	Status    string     `json:"status"` // active, completed
	CreatedAt time.Time  `json:"created_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type Observation struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	SessionID *string   `json:"session_id,omitempty"`
	Type      string    `json:"type"`      // bugfix, decision, architecture, discovery, pattern, config, preference
	Scope     string    `json:"scope"`     // project, personal, global
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	TopicKey  string    `json:"topic_key,omitempty"`
	IsPinned   bool       `json:"is_pinned"`
	IsArchived bool       `json:"is_archived"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type PruneOptions struct {
	ProjectID  string        `json:"project_id,omitempty"`
	OlderThan  time.Duration `json:"older_than"`
	Types      []string      `json:"types,omitempty"`
	IncludeAll bool          `json:"include_all"`
	HardDelete bool          `json:"hard_delete"`
	DryRun     bool          `json:"dry_run"`
}

type PruneResult struct {
	MatchedCount  int           `json:"matched_count"`
	AffectedCount int           `json:"affected_count"`
	Action        string        `json:"action"` // "archived" or "deleted"
	DryRun        bool          `json:"dry_run"`
	Observations  []Observation `json:"observations,omitempty"`
}

type SearchResult struct {
	Observation Observation `json:"observation"`
	Score       float64     `json:"score"`
	Source      string      `json:"source"` // "fts5", "vector", "hybrid"
}

type Prompt struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id"`
	SessionID *string   `json:"session_id,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
