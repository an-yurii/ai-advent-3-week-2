package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"

	"ai-agent-gigachat/internal/agent"
	"ai-agent-gigachat/internal/storage"
	"ai-agent-gigachat/internal/storage/memory"
	"ai-agent-gigachat/internal/storage/postgres"
)

var aiAgent *agent.Agent
var store storage.Storage

func createStorage() storage.Storage {
	// Read PostgreSQL environment variables
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "postgres"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}
	password := os.Getenv("DB_PASSWORD")
	if password == "" {
		password = "postgres"
	}
	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "ai_agent"
	}

	// Build connection string
	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, password, host, port, dbname)

	ctx := context.Background()
	pgStore, err := postgres.New(ctx, connString)
	if err != nil {
		log.Printf("Failed to connect to PostgreSQL: %v. Falling back to in-memory storage.", err)
		return memory.New()
	}
	log.Println("Using PostgreSQL storage")
	return pgStore
}

func main() {
	// Initialize storage (PostgreSQL with fallback to memory)
	store = createStorage()
	// API key from environment (required for GigaChat)
	apiKey := os.Getenv("GIGACHAT_API_KEY")
	if apiKey == "" {
		log.Fatal("GIGACHAT_API_KEY environment variable is required")
	}
	aiAgent = agent.NewAgent(apiKey, store)

	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// API routes
	http.HandleFunc("/api/chat", handleChat)
	http.HandleFunc("/api/sessions", handleSessions)
	http.HandleFunc("/api/sessions/", handleSession)

	port := ":8080"
	log.Printf("Server starting on %s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

// handleChat processes POST /api/chat
func handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message   string `json:"message"`
		SessionID string `json:"session_id"`
		Strategy  string `json:"strategy,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Message == "" || req.SessionID == "" {
		http.Error(w, "Missing 'message' or 'session_id'", http.StatusBadRequest)
		return
	}

	// Update session strategy if provided and valid
	if req.Strategy != "" && (req.Strategy == storage.StrategySummary || req.Strategy == storage.StrategySlidingWindow || req.Strategy == storage.StrategyStickyFacts) {
		// Ensure session exists (idempotent)
		if err := store.CreateSession(req.SessionID); err != nil {
			log.Printf("Failed to create session for strategy update: %v", err)
			// Continue anyway, SendMessage will also try to create
		} else {
			if err := store.UpdateStrategy(req.SessionID, req.Strategy); err != nil {
				log.Printf("Failed to update session strategy: %v", err)
				// Non‑fatal, proceed with existing strategy
			}
		}
	}

	result, err := aiAgent.SendMessage(req.SessionID, req.Message)
	if err != nil {
		log.Printf("Error sending message: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"response":   result.Content,
		"session_id": req.SessionID,
		"usage": map[string]interface{}{
			"prompt_tokens":     result.Usage.PromptTokens,
			"completion_tokens": result.Usage.CompletionTokens,
			"total_tokens":      result.Usage.TotalTokens,
		},
	})
}

// handleSessions returns list of sessions (GET /api/sessions)
func handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionIDs, err := store.ListSessions()
	if err != nil {
		log.Printf("Error listing sessions: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Build session objects with metadata (for simplicity we just return IDs)
	sessions := make([]map[string]interface{}, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		session, err := store.GetSession(id)
		if err != nil {
			continue
		}
		lastMessage := ""
		if len(session.History) > 0 {
			lastMessage = session.History[len(session.History)-1].Content
		}
		sessions = append(sessions, map[string]interface{}{
			"id":           id,
			"last_message": lastMessage,
			"strategy":     session.Strategy,
			"updated_at":   session.UpdatedAt,
			"created_at":   session.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// handleSession handles GET /api/sessions/{id} and DELETE /api/sessions/{id}
func handleSession(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if path == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}
	// Check if the path ends with "/copy" for copy operation
	if strings.HasSuffix(path, "/copy") {
		// Delegate to handleSessionCopy
		handleSessionCopy(w, r)
		return
	}
	sessionID := path

	switch r.Method {
	case http.MethodGet:
		session, err := aiAgent.GetSession(sessionID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if session == nil {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(session)
	case http.MethodDelete:
		err := aiAgent.ClearSession(sessionID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSessionCopy creates a copy of an existing session.
func handleSessionCopy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract source session ID from URL path
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if path == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}
	// Remove "/copy" suffix
	if !strings.HasSuffix(path, "/copy") {
		// This should not happen if routing is correct
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	sourceID := strings.TrimSuffix(path, "/copy")
	if sourceID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	// Generate new session ID
	newID := uuid.New().String()

	// Copy session using agent
	err := aiAgent.CopySession(sourceID, newID)
	if err != nil {
		if err == agent.ErrSessionNotFound {
			http.Error(w, "Source session not found", http.StatusNotFound)
			return
		}
		log.Printf("Error copying session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"new_session_id":    newID,
		"source_session_id": sourceID,
		"message":           "Session copied successfully",
	})
}
