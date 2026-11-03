package store

const SchemaDDL = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS projects (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    root_path TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    goal TEXT,
    summary TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME NOT NULL,
    ended_at DATETIME,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS observations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT,
    type TEXT NOT NULL DEFAULT 'discovery',
    scope TEXT NOT NULL DEFAULT 'project',
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    topic_key TEXT,
    is_pinned INTEGER NOT NULL DEFAULT 0,
    is_archived INTEGER NOT NULL DEFAULT 0,
    archived_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_obs_project ON observations(project_id);
CREATE INDEX IF NOT EXISTS idx_obs_topic ON observations(topic_key);
CREATE INDEX IF NOT EXISTS idx_obs_type ON observations(type);
CREATE INDEX IF NOT EXISTS idx_obs_created ON observations(created_at);

CREATE VIRTUAL TABLE IF NOT EXISTS observations_fts USING fts5(
    id UNINDEXED,
    project_id UNINDEXED,
    title,
    content,
    topic_key,
    type,
    scope,
    tokenize = 'porter unicode61'
);

CREATE TRIGGER IF NOT EXISTS trig_obs_ai AFTER INSERT ON observations BEGIN
    INSERT INTO observations_fts (id, project_id, title, content, topic_key, type, scope)
    VALUES (new.id, new.project_id, new.title, new.content, new.topic_key, new.type, new.scope);
END;

CREATE TRIGGER IF NOT EXISTS trig_obs_ad AFTER DELETE ON observations BEGIN
    DELETE FROM observations_fts WHERE id = old.id;
END;

CREATE TRIGGER IF NOT EXISTS trig_obs_au AFTER UPDATE ON observations BEGIN
    UPDATE observations_fts SET 
        title = new.title,
        content = new.content,
        topic_key = new.topic_key,
        type = new.type,
        scope = new.scope
    WHERE id = old.id;
END;

CREATE TABLE IF NOT EXISTS embeddings (
    observation_id TEXT PRIMARY KEY,
    model TEXT NOT NULL,
    dimensions INTEGER NOT NULL,
    vector BLOB NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (observation_id) REFERENCES observations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS prompts (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    session_id TEXT,
    content TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL
);
`
