package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

// New opens or creates the SQLite database and executes initial migrations.
func New(dbPath string) (*Store, error) {
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	dsn := dbPath
	if dbPath != ":memory:" {
		// WAL mode and busy timeout in DSN
		dsn = fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", dbPath)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Single writer / connection pooling settings for SQLite
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(SchemaDDL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema DDL: %w", err)
	}

	// Auto-migration for is_pinned column
	_, _ = db.Exec(`ALTER TABLE observations ADD COLUMN is_pinned INTEGER NOT NULL DEFAULT 0;`)
	_, _ = db.Exec(`ALTER TABLE observations ADD COLUMN is_archived INTEGER NOT NULL DEFAULT 0;`)
	_, _ = db.Exec(`ALTER TABLE observations ADD COLUMN archived_at DATETIME;`)
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_obs_archived ON observations(is_archived);`)

	return &Store{db: db}, nil
}

// ParseAge parses human age durations like "90d", "30d", "2w", "1y", "720h" into a time.Duration.
func ParseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, "d") {
		numStr := strings.TrimSuffix(lower, "d")
		days, err := strconv.Atoi(strings.TrimSpace(numStr))
		if err != nil {
			return 0, fmt.Errorf("invalid days format: %s", s)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	if strings.HasSuffix(lower, "w") {
		numStr := strings.TrimSuffix(lower, "w")
		weeks, err := strconv.Atoi(strings.TrimSpace(numStr))
		if err != nil {
			return 0, fmt.Errorf("invalid weeks format: %s", s)
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}

	if strings.HasSuffix(lower, "y") {
		numStr := strings.TrimSuffix(lower, "y")
		years, err := strconv.Atoi(strings.TrimSpace(numStr))
		if err != nil {
			return 0, fmt.Errorf("invalid years format: %s", s)
		}
		return time.Duration(years) * 365 * 24 * time.Hour, nil
	}

	return time.ParseDuration(s)
}
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced/custom queries.
func (s *Store) DB() *sql.DB {
	return s.db
}

// --- Project Operations ---

func (s *Store) GetOrCreateProject(name, rootPath string) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var p Project
	var createdAt, updatedAt string
	err := s.db.QueryRow(
		`SELECT id, name, COALESCE(root_path, ''), created_at, updated_at FROM projects WHERE name = ?`,
		name,
	).Scan(&p.ID, &p.Name, &p.RootPath, &createdAt, &updatedAt)

	if err == nil {
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		if rootPath != "" && p.RootPath != rootPath {
			now := time.Now().UTC()
			_, _ = s.db.Exec(`UPDATE projects SET root_path = ?, updated_at = ? WHERE id = ?`, rootPath, now.Format(time.RFC3339Nano), p.ID)
			p.RootPath = rootPath
			p.UpdatedAt = now
		}
		return &p, nil
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	// Create new project
	now := time.Now().UTC()
	p = Project{
		ID:        uuid.New().String(),
		Name:      name,
		RootPath:  rootPath,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = s.db.Exec(
		`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.RootPath, p.CreatedAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert project: %w", err)
	}

	return &p, nil
}

