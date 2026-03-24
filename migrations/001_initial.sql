-- Create sessions table
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create messages table
CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    sequence INTEGER NOT NULL
);

-- Index for faster retrieval of messages by session
CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);

-- Index for ordering messages within a session
CREATE INDEX IF NOT EXISTS idx_messages_session_sequence ON messages(session_id, sequence);