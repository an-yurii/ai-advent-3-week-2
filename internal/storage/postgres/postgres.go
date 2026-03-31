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
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			strategy TEXT DEFAULT 'summary',
			facts TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS messages (
			id BIGSERIAL PRIMARY KEY,
			session_id UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			role VARCHAR(20) NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			sequence INTEGER NOT NULL,
			prompt_tokens INTEGER DEFAULT 0,
			completion_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS profiles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL,
			style TEXT NOT NULL DEFAULT '',
			constraints TEXT NOT NULL DEFAULT '',
			context TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			is_default BOOLEAN DEFAULT FALSE
		);

		CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
		CREATE INDEX IF NOT EXISTS idx_messages_session_sequence ON messages(session_id, sequence);
		CREATE INDEX IF NOT EXISTS idx_profiles_name ON profiles(name);
		CREATE INDEX IF NOT EXISTS idx_profiles_created_at ON profiles(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_profiles_is_default ON profiles(is_default) WHERE is_default = TRUE;
	`)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Ensure columns exist (for existing databases)
	_, err = s.pool.Exec(ctx, `
		ALTER TABLE messages
		ADD COLUMN IF NOT EXISTS prompt_tokens INTEGER DEFAULT 0,
		ADD COLUMN IF NOT EXISTS completion_tokens INTEGER DEFAULT 0,
		ADD COLUMN IF NOT EXISTS total_tokens INTEGER DEFAULT 0;

		ALTER TABLE sessions
		ADD COLUMN IF NOT EXISTS strategy TEXT DEFAULT 'summary',
		ADD COLUMN IF NOT EXISTS facts TEXT DEFAULT '',
		ADD COLUMN IF NOT EXISTS profile_id UUID REFERENCES profiles(id) ON DELETE SET NULL;

		CREATE INDEX IF NOT EXISTS idx_sessions_profile_id ON sessions(profile_id);
	`)
	if err != nil {
		return fmt.Errorf("failed to add columns: %w", err)
	}

	// Insert default profile if none exists
	_, err = s.pool.Exec(ctx, `
		INSERT INTO profiles (id, name, style, constraints, context, is_default, created_at, updated_at)
		VALUES (
			'00000000-0000-0000-0000-000000000000',
			'Default',
			'Respond in a helpful, friendly, and professional manner.',
			'Be accurate, concise, and avoid harmful content.',
			'You are an AI assistant helping users with their questions.',
			TRUE,
			NOW(),
			NOW()
		) ON CONFLICT (id) DO NOTHING;
	`)
	if err != nil {
		return fmt.Errorf("failed to insert default profile: %w", err)
	}

	return nil
}

// GetSession retrieves a session by ID, including its message history.
func (s *PostgresStorage) GetSession(id string) (*storage.Session, error) {
	ctx := context.Background()

	// First, check if session exists and get its metadata
	var createdAt, updatedAt time.Time
	var strategy string
	var factsText string
	var profileID *string // Use pointer to handle NULL
	err := s.pool.QueryRow(ctx,
		`SELECT created_at, updated_at, strategy, facts, profile_id FROM sessions WHERE id = $1`, id).
		Scan(&createdAt, &updatedAt, &strategy, &factsText, &profileID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	// Fetch messages ordered by sequence
	rows, err := s.pool.Query(ctx,
		`SELECT role, content, created_at, sequence, prompt_tokens, completion_tokens, total_tokens FROM messages
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
		var promptTokens, completionTokens, totalTokens int
		if err := rows.Scan(&role, &content, &msgTime, &seq, &promptTokens, &completionTokens, &totalTokens); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		history = append(history, storage.Message{
			Role:             role,
			Content:          content,
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      totalTokens,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	// Convert profileID pointer to string (empty if nil)
	profileIDStr := ""
	if profileID != nil {
		profileIDStr = *profileID
	}

	return &storage.Session{
		ID:        id,
		History:   history,
		Strategy:  strategy,
		Facts:     factsText,
		ProfileID: profileIDStr,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
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
		`INSERT INTO messages (session_id, role, content, sequence, prompt_tokens, completion_tokens, total_tokens) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		sessionID, msg.Role, msg.Content, seq, msg.PromptTokens, msg.CompletionTokens, msg.TotalTokens)
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

// ReplaceHistory replaces the entire message history of a session with the given messages.
func (s *PostgresStorage) ReplaceHistory(sessionID string, messages []storage.Message) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Check that session exists
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sessions WHERE id = $1)`, sessionID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("session %s does not exist", sessionID)
	}

	// Delete all existing messages for this session
	_, err = tx.Exec(ctx, `DELETE FROM messages WHERE session_id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete old messages: %w", err)
	}

	// Insert new messages with sequential order
	for i, msg := range messages {
		seq := i + 1
		_, err = tx.Exec(ctx,
			`INSERT INTO messages (session_id, role, content, sequence, prompt_tokens, completion_tokens, total_tokens)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			sessionID, msg.Role, msg.Content, seq, msg.PromptTokens, msg.CompletionTokens, msg.TotalTokens)
		if err != nil {
			return fmt.Errorf("failed to insert message %d: %w", i, err)
		}
	}

	// Update session's updated_at timestamp
	_, err = tx.Exec(ctx, `UPDATE sessions SET updated_at = NOW() WHERE id = $1`, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session timestamp: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

// UpdateStrategy updates the context management strategy for a session.
func (s *PostgresStorage) UpdateStrategy(sessionID string, strategy string) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET strategy = $1, updated_at = NOW() WHERE id = $2`,
		strategy, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update strategy: %w", err)
	}
	return nil
}

// UpdateFacts updates the facts text for a session.
func (s *PostgresStorage) UpdateFacts(sessionID string, facts string) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET facts = $1, updated_at = NOW() WHERE id = $2`,
		facts, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update facts: %w", err)
	}
	return nil
}

// UpdateSessionProfile updates the profile associated with a session.
func (s *PostgresStorage) UpdateSessionProfile(sessionID string, profileID string) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET profile_id = $1, updated_at = NOW() WHERE id = $2`,
		profileID, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update session profile: %w", err)
	}
	return nil
}

// ListProfiles returns all profiles ordered by creation time.
func (s *PostgresStorage) ListProfiles() ([]storage.Profile, error) {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, style, constraints, context, created_at, updated_at, is_default
		 FROM profiles ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query profiles: %w", err)
	}
	defer rows.Close()

	var profiles []storage.Profile
	for rows.Next() {
		var p storage.Profile
		err := rows.Scan(&p.ID, &p.Name, &p.Style, &p.Constraints, &p.Context, &p.CreatedAt, &p.UpdatedAt, &p.IsDefault)
		if err != nil {
			return nil, fmt.Errorf("failed to scan profile: %w", err)
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// GetProfile retrieves a profile by ID.
func (s *PostgresStorage) GetProfile(id string) (*storage.Profile, error) {
	ctx := context.Background()
	var p storage.Profile
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, style, constraints, context, created_at, updated_at, is_default
		 FROM profiles WHERE id = $1`, id).
		Scan(&p.ID, &p.Name, &p.Style, &p.Constraints, &p.Context, &p.CreatedAt, &p.UpdatedAt, &p.IsDefault)
	if err == pgx.ErrNoRows {
		return nil, storage.ErrProfileNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}
	return &p, nil
}