func (s *Store) GetProjectByName(name string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var p Project
	var createdAt, updatedAt string
	err := s.db.QueryRow(
		`SELECT id, name, COALESCE(root_path, ''), created_at, updated_at FROM projects WHERE name = ?`,
		name,
	).Scan(&p.ID, &p.Name, &p.RootPath, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &p, nil
}

func (s *Store) GetProjectByID(id string) (*Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var p Project
	var createdAt, updatedAt string
	err := s.db.QueryRow(
		`SELECT id, name, COALESCE(root_path, ''), created_at, updated_at FROM projects WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.Name, &p.RootPath, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &p, nil
}

func (s *Store) ListProjects() ([]Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT id, name, COALESCE(root_path, ''), created_at, updated_at FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.RootPath, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		p.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		projects = append(projects, p)
	}
	return projects, nil
}

// --- Session Operations ---

func (s *Store) StartSession(projectID, goal string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	sess := &Session{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		Goal:      goal,
		Status:    "active",
		CreatedAt: now,
	}

	_, err := s.db.Exec(
		`INSERT INTO sessions (id, project_id, goal, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		sess.ID, sess.ProjectID, sess.Goal, sess.Status, sess.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}
	return sess, nil
}

func (s *Store) EndSession(sessionID, summary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	res, err := s.db.Exec(
		`UPDATE sessions SET summary = ?, status = 'completed', ended_at = ? WHERE id = ?`,
		summary, now.Format(time.RFC3339Nano), sessionID,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session not found: %s", sessionID)
	}
	return nil
}

func (s *Store) GetRecentSessions(projectID string, limit int) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 5
	}

	query := `SELECT id, project_id, COALESCE(goal, ''), COALESCE(summary, ''), status, created_at, ended_at 
              FROM sessions WHERE project_id = ? ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.Query(query, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var sess Session
		var createdAt string
		var endedAt sql.NullString
		if err := rows.Scan(&sess.ID, &sess.ProjectID, &sess.Goal, &sess.Summary, &sess.Status, &createdAt, &endedAt); err != nil {
			return nil, err
		}
		sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		if endedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, endedAt.String)
			sess.EndedAt = &t
		}
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

func (s *Store) GetActiveSession(projectID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sess Session
	var createdAt string
	var endedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT id, project_id, COALESCE(goal, ''), COALESCE(summary, ''), status, created_at, ended_at 
         FROM sessions WHERE project_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 1`,
		projectID,
	).Scan(&sess.ID, &sess.ProjectID, &sess.Goal, &sess.Summary, &sess.Status, &createdAt, &endedAt)

	if err != nil {
		return nil, err
	}
	sess.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &sess, nil
}

// --- Observation Operations ---

