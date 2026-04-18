package knowledge

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
)

// VectorIndex provides similarity search over embedding vectors.
type VectorIndex interface {
	// Search returns the k nearest neighbor IDs and their distances for the query vector.
	Search(query []float32, k int) ([]string, []float32, error)
	// Close releases any resources.
	Close() error
}

// vectorIndex implements VectorIndex using an in‑memory cosine similarity search.
// It loads all embeddings from the SQLite database into memory.
type vectorIndex struct {
	vectors map[string][]float32 // chunk ID → embedding vector
	dim     int
}

// NewVectorIndex creates a new in‑memory vector index by loading embeddings from SQLite.
func NewVectorIndex(dbPath string) (VectorIndex, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %w", err)
	}
	defer db.Close()

	// Query embeddings table
	rows, err := db.Query(`
		SELECT chunk_id, model, dimension, vector_data
		FROM embeddings
		WHERE model = 'nomic-embed-text' OR model LIKE '%embed%'
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query embeddings: %w", err)
	}
	defer rows.Close()

	vectors := make(map[string][]float32)
	var dimension int

	for rows.Next() {
		var chunkID, model string
		var dim int
		var vectorData []byte

		if err := rows.Scan(&chunkID, &model, &dim, &vectorData); err != nil {
			return nil, fmt.Errorf("failed to scan embedding row: %w", err)
		}

		// Parse vector data (assuming it's stored as little‑endian float32s)
		if len(vectorData)%4 != 0 {
			return nil, fmt.Errorf("vector data length %d is not a multiple of 4 for chunk %s", len(vectorData), chunkID)
		}

		vector := make([]float32, len(vectorData)/4)
		for i := 0; i < len(vector); i++ {
			bits := binary.LittleEndian.Uint32(vectorData[i*4 : (i+1)*4])
			vector[i] = math.Float32frombits(bits)
		}

		if dimension == 0 {
			dimension = dim
		} else if dimension != dim {
			return nil, fmt.Errorf("inconsistent embedding dimension: expected %d, got %d for chunk %s", dimension, dim, chunkID)
		}

		vectors[chunkID] = vector
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating embedding rows: %w", err)
	}

	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embeddings found in database")
	}

	return &vectorIndex{
		vectors: vectors,
		dim:     dimension,
	}, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns a value in [‑1, 1], where 1 means identical direction.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / float32(math.Sqrt(float64(normA)*float64(normB)))
}

// Search returns the k nearest neighbor IDs and their cosine similarities.
// Higher similarity means more similar (1.0 = identical).
func (idx *vectorIndex) Search(query []float32, k int) ([]string, []float32, error) {
	if len(query) != idx.dim {
		return nil, nil, fmt.Errorf("query dimension %d does not match index dimension %d", len(query), idx.dim)
	}

	type scoredChunk struct {
		id         string
		similarity float32
	}

	scored := make([]scoredChunk, 0, len(idx.vectors))
	for id, vec := range idx.vectors {
		sim := cosineSimilarity(query, vec)
		scored = append(scored, scoredChunk{id: id, similarity: sim})
	}

	// Sort by similarity descending (most similar first)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].similarity > scored[j].similarity
	})

	// Take top k
	if k > len(scored) {
		k = len(scored)
	}

	ids := make([]string, k)
	similarities := make([]float32, k)
	for i := 0; i < k; i++ {
		ids[i] = scored[i].id
		similarities[i] = scored[i].similarity
	}

	return ids, similarities, nil
}

// Close releases resources (nothing to do for in‑memory index).
func (idx *vectorIndex) Close() error {
	idx.vectors = nil
	return nil
}
