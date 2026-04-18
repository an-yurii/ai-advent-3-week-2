package knowledge

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Chunk represents a text chunk stored in the knowledge base.
type Chunk struct {
	ID          string
	DocumentID  int64
	Text        string
	Section     string
	FilePath    string
	ChunkIndex  int
	StartOffset int
	EndOffset   int
	TokenCount  int
}

// SQLiteRepository provides access to the SQLite knowledge base.
type SQLiteRepository interface {
	// GetChunksByIDs retrieves chunks by their IDs.
	GetChunksByIDs(ids []string) ([]Chunk, error)
	// GetChunkByID retrieves a single chunk by its ID.
	GetChunkByID(id string) (*Chunk, error)
	// Close closes the database connection.
	Close() error
}

// sqliteRepository implements SQLiteRepository.
type sqliteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository opens a SQLite database at the given path and returns a repository.
func NewSQLiteRepository(path string) (SQLiteRepository, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database at %s: %w", path, err)
	}

	// Verify the connection and check schema
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping SQLite database: %w", err)
	}

	// Check that required tables exist
	requiredTables := []string{"documents", "chunks", "embeddings"}
	for _, table := range requiredTables {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='%s'", table)
		if err := db.QueryRow(query).Scan(&count); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to check for table %s: %w", table, err)
		}
		if count == 0 {
			db.Close()
			return nil, fmt.Errorf("required table %s does not exist in database", table)
		}
	}

	return &sqliteRepository{db: db}, nil
}

// GetChunksByIDs retrieves chunks by their IDs.
func (r *sqliteRepository) GetChunksByIDs(ids []string) ([]Chunk, error) {
	if len(ids) == 0 {
		return []Chunk{}, nil
	}

	// Build placeholders for the IN clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT 
			c.id, c.document_id, c.text, c.section, c.chunk_index,
			c.start_offset, c.end_offset, c.token_count,
			d.file_path
		FROM chunks c
		JOIN documents d ON c.document_id = d.id
		WHERE c.id IN (%s)
		ORDER BY c.chunk_index
	`, strings.Join(placeholders, ","))

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var chunk Chunk
		err := rows.Scan(
			&chunk.ID,
			&chunk.DocumentID,
			&chunk.Text,
			&chunk.Section,
			&chunk.ChunkIndex,
			&chunk.StartOffset,
			&chunk.EndOffset,
			&chunk.TokenCount,
			&chunk.FilePath,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk row: %w", err)
		}
		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chunk rows: %w", err)
	}

	return chunks, nil
}

// GetChunkByID retrieves a single chunk by its ID.
func (r *sqliteRepository) GetChunkByID(id string) (*Chunk, error) {
	query := `
		SELECT 
			c.id, c.document_id, c.text, c.section, c.chunk_index,
			c.start_offset, c.end_offset, c.token_count,
			d.file_path
		FROM chunks c
		JOIN documents d ON c.document_id = d.id
		WHERE c.id = ?
	`

	var chunk Chunk
	err := r.db.QueryRow(query, id).Scan(
		&chunk.ID,
		&chunk.DocumentID,
		&chunk.Text,
		&chunk.Section,
		&chunk.ChunkIndex,
		&chunk.StartOffset,
		&chunk.EndOffset,
		&chunk.TokenCount,
		&chunk.FilePath,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query chunk by ID: %w", err)
	}

	return &chunk, nil
}

// Close closes the database connection.
func (r *sqliteRepository) Close() error {
	return r.db.Close()
}
