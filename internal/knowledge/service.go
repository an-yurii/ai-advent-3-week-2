package knowledge

import (
	"fmt"
	"sort"
	"strings"
)

// SearchResult represents a chunk found by the knowledge base search.
type SearchResult struct {
	Chunk      Chunk
	Similarity float32 // cosine similarity, higher is more similar
	Distance   float32 // 1 - similarity, for compatibility with FAISS distance
}

// KnowledgeService orchestrates embedding generation, vector search, and chunk retrieval.
type KnowledgeService interface {
	// Search performs a knowledge base search for the given query.
	Search(query string) ([]SearchResult, error)
	// FormatContext formats the search results into the required context string.
	FormatContext(query string, results []SearchResult) string
	// Close releases any resources.
	Close() error
}

// knowledgeService implements KnowledgeService.
type knowledgeService struct {
	config *Config
	ollama OllamaClient
	index  VectorIndex
	repo   SQLiteRepository
}

// NewKnowledgeService creates a new knowledge service with the given configuration.
func NewKnowledgeService(cfg Config) (KnowledgeService, error) {
	if !cfg.Enabled {
		return &disabledService{}, nil
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid knowledge base configuration: %w", err)
	}

	// Initialize Ollama client
	ollama := NewOllamaClient(cfg.OllamaHost, cfg.OllamaModel)

	// Initialize vector index (loads embeddings from SQLite)
	index, err := NewVectorIndex(cfg.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create vector index: %w", err)
	}

	// Initialize SQLite repository
	repo, err := NewSQLiteRepository(cfg.SQLitePath)
	if err != nil {
		index.Close()
		return nil, fmt.Errorf("failed to create SQLite repository: %w", err)
	}

	return &knowledgeService{
		config: &cfg,
		ollama: ollama,
		index:  index,
		repo:   repo,
	}, nil
}

// Search performs a knowledge base search for the given query.
func (s *knowledgeService) Search(query string) ([]SearchResult, error) {
	// 1. Get embedding for the query
	embedding, err := s.ollama.GetEmbedding(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get query embedding: %w", err)
	}

	// 2. Search for similar vectors
	chunkIDs, similarities, err := s.index.Search(embedding, s.config.K)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 3. Retrieve chunks from database
	chunks, err := s.repo.GetChunksByIDs(chunkIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve chunks: %w", err)
	}

	// 4. Build results, preserving order from search
	results := make([]SearchResult, 0, len(chunks))
	chunkMap := make(map[string]Chunk)
	for _, chunk := range chunks {
		chunkMap[chunk.ID] = chunk
	}

	for i, id := range chunkIDs {
		chunk, ok := chunkMap[id]
		if !ok {
			continue // chunk not found in database (should not happen)
		}

		similarity := similarities[i]
		distance := 1 - similarity // convert similarity to distance

		// Filter by relevance threshold (distance-based)
		if distance > float32(s.config.RelevanceThreshold) {
			continue
		}

		results = append(results, SearchResult{
			Chunk:      chunk,
			Similarity: similarity,
			Distance:   distance,
		})
	}

	// 5. Sort by distance (ascending) / similarity (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Distance < results[j].Distance
	})

	// 6. Limit to MaxChunks
	if len(results) > s.config.MaxChunks {
		results = results[:s.config.MaxChunks]
	}

	return results, nil
}

// FormatContext formats the search results into the required context string.
func (s *knowledgeService) FormatContext(query string, results []SearchResult) string {
	if len(results) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Вопрос: ")
	builder.WriteString(query)
	builder.WriteString("\n\nКонтекст:\n")

	for i, result := range results {
		chunk := result.Chunk
		builder.WriteString(chunk.Text)
		builder.WriteString("\n")
		builder.WriteString("file_path: ")
		builder.WriteString(chunk.FilePath)
		builder.WriteString("\n")
		if chunk.Section != "" {
			builder.WriteString("section: ")
			builder.WriteString(chunk.Section)
			builder.WriteString("\n")
		}

		if i < len(results)-1 {
			builder.WriteString("---\n")
		}
	}

	return builder.String()
}

// Close releases resources.
func (s *knowledgeService) Close() error {
	var errs []error

	if s.index != nil {
		if err := s.index.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.repo != nil {
		if err := s.repo.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing knowledge service: %v", errs)
	}
	return nil
}

// disabledService is a no‑op implementation used when the knowledge base is disabled.
type disabledService struct{}

func (s *disabledService) Search(query string) ([]SearchResult, error) {
	return nil, nil
}

func (s *disabledService) FormatContext(query string, results []SearchResult) string {
	return ""
}

func (s *disabledService) Close() error {
	return nil
}