func (s *Store) SaveObservation(obs *Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if obs.Type == "" {
		obs.Type = "discovery"
	}
	if obs.Scope == "" {
		obs.Scope = "project"
	}

	// Topic key update behavior: if topic_key is set, check if an existing observation with that key exists for this project
	if obs.TopicKey != "" && obs.ID == "" {
		var existingID string
		err := s.db.QueryRow(
			`SELECT id FROM observations WHERE project_id = ? AND topic_key = ? LIMIT 1`,
			obs.ProjectID, obs.TopicKey,
		).Scan(&existingID)
		if err == nil {
			obs.ID = existingID
		}
	}

	pinnedInt := 0
	if obs.IsPinned {
		pinnedInt = 1
	}
	archivedInt := 0
	var archivedAtStr *string
	if obs.IsArchived {
		archivedInt = 1
		if obs.ArchivedAt != nil {
			s := obs.ArchivedAt.Format(time.RFC3339Nano)
			archivedAtStr = &s
		}
	}

	if obs.ID == "" {
		obs.ID = uuid.New().String()
		obs.CreatedAt = now
		obs.UpdatedAt = now

		_, err := s.db.Exec(
			`INSERT INTO observations (id, project_id, session_id, type, scope, title, content, topic_key, is_pinned, is_archived, archived_at, created_at, updated_at) 
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			obs.ID, obs.ProjectID, obs.SessionID, obs.Type, obs.Scope, obs.Title, obs.Content, obs.TopicKey, pinnedInt,
			archivedInt, archivedAtStr, obs.CreatedAt.Format(time.RFC3339Nano), obs.UpdatedAt.Format(time.RFC3339Nano),
		)
		return err
	}

	// Update existing observation
	obs.UpdatedAt = now
	_, err := s.db.Exec(
		`UPDATE observations SET 
            type = ?, scope = ?, title = ?, content = ?, topic_key = ?, is_pinned = ?, is_archived = ?, archived_at = ?, session_id = COALESCE(?, session_id), updated_at = ?
         WHERE id = ?`,
		obs.Type, obs.Scope, obs.Title, obs.Content, obs.TopicKey, pinnedInt, archivedInt, archivedAtStr, obs.SessionID, obs.UpdatedAt.Format(time.RFC3339Nano), obs.ID,
	)
	return err
}

func (s *Store) GetObservation(id string) (*Observation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getObservationLocked(id)
}

func (s *Store) getObservationLocked(id string) (*Observation, error) {
	var obs Observation
	var createdAt, updatedAt string
	var sessionID, topicKey, archivedAt sql.NullString
	var isPinnedInt, isArchivedInt int

	err := s.db.QueryRow(
		`SELECT id, project_id, session_id, type, scope, title, content, topic_key, is_pinned, is_archived, archived_at, created_at, updated_at 
         FROM observations WHERE id = ?`,
		id,
	).Scan(&obs.ID, &obs.ProjectID, &sessionID, &obs.Type, &obs.Scope, &obs.Title, &obs.Content, &topicKey, &isPinnedInt, &isArchivedInt, &archivedAt, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	if sessionID.Valid {
		obs.SessionID = &sessionID.String
	}
	if topicKey.Valid {
		obs.TopicKey = topicKey.String
	}
	obs.IsPinned = (isPinnedInt == 1)
	obs.IsArchived = (isArchivedInt == 1)
	if archivedAt.Valid {
		t, _ := time.Parse(time.RFC3339Nano, archivedAt.String)
		obs.ArchivedAt = &t
	}
	obs.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	obs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return &obs, nil
}

func (s *Store) DeleteObservation(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM observations WHERE id = ?`, id)
	return err
}

func (s *Store) ListObservations(projectID string, limit int) ([]Observation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	query := `SELECT id, project_id, session_id, type, scope, title, content, COALESCE(topic_key, ''), is_pinned, is_archived, archived_at, created_at, updated_at 
              FROM observations WHERE (project_id = ? OR scope = 'global') AND is_archived = 0 ORDER BY is_pinned DESC, updated_at DESC LIMIT ?`
	rows, err := s.db.Query(query, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Observation
	for rows.Next() {
		var obs Observation
		var sessionID, archivedAt sql.NullString
		var createdAt, updatedAt string
		var isPinnedInt, isArchivedInt int
		if err := rows.Scan(&obs.ID, &obs.ProjectID, &sessionID, &obs.Type, &obs.Scope, &obs.Title, &obs.Content, &obs.TopicKey, &isPinnedInt, &isArchivedInt, &archivedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			obs.SessionID = &sessionID.String
		}
		obs.IsPinned = (isPinnedInt == 1)
		obs.IsArchived = (isArchivedInt == 1)
		if archivedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, archivedAt.String)
			obs.ArchivedAt = &t
		}
		obs.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		obs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		list = append(list, obs)
	}
	return list, nil
}

// --- FTS5 and Search ---

// SearchFTS5 performs full-text search using SQLite FTS5 table with BM25 ranking.
func (s *Store) SearchFTS5(projectID, query string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Sanitize FTS5 query terms (wrap individual tokens or remove special syntax errors)
	cleanQuery := sanitizeFTS5Query(query)
	if cleanQuery == "" {
		return nil, nil
	}

		sqlQuery := `
        SELECT o.id, o.project_id, o.session_id, o.type, o.scope, o.title, o.content, COALESCE(o.topic_key, ''), o.is_pinned,
               o.is_archived, o.archived_at, o.created_at, o.updated_at, bm25(observations_fts) AS rank
        FROM observations_fts fts
        JOIN observations o ON fts.id = o.id
        WHERE observations_fts MATCH ? AND (o.project_id = ? OR o.scope = 'global') AND o.is_archived = 0
        ORDER BY o.is_pinned DESC, rank ASC
        LIMIT ?
    `

	rows, err := s.db.Query(sqlQuery, cleanQuery, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("FTS5 query failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var obs Observation
		var sessionID, archivedAt sql.NullString
		var createdAt, updatedAt string
		var isPinnedInt, isArchivedInt int
		var rawRank float64

		if err := rows.Scan(&obs.ID, &obs.ProjectID, &sessionID, &obs.Type, &obs.Scope, &obs.Title, &obs.Content, &obs.TopicKey, &isPinnedInt, &isArchivedInt, &archivedAt, &createdAt, &updatedAt, &rawRank); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			obs.SessionID = &sessionID.String
		}
		obs.IsPinned = (isPinnedInt == 1)
		obs.IsArchived = (isArchivedInt == 1)
		if archivedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, archivedAt.String)
			obs.ArchivedAt = &t
		}
		obs.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		obs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

		// SQLite bm25 returns negative score where lower (more negative) is better match.
		// Normalize to positive relevance score: higher = better.
		normalizedScore := 1.0 / (1.0 + (-rawRank))
		if rawRank > 0 {
			normalizedScore = 1.0 / (1.0 + rawRank)
		}

		results = append(results, SearchResult{
			Observation: obs,
			Score:       normalizedScore,
			Source:      "fts5",
		})
	}

	return results, nil
}

