// This is an example script for populating the knowledge base with documents.
// It demonstrates how to create a SQLite database with the required schema,
// chunk documents, generate embeddings using Ollama, and store them.
//
// Usage:
//  1. Ensure Ollama is running: `ollama serve` or via Docker
//  2. Pull the embedding model: `ollama pull nomic-embed-text`
//  3. Run: `go run scripts/populate_knowledge.go`
package main

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Configuration
	dbPath := "./knowledge.db"
	// ollamaHost := "http://localhost:11434" // Unused in this example
	embeddingModel := "nomic-embed-text"

	// Example documents to add to the knowledge base
	documents := []struct {
		filePath string
		content  string
		sections []string
	}{
		{
			filePath: "project/README.md",
			content: `# AI Agent with GigaChat Integration

This project provides a web interface for interacting with GigaChat API through a Go backend.

## Features
- Session-based conversation history
- Multiple context management strategies
- PostgreSQL storage for persistence
- Docker deployment`,
			sections: []string{"Project Overview", "Features"},
		},
		{
			filePath: "docs/architecture.md",
			content: `## System Architecture

The system consists of three main components:

1. **Frontend**: Web interface built with HTML/CSS/JavaScript
2. **Backend**: Go HTTP server with REST API
3. **Database**: PostgreSQL for conversation storage

### Communication Flow
User → HTTP Request → Go Server → GigaChat API → Response → User`,
			sections: []string{"Architecture", "Components", "Communication Flow"},
		},
		{
			filePath: "docs/api.md",
			content: `## API Endpoints

### POST /api/chat
Send a message to the AI agent.

Request body:
{
  "message": "Hello, how are you?",
  "session_id": "uuid-string"
}

Response:
{
  "response": "I'm doing well, thank you!",
  "session_id": "uuid-string",
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 5,
    "total_tokens": 15
  }
}`,
			sections: []string{"API Endpoints", "POST /api/chat"},
		},
	}

	// Create or open SQLite database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create tables if they don't exist
	if err := createTables(db); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}

	log.Printf("Database initialized at %s", dbPath)

	// Process each document
	for _, doc := range documents {
		log.Printf("Processing document: %s", doc.filePath)

		// Check if document already exists
		var existingID int64
		err := db.QueryRow("SELECT id FROM documents WHERE file_path = ?", doc.filePath).Scan(&existingID)
		if err == nil {
			log.Printf("Document already exists with ID %d, skipping", existingID)
			continue
		} else if err != sql.ErrNoRows {
			log.Printf("Error checking for existing document: %v", err)
		}

		// Insert document
		docID, err := insertDocument(db, doc.filePath, doc.content)
		if err != nil {
			log.Printf("Failed to insert document: %v", err)
			continue
		}

		// Chunk the document (simple splitting by paragraphs for demonstration)
		chunks := chunkDocument(doc.content, doc.sections)
		log.Printf("Created %d chunks for document %s", len(chunks), doc.filePath)

		// Insert chunks and generate embeddings
		for i, chunk := range chunks {
			chunkID := fmt.Sprintf("%s-%d", filepath.Base(doc.filePath), i)

			// Insert chunk
			if err := insertChunk(db, chunkID, docID, chunk.text, chunk.section, i, 0, len(chunk.text)); err != nil {
				log.Printf("Failed to insert chunk %d: %v", i, err)
				continue
			}

			// Generate embedding (in a real implementation, call Ollama API)
			// For this example, we'll create a dummy embedding
			embedding := generateDummyEmbedding(384) // nomic-embed-text has 384 dimensions

			// Store embedding
			if err := insertEmbedding(db, chunkID, embeddingModel, 384, embedding); err != nil {
				log.Printf("Failed to insert embedding for chunk %s: %v", chunkID, err)
			}
		}

		// Update document chunk count
		if _, err := db.Exec("UPDATE documents SET chunk_count = ? WHERE id = ?", len(chunks), docID); err != nil {
			log.Printf("Failed to update chunk count: %v", err)
		}

		log.Printf("Successfully added document %s with %d chunks", doc.filePath, len(chunks))
	}

	log.Println("Knowledge base population completed!")
	log.Println("Note: This example uses dummy embeddings. In production, you should:")
	log.Println("  1. Call Ollama API to generate real embeddings")
	log.Println("  2. Build a proper FAISS index")
	log.Println("  3. Use the vector_index.go implementation for similarity search")
}