// CreateProfile creates a new profile.
func (s *PostgresStorage) CreateProfile(profile storage.Profile) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO profiles (id, name, style, constraints, context, is_default, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`,
		profile.ID, profile.Name, profile.Style, profile.Constraints, profile.Context, profile.IsDefault)
	if err != nil {
		return fmt.Errorf("failed to create profile: %w", err)
	}
	return nil
}

// UpdateProfile updates an existing profile.
func (s *PostgresStorage) UpdateProfile(id string, profile storage.Profile) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`UPDATE profiles
		 SET name = $1, style = $2, constraints = $3, context = $4, is_default = $5, updated_at = NOW()
		 WHERE id = $6`,
		profile.Name, profile.Style, profile.Constraints, profile.Context, profile.IsDefault, id)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}
	return nil
}

// DeleteProfile deletes a profile.
func (s *PostgresStorage) DeleteProfile(id string) error {
	ctx := context.Background()

	// Check if profile is in use
	var count int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM sessions WHERE profile_id = $1`, id).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check profile usage: %w", err)
	}
	if count > 0 {
		return storage.ErrProfileInUse
	}

	// Delete the profile
	_, err = s.pool.Exec(ctx, `DELETE FROM profiles WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}
	return nil
}

// SetDefaultProfile sets a profile as default and unsets any other default.
func (s *PostgresStorage) SetDefaultProfile(id string) error {
	ctx := context.Background()

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Unset all defaults
	_, err = tx.Exec(ctx, `UPDATE profiles SET is_default = FALSE WHERE is_default = TRUE`)
	if err != nil {
		return fmt.Errorf("failed to unset defaults: %w", err)
	}

	// Set new default
	_, err = tx.Exec(ctx, `UPDATE profiles SET is_default = TRUE, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to set default: %w", err)
	}

	return tx.Commit(ctx)
}

// GetDefaultProfile returns the default profile, if any.
func (s *PostgresStorage) GetDefaultProfile() (*storage.Profile, error) {
	ctx := context.Background()
	var p storage.Profile
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, style, constraints, context, created_at, updated_at, is_default
		 FROM profiles WHERE is_default = TRUE LIMIT 1`).
		Scan(&p.ID, &p.Name, &p.Style, &p.Constraints, &p.Context, &p.CreatedAt, &p.UpdatedAt, &p.IsDefault)
	if err == pgx.ErrNoRows {
		return nil, nil // No default profile is okay
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get default profile: %w", err)
	}
	return &p, nil
}

// Close releases the connection pool.
func (s *PostgresStorage) Close() error {
	s.pool.Close()
	return nil
}

// Ensure PostgresStorage implements storage.Storage
var _ storage.Storage = (*PostgresStorage)(nil)