// sanitizeFTS5Query prepares a user query for safe FTS5 MATCH execution.
func sanitizeFTS5Query(q string) string {
	terms := strings.Fields(q)
	if len(terms) == 0 {
		return ""
	}
	var clean []string
	for _, t := range terms {
		// Remove characters that trigger FTS5 syntax errors
		t = strings.Map(func(r rune) rune {
			if strings.ContainsRune(`"':*+-^(){}`, r) {
				return -1
			}
			return r
		}, t)
		if len(t) > 0 {
			clean = append(clean, `"`+t+`"*`)
		}
	}
	return strings.Join(clean, " OR ")
}

// --- Embeddings Storage ---

func (s *Store) SaveEmbedding(obsID, model string, vec []float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	blob := EncodeVector(vec)
	now := time.Now().UTC()

	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO embeddings (observation_id, model, dimensions, vector, created_at) 
         VALUES (?, ?, ?, ?, ?)`,
		obsID, model, len(vec), blob, now.Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) GetEmbeddingsForProject(projectID string) (map[string][]float32, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
        SELECT e.observation_id, e.vector 
        FROM embeddings e
        JOIN observations o ON e.observation_id = o.id
        WHERE (o.project_id = ? OR o.scope = 'global') AND o.is_archived = 0
    `
	rows, err := s.db.Query(query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string][]float32)
	for rows.Next() {
		var obsID string
		var blob []byte
		if err := rows.Scan(&obsID, &blob); err != nil {
			return nil, err
		}
		vec := DecodeVector(blob)
		if vec != nil {
			res[obsID] = vec
		}
	}
	return res, nil
}

func (s *Store) GetObservationsByIDs(ids []string) ([]Observation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT id, project_id, session_id, type, scope, title, content, COALESCE(topic_key, ''), is_pinned, is_archived, archived_at, created_at, updated_at 
         FROM observations WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Observation
	for rows.Next() {
		var obs Observation
		var sessionID, archivedAt sql.NullString
		var createdAt, updatedAt string
		var isPinnedInt, isArchivedInt int
		if err := rows.Scan(&obs.ID, &obs.ProjectID, &sessionID, &obs.Type, &obs.Scope, &obs.Title, &obs.Content, &obs.TopicKey, &isPinnedInt, &isArchivedInt, &archivedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			obs.SessionID = &sessionID.String
		}
		obs.IsPinned = (isPinnedInt == 1)
		obs.IsArchived = (isArchivedInt == 1)
		if archivedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, archivedAt.String)
			obs.ArchivedAt = &t
		}
		obs.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		obs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		list = append(list, obs)
	}
	return list, nil
}

func (s *Store) PinObservation(id string, pinned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	pinnedInt := 0
	if pinned {
		pinnedInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.Exec(`UPDATE observations SET is_pinned = ?, updated_at = ? WHERE id = ?`, pinnedInt, now, id)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("observation not found: %s", id)
	}
	return nil
}

