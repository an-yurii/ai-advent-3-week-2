package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"ai-agent-gigachat/internal/agent"
	"ai-agent-gigachat/internal/storage"
	"ai-agent-gigachat/internal/storage/memory"
)

var aiAgent *agent.Agent
var store storage.Storage

func main() {
	// Initialize storage (in-memory for now)
	store = memory.New()
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Message == "" || req.SessionID == "" {
		http.Error(w, "Missing 'message' or 'session_id'", http.StatusBadRequest)
		return
	}

	response, err := aiAgent.SendMessage(req.SessionID, req.Message)
	if err != nil {
		log.Printf("Error sending message: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"response":   response,
		"session_id": req.SessionID,
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
			"updated_at":   nil, // memory storage doesn't have timestamps
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