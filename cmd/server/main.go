package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

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

	// Determine agent type
	agentType := os.Getenv("AGENT_TYPE")
	if agentType == "" {
		agentType = "gigachat" // default
	}

	var apiKey string
	if agentType == "gigachat" {
		// API key is required for GigaChat
		apiKey = os.Getenv("GIGACHAT_API_KEY")
		if apiKey == "" {
			log.Fatal("GIGACHAT_API_KEY environment variable is required when AGENT_TYPE=gigachat")
		}
	} else {
		// For Ollama, API key is not required
		apiKey = ""
	}

	aiAgent = agent.NewAgent(apiKey, store)

	// Serve static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// API routes
	http.HandleFunc("/api/chat", handleChat)
	http.HandleFunc("/api/sessions", handleSessions)
	http.HandleFunc("/api/sessions/", handleSession)
	http.HandleFunc("/api/profiles", handleProfiles)
	http.HandleFunc("/api/profiles/", handleProfile)
	http.HandleFunc("/api/session/state/", handleSessionState)

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
		ProfileID string `json:"profile_id,omitempty"`
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

	// Update session profile if provided, but only if session doesn't already have a profile
	if req.ProfileID != "" {
		// Ensure session exists (idempotent)
		if err := store.CreateSession(req.SessionID); err != nil {
			log.Printf("Failed to create session for profile update: %v", err)
			// Continue anyway, SendMessage will also try to create
		} else {
			// Check if session already has a profile
			session, err := store.GetSession(req.SessionID)
			if err != nil {
				log.Printf("Failed to retrieve session for profile check: %v", err)
				// Proceed with update as fallback
			} else if session != nil && session.ProfileID != "" {
				log.Printf("Session %s already has profile %s, keeping it (requested profile %s)", req.SessionID, session.ProfileID, req.ProfileID)
				// Skip updating profile
			} else {
				// Session has no profile, update with requested profile
				if err := store.UpdateSessionProfile(req.SessionID, req.ProfileID); err != nil {
					log.Printf("Failed to update session profile: %v", err)
					// Non‑fatal, proceed without profile
				} else {
					log.Printf("Updated session %s profile to %s", req.SessionID, req.ProfileID)
				}
			}
		}
	}

	result, err := aiAgent.SendMessage(req.SessionID, req.Message)
	if err != nil {
		log.Printf("Error sending message: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get FSM state info if available
	var fsmState map[string]interface{}
	if aiAgent != nil {
		stateInfo, err := aiAgent.GetFSMStateInfo(req.SessionID)
		if err == nil && stateInfo != nil {
			fsmState = map[string]interface{}{
				"step_number": stateInfo.StepNumber,
				"steps_count": stateInfo.StepsCount,
				"description": stateInfo.Description,
				"state":       stateInfo.State,
				"done":        stateInfo.Done,
				"error":       stateInfo.Error,
			}
		}
	}

	response := map[string]interface{}{
		"response":   result.Content,
		"session_id": req.SessionID,
		"usage": map[string]interface{}{
			"prompt_tokens":     result.Usage.PromptTokens,
			"completion_tokens": result.Usage.CompletionTokens,
			"total_tokens":      result.Usage.TotalTokens,
		},
	}

	// Add FSM state if available
	if fsmState != nil {
		response["fsm_state"] = fsmState
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSessionState returns FSM state for a session (GET /api/session/state/{sessionID})
func handleSessionState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract session ID from URL path
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	sessionID := pathParts[4]

	if aiAgent == nil {
		http.Error(w, "Agent not initialized", http.StatusServiceUnavailable)
		return
	}

	stateInfo, err := aiAgent.GetFSMStateInfo(sessionID)
	if err != nil {
		// Check if error is because FSM is not configured
		if err.Error() == "FSM not configured" {
			// Return error state info
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"initialized": true,
				"error":       true,
				"message":     "FSM not configured for this session",
				"step_number": 0,
				"steps_count": 0,
				"description": "",
				"state":       "",
				"done":        false,
			})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if stateInfo == nil {
		// No FSM context
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"initialized": false,
			"message":     "No FSM context for this session",
		})
		return
	}

	// Get task context for additional details
	var taskContext *storage.TaskContext
	if store != nil {
		taskContext, _ = store.GetTaskContext(sessionID)
	}

	response := map[string]interface{}{
		"step_number": stateInfo.StepNumber,
		"steps_count": stateInfo.StepsCount,
		"description": stateInfo.Description,
		"state":       stateInfo.State,
		"done":        stateInfo.Done,
		"error":       stateInfo.Error,
		"initialized": true,
	}

	if taskContext != nil {
		response["task"] = taskContext.Task
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
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
			"profile_id":   session.ProfileID,
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

// handleProfiles handles GET /api/profiles and POST /api/profiles
func handleProfiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List all profiles
		profiles, err := store.ListProfiles()
		if err != nil {
			log.Printf("Error listing profiles: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profiles)

	case http.MethodPost:
		// Create new profile
		var profile storage.Profile
		if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if profile.Name == "" {
			http.Error(w, "Profile name is required", http.StatusBadRequest)
			return
		}

		// Generate ID if not provided
		if profile.ID == "" {
			profile.ID = uuid.New().String()
		}

		// Set timestamps
		now := time.Now()
		profile.CreatedAt = now
		profile.UpdatedAt = now

		// Create profile
		if err := store.CreateProfile(profile); err != nil {
			log.Printf("Error creating profile: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(profile)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProfile handles GET /api/profiles/{id}, PUT /api/profiles/{id}, DELETE /api/profiles/{id}
func handleProfile(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/profiles/")
	if path == "" {
		http.Error(w, "Profile ID required", http.StatusBadRequest)
		return
	}

	// Check for set-default endpoint
	if strings.HasSuffix(path, "/set-default") {
		profileID := strings.TrimSuffix(path, "/set-default")
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := store.SetDefaultProfile(profileID); err != nil {
			if err == storage.ErrProfileNotFound {
				http.Error(w, "Profile not found", http.StatusNotFound)
				return
			}
			log.Printf("Error setting default profile: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Default profile updated"})
		return
	}

	profileID := path

	switch r.Method {
	case http.MethodGet:
		// Get profile by ID
		profile, err := store.GetProfile(profileID)
		if err != nil {
			if err == storage.ErrProfileNotFound {
				http.Error(w, "Profile not found", http.StatusNotFound)
				return
			}
			log.Printf("Error getting profile: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile)

	case http.MethodPut:
		// Update profile
		var profile storage.Profile
		if err := json.NewDecoder(r.Body).Decode(&profile); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Ensure ID matches
		profile.ID = profileID
		profile.UpdatedAt = time.Now()

		if err := store.UpdateProfile(profileID, profile); err != nil {
			if err == storage.ErrProfileNotFound {
				http.Error(w, "Profile not found", http.StatusNotFound)
				return
			}
			log.Printf("Error updating profile: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile)

	case http.MethodDelete:
		// Delete profile
		if err := store.DeleteProfile(profileID); err != nil {
			if err == storage.ErrProfileNotFound {
				http.Error(w, "Profile not found", http.StatusNotFound)
				return
			}
			if err == storage.ErrProfileInUse {
				http.Error(w, "Profile is in use by sessions", http.StatusConflict)
				return
			}
			log.Printf("Error deleting profile: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"message": "Profile deleted"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