func (s *Store) UpdateObservation(id string, title, content, obsType, scope, topicKey *string) (*Observation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	obs, err := s.getObservationLocked(id)
	if err != nil {
		return nil, err
	}

	if title != nil && *title != "" {
		obs.Title = *title
	}
	if content != nil && *content != "" {
		obs.Content = *content
	}
	if obsType != nil && *obsType != "" {
		obs.Type = *obsType
	}
	if scope != nil && *scope != "" {
		obs.Scope = *scope
	}
	if topicKey != nil {
		obs.TopicKey = *topicKey
	}

	now := time.Now().UTC()
	obs.UpdatedAt = now

	_, err = s.db.Exec(
		`UPDATE observations SET title = ?, content = ?, type = ?, scope = ?, topic_key = ?, updated_at = ? WHERE id = ?`,
		obs.Title, obs.Content, obs.Type, obs.Scope, obs.TopicKey, obs.UpdatedAt.Format(time.RFC3339Nano), obs.ID,
	)
	if err != nil {
		return nil, err
	}
	return obs, nil
}

func (s *Store) SavePrompt(projectID, sessionID, content string) (*Prompt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	p := &Prompt{
		ID:        uuid.New().String(),
		ProjectID: projectID,
		Content:   content,
		CreatedAt: now,
	}
	if sessionID != "" {
		p.SessionID = &sessionID
	}

	_, err := s.db.Exec(
		`INSERT INTO prompts (id, project_id, session_id, content, created_at) VALUES (?, ?, ?, ?, ?)`,
		p.ID, p.ProjectID, p.SessionID, p.Content, p.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Store) ArchiveObservation(id string, archived bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	archivedInt := 0
	var archivedAt sql.NullString
	if archived {
		archivedInt = 1
		archivedAt = sql.NullString{String: time.Now().UTC().Format(time.RFC3339Nano), Valid: true}
	}

	res, err := s.db.Exec(
		`UPDATE observations SET is_archived = ?, archived_at = ?, updated_at = ? WHERE id = ?`,
		archivedInt, archivedAt, time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("observation not found: %s", id)
	}
	return nil
}

func (s *Store) ListArchivedObservations(projectID string, limit int) ([]Observation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	query := `SELECT id, project_id, session_id, type, scope, title, content, COALESCE(topic_key, ''), is_pinned, is_archived, archived_at, created_at, updated_at 
              FROM observations WHERE (project_id = ? OR scope = 'global') AND is_archived = 1 ORDER BY updated_at DESC LIMIT ?`
	rows, err := s.db.Query(query, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Observation
	for rows.Next() {
		var obs Observation
		var sessionID, archivedAt sql.NullString
		var createdAt, updatedAt string
		var isPinnedInt, isArchivedInt int
		if err := rows.Scan(&obs.ID, &obs.ProjectID, &sessionID, &obs.Type, &obs.Scope, &obs.Title, &obs.Content, &obs.TopicKey, &isPinnedInt, &isArchivedInt, &archivedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			obs.SessionID = &sessionID.String
		}
		obs.IsPinned = (isPinnedInt == 1)
		obs.IsArchived = (isArchivedInt == 1)
		if archivedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, archivedAt.String)
			obs.ArchivedAt = &t
		}
		obs.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		obs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		list = append(list, obs)
	}
	return list, nil
}

