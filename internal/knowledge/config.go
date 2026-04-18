package knowledge

import (
	"os"
	"strconv"
	"strings"
)

// Config holds configuration for the knowledge base search feature.
type Config struct {
	// Enabled toggles the knowledge base feature.
	Enabled bool

	// SQLitePath is the file path to the SQLite database.
	SQLitePath string

	// FAISSPath is the file path to the FAISS index.
	FAISSPath string

	// OllamaHost is the base URL of the Ollama service.
	OllamaHost string

	// OllamaModel is the embedding model to use (e.g., "nomic-embed-text").
	OllamaModel string

	// K is the number of nearest neighbors to retrieve from FAISS.
	K int

	// RelevanceThreshold is the maximum distance (0.0 = identical, higher = less similar)
	// for a chunk to be considered relevant. Chunks with distance > threshold are filtered out.
	RelevanceThreshold float64

	// MaxChunks is the maximum number of chunks to include in the context.
	MaxChunks int
}

// DefaultConfig returns a configuration with default values.
func DefaultConfig() Config {
	return Config{
		Enabled:            false,
		SQLitePath:         "./knowledge.db",
		FAISSPath:          "./faiss.index",
		OllamaHost:         "http://localhost:11434",
		OllamaModel:        "nomic-embed-text",
		K:                  5,
		RelevanceThreshold: 0.8,
		MaxChunks:          3,
	}
}

// LoadConfig loads configuration from environment variables.
// Values not set in the environment keep their defaults.
func LoadConfig() Config {
	cfg := DefaultConfig()

	if s := os.Getenv("KNOWLEDGE_BASE_ENABLED"); s != "" {
		if v, err := strconv.ParseBool(s); err == nil {
			cfg.Enabled = v
		}
	}

	if s := os.Getenv("KNOWLEDGE_BASE_SQLITE_PATH"); s != "" {
		cfg.SQLitePath = strings.TrimSpace(s)
	}

	if s := os.Getenv("KNOWLEDGE_BASE_FAISS_PATH"); s != "" {
		cfg.FAISSPath = strings.TrimSpace(s)
	}

	if s := os.Getenv("OLLAMA_HOST"); s != "" {
		cfg.OllamaHost = strings.TrimSpace(s)
	}

	if s := os.Getenv("OLLAMA_EMBEDDING_MODEL"); s != "" {
		cfg.OllamaModel = strings.TrimSpace(s)
	}

	if s := os.Getenv("KNOWLEDGE_BASE_K"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			cfg.K = v
		}
	}

	if s := os.Getenv("KNOWLEDGE_BASE_RELEVANCE_THRESHOLD"); s != "" {
		if v, err := strconv.ParseFloat(s, 32); err == nil && v >= 0 {
			cfg.RelevanceThreshold = v
		}
	}

	if s := os.Getenv("KNOWLEDGE_BASE_MAX_CHUNKS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			cfg.MaxChunks = v
		}
	}

	return cfg
}

// Validate checks the configuration for logical errors.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // nothing to validate if disabled
	}

	if c.SQLitePath == "" {
		return ErrConfigValidation("KNOWLEDGE_BASE_SQLITE_PATH must be set")
	}

	if c.FAISSPath == "" {
		return ErrConfigValidation("KNOWLEDGE_BASE_FAISS_PATH must be set")
	}

	if c.OllamaHost == "" {
		return ErrConfigValidation("OLLAMA_HOST must be set")
	}

	if c.OllamaModel == "" {
		return ErrConfigValidation("OLLAMA_EMBEDDING_MODEL must be set")
	}

	if c.K <= 0 {
		return ErrConfigValidation("KNOWLEDGE_BASE_K must be positive")
	}

	if c.RelevanceThreshold < 0 {
		return ErrConfigValidation("KNOWLEDGE_BASE_RELEVANCE_THRESHOLD must be non‑negative")
	}

	if c.MaxChunks <= 0 {
		return ErrConfigValidation("KNOWLEDGE_BASE_MAX_CHUNKS must be positive")
	}

	return nil
}

// ErrConfigValidation is returned when a configuration value is invalid.
type ErrConfigValidation string

func (e ErrConfigValidation) Error() string {
	return string(e)
}
