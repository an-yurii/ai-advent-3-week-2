package postgres

import (
	"context"
	"fmt"
	"time"

	"ai-agent-gigachat/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStorage implements storage.Storage using PostgreSQL.
type PostgresStorage struct {
	pool *pgxpool.Pool
}

// New creates a new PostgresStorage and ensures the database schema is up to date.
func New(ctx context.Context, connString string) (*PostgresStorage, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Ping to verify connection
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	storage := &PostgresStorage{pool: pool}

	// Run migrations
	if err := storage.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return storage, nil
}

// migrate runs the necessary SQL migrations.
func (s *PostgresStorage) migrate(ctx context.Context) error {
	// Execute schema creation directly using the pool.
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS sessions (
			id UUID PRIMARY KEY,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS messages (
			id BIGSERIAL PRIMARY KEY,
			session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			role VARCHAR(20) NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			sequence INTEGER NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
		CREATE INDEX IF NOT EXISTS idx_messages_session_sequence ON messages(session_id, sequence);
	`)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID, including its message history.
func (s *PostgresStorage) GetSession(id string) (*storage.Session, error) {
	ctx := context.Background()

	// First, check if session exists and get its metadata
	var createdAt, updatedAt time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT created_at, updated_at FROM sessions WHERE id = $1`, id).
		Scan(&createdAt, &updatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	// Fetch messages ordered by sequence
	rows, err := s.pool.Query(ctx,
		`SELECT role, content, created_at, sequence FROM messages
		 WHERE session_id = $1 ORDER BY sequence`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var history []storage.Message
	for rows.Next() {
		var role, content string
		var msgTime time.Time
		var seq int
		if err := rows.Scan(&role, &content, &msgTime, &seq); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		history = append(history, storage.Message{Role: role, Content: content})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	return &storage.Session{
		ID:      id,
		History: history,
	}, nil
}

// CreateSession creates a new empty session with the given ID.
func (s *PostgresStorage) CreateSession(id string) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (id) VALUES ($1) ON CONFLICT (id) DO NOTHING`, id)
	if err != nil {
		return fmt.Errorf("failed to insert session: %w", err)
	}
	return nil
}

// AddMessage adds a message to the session's history.
func (s *PostgresStorage) AddMessage(sessionID string, msg storage.Message) error {
	ctx := context.Background()

	// Start a transaction to ensure consistency
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check that session exists
	var exists bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM sessions WHERE id = $1)`, sessionID).
		Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("session %s does not exist", sessionID)
	}

	// Get the next sequence number for this session
	var seq int
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(sequence), 0) + 1 FROM messages WHERE session_id = $1`, sessionID).
		Scan(&seq)
	if err != nil {
		return fmt.Errorf("failed to get next sequence: %w", err)
	}

	// Insert the message
	_, err = tx.Exec(ctx,
		`INSERT INTO messages (session_id, role, content, sequence) VALUES ($1, $2, $3, $4)`,
		sessionID, msg.Role, msg.Content, seq)
	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	// Update session's updated_at timestamp
	_, err = tx.Exec(ctx,
		`UPDATE sessions SET updated_at = NOW() WHERE id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session timestamp: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// DeleteSession deletes a session and all its messages.
func (s *PostgresStorage) DeleteSession(id string) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// ListSessions returns a list of all session IDs, ordered by creation time (newest first).
func (s *PostgresStorage) ListSessions() ([]string, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM sessions ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan session id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}
	return ids, nil
}

// Close releases the connection pool.
func (s *PostgresStorage) Close() error {
	s.pool.Close()
	return nil
}

// Ensure PostgresStorage implements storage.Storage
var _ storage.Storage = (*PostgresStorage)(nil)