func (s *Store) Prune(opts PruneOptions) (*PruneResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	olderThan := opts.OlderThan
	if olderThan <= 0 {
		olderThan = 90 * 24 * time.Hour
	}
	cutoff := time.Now().UTC().Add(-olderThan)

	var whereClauses []string
	var args []interface{}

	// Rule 1: NEVER prune pinned observations
	whereClauses = append(whereClauses, "is_pinned = 0")

	// Rule 2: Age condition based on updated_at
	whereClauses = append(whereClauses, "updated_at <= ?")
	args = append(args, cutoff.Format(time.RFC3339Nano))

	// Rule 3: Project filter if provided
	if opts.ProjectID != "" {
		whereClauses = append(whereClauses, "project_id = ?")
		args = append(args, opts.ProjectID)
	}

	// Rule 4: If soft-delete (archiving), target only active (is_archived = 0)
	if !opts.HardDelete {
		whereClauses = append(whereClauses, "is_archived = 0")
	}

	// Rule 5: Type filtering (protect architecture & decision by default)
	if !opts.IncludeAll {
		if len(opts.Types) > 0 {
			placeholders := make([]string, len(opts.Types))
			for i, t := range opts.Types {
				placeholders[i] = "?"
				args = append(args, strings.TrimSpace(t))
			}
			whereClauses = append(whereClauses, fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")))
		} else {
			whereClauses = append(whereClauses, "type NOT IN ('architecture', 'decision')")
		}
	}

	query := fmt.Sprintf(`
		SELECT id, project_id, session_id, type, scope, title, content, COALESCE(topic_key, ''), is_pinned, is_archived, archived_at, created_at, updated_at
		FROM observations
		WHERE %s
		ORDER BY updated_at ASC
	`, strings.Join(whereClauses, " AND "))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query observations for prune: %w", err)
	}
	defer rows.Close()

	var candidates []Observation
	for rows.Next() {
		var obs Observation
		var sessionID, archivedAt sql.NullString
		var createdAt, updatedAt string
		var isPinnedInt, isArchivedInt int

		if err := rows.Scan(&obs.ID, &obs.ProjectID, &sessionID, &obs.Type, &obs.Scope, &obs.Title, &obs.Content, &obs.TopicKey, &isPinnedInt, &isArchivedInt, &archivedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if sessionID.Valid {
			obs.SessionID = &sessionID.String
		}
		obs.IsPinned = (isPinnedInt == 1)
		obs.IsArchived = (isArchivedInt == 1)
		if archivedAt.Valid {
			t, _ := time.Parse(time.RFC3339Nano, archivedAt.String)
			obs.ArchivedAt = &t
		}
		obs.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		obs.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
		candidates = append(candidates, obs)
	}

	result := &PruneResult{
		MatchedCount:  len(candidates),
		AffectedCount: 0,
		Action:        "archived",
		DryRun:        opts.DryRun,
		Observations:  candidates,
	}

	if opts.HardDelete {
		result.Action = "deleted"
	}

	if opts.DryRun || len(candidates) == 0 {
		return result, nil
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	if opts.HardDelete {
		for _, obs := range candidates {
			if _, err := s.db.Exec(`DELETE FROM observations WHERE id = ?`, obs.ID); err != nil {
				return nil, fmt.Errorf("failed to delete observation %s: %w", obs.ID, err)
			}
		}
		result.AffectedCount = len(candidates)
	} else {
		for _, obs := range candidates {
			if _, err := s.db.Exec(`UPDATE observations SET is_archived = 1, archived_at = ?, updated_at = ? WHERE id = ?`, nowStr, nowStr, obs.ID); err != nil {
				return nil, fmt.Errorf("failed to archive observation %s: %w", obs.ID, err)
			}
		}
		result.AffectedCount = len(candidates)
	}

	return result, nil
}

func (s *Store) GetStats(projectID string) (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalObs, activeObs, archivedObs, pinnedObs, totalSessions, totalPrompts int
	if projectID != "" {
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE project_id = ?`, projectID).Scan(&totalObs)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE project_id = ? AND is_archived = 0`, projectID).Scan(&activeObs)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE project_id = ? AND is_archived = 1`, projectID).Scan(&archivedObs)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE project_id = ? AND is_pinned = 1`, projectID).Scan(&pinnedObs)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE project_id = ?`, projectID).Scan(&totalSessions)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM prompts WHERE project_id = ?`, projectID).Scan(&totalPrompts)
	} else {
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations`).Scan(&totalObs)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE is_archived = 0`).Scan(&activeObs)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE is_archived = 1`).Scan(&archivedObs)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM observations WHERE is_pinned = 1`).Scan(&pinnedObs)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&totalSessions)
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM prompts`).Scan(&totalPrompts)
	}

	var totalProjects int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&totalProjects)

	return map[string]any{
		"total_observations":    totalObs,
		"active_observations":   activeObs,
		"archived_observations": archivedObs,
		"pinned_observations":   pinnedObs,
		"total_sessions":        totalSessions,
		"total_prompts":         totalPrompts,
		"total_projects":        totalProjects,
	}, nil
}

