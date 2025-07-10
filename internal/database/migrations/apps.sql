CREATE TABLE IF NOT EXISTS apps (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    language TEXT,
    port INTEGER NOT NULL,
    container_id TEXT,
    image_id TEXT,
    status TEXT DEFAULT 'idle',
    error_msg TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);