// createTables creates the required tables in the SQLite database.
func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			file_path TEXT NOT NULL UNIQUE,
			file_hash TEXT NOT NULL,
			file_size INTEGER NOT NULL,
			modified_at TIMESTAMP NOT NULL,
			chunk_count INTEGER DEFAULT 0,
			indexed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			document_id INTEGER NOT NULL,
			text TEXT NOT NULL,
			section TEXT,
			chunk_index INTEGER NOT NULL,
			start_offset INTEGER NOT NULL,
			end_offset INTEGER NOT NULL,
			token_count INTEGER,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS embeddings (
			chunk_id TEXT PRIMARY KEY,
			model TEXT NOT NULL,
			dimension INTEGER NOT NULL,
			vector_data BLOB NOT NULL,
			generated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS index_metadata (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_file_hash ON documents(file_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_documents_file_path ON documents(file_path)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_document_id ON chunks(document_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_section ON chunks(section)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query %q: %w", query, err)
		}
	}
	return nil
}

// insertDocument inserts a document into the documents table.
func insertDocument(db *sql.DB, filePath, content string) (int64, error) {
	// Simple hash for demonstration
	hash := fmt.Sprintf("%x", len(content))
	fileSize := len(content)
	modifiedAt := time.Now()

	result, err := db.Exec(
		`INSERT INTO documents (file_path, file_hash, file_size, modified_at) 
		 VALUES (?, ?, ?, ?)`,
		filePath, hash, fileSize, modifiedAt,
	)
	if err != nil {
		return 0, err
	}

	return result.LastInsertId()
}

type chunk struct {
	text    string
	section string
}

// chunkDocument splits a document into chunks (simple paragraph-based splitting).
func chunkDocument(content string, sections []string) []chunk {
	paragraphs := strings.Split(content, "\n\n")
	var chunks []chunk

	sectionIndex := 0
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// Determine section (simple heuristic: if paragraph starts with #, it's a new section)
		section := ""
		if sectionIndex < len(sections) {
			section = sections[sectionIndex]
		}
		if strings.HasPrefix(p, "#") || strings.HasPrefix(p, "##") {
			sectionIndex++
			if sectionIndex < len(sections) {
				section = sections[sectionIndex]
			}
		}

		chunks = append(chunks, chunk{
			text:    p,
			section: section,
		})
	}

	return chunks
}

// insertChunk inserts a chunk into the chunks table.
func insertChunk(db *sql.DB, chunkID string, docID int64, text, section string, index, start, end int) error {
	_, err := db.Exec(
		`INSERT INTO chunks (id, document_id, text, section, chunk_index, start_offset, end_offset) 
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		chunkID, docID, text, section, index, start, end,
	)
	return err
}

// generateDummyEmbedding creates a dummy embedding vector for demonstration.
// In production, replace this with a call to Ollama's embedding API.
func generateDummyEmbedding(dim int) []float32 {
	vec := make([]float32, dim)
	for i := range vec {
		vec[i] = float32(i) / float32(dim) // Simple pattern
	}
	return vec
}

// insertEmbedding stores an embedding vector in the embeddings table.
func insertEmbedding(db *sql.DB, chunkID, model string, dim int, vector []float32) error {
	// Convert float32 slice to byte array (little-endian)
	data := make([]byte, len(vector)*4)
	for i, v := range vector {
		binary.LittleEndian.PutUint32(data[i*4:(i+1)*4], math.Float32bits(v))
	}

	_, err := db.Exec(
		`INSERT INTO embeddings (chunk_id, model, dimension, vector_data) 
		 VALUES (?, ?, ?, ?)`,
		chunkID, model, dim, data,
	)
	return err
}
