# API Documentation

## Base URL

When running locally: `http://localhost:8080`

## Endpoints

### `GET /`

Serves the web interface (HTML, CSS, JavaScript).

**Response:** HTML page.

---

### `GET /api/sessions`

Returns a list of all conversation sessions, ordered by creation time (newest first).

#### Response

**Success (200 OK)**

```json
[
  {
    "id": "string",
    "last_message": "string",
    "updated_at": "ISO 8601 timestamp or null"
  },
  ...
]
```

- `id`: Session identifier.
- `last_message`: Content of the most recent message in the session (or empty string).
- `updated_at`: Timestamp of the last update (if available).

**Error Responses**

| Status Code | Description          | Example Body                          |
|-------------|----------------------|---------------------------------------|
| 500         | Internal server error | `{"error":"Internal server error"}`   |

#### Example

```bash
curl http://localhost:8080/api/sessions
```

---

### `GET /api/sessions/{id}`

Returns the full conversation history for a specific session.

#### Path Parameters

- `id` (required) – Session identifier (UUID).

#### Response

**Success (200 OK)**

```json
{
  "id": "string",
  "history": [
    {
      "role": "user|assistant",
      "content": "string"
    }
  ]
}
```

- `history`: Array of messages in chronological order.

**Error Responses**

| Status Code | Description          | Example Body                          |
|-------------|----------------------|---------------------------------------|
| 404         | Session not found    | `{"error":"Session not found"}`       |
| 500         | Internal server error | `{"error":"Internal server error"}`   |

#### Example

```bash
curl http://localhost:8080/api/sessions/abc123
```

---

### `DELETE /api/sessions/{id}`

Deletes a session and all its messages.

#### Path Parameters

- `id` (required) – Session identifier (UUID).

#### Response

**Success (204 No Content)**

No body.

**Error Responses**

| Status Code | Description          | Example Body                          |
|-------------|----------------------|---------------------------------------|
| 404         | Session not found    | `{"error":"Session not found"}`       |
| 500         | Internal server error | `{"error":"Internal server error"}`   |

#### Example

```bash
curl -X DELETE http://localhost:8080/api/sessions/abc123
```

---

### `POST /api/chat`

Processes a user message and returns the AI assistant's response.

#### Request Headers

```
Content-Type: application/json
```

#### Request Body

```json
{
  "message": "string, required",
  "session_id": "string, required",
  "strategy": "string, optional"
}
```

- `message`: The user's input text.
- `session_id`: Unique identifier for the conversation session. If a new session ID is provided, a new conversation history is started.
- `strategy`: Context management strategy. One of: `summary` (default), `sliding_window`, `sticky_facts`. If omitted, the session's current strategy is used (defaults to `summary`).

#### Response

**Success (200 OK)**

```json
{
  "response": "string",
  "session_id": "string"
}
```

- `response`: The AI assistant's reply.
- `session_id`: Echoes the session ID from the request.

**Error Responses**

| Status Code | Description                               | Example Body                              |
|-------------|-------------------------------------------|-------------------------------------------|
| 400         | Missing or invalid fields                 | `{"error":"Missing 'message' or 'session_id'"}` |
| 405         | Method not allowed                        | `{"error":"Method not allowed"}`          |
| 503         | GigaChat API unavailable or internal error | `{"error":"Service unavailable"}`         |
| 500         | Internal server error                     | `{"error":"Internal server error"}`       |

#### Example

**Request**

```bash
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"Hello, how are you?", "session_id":"abc123"}'
```

**Response**

```json
{
  "response": "I'm fine, thank you! How can I assist you today?",
  "session_id": "abc123"
}
```

## Session Management

- Each `session_id` maps to an independent conversation history.
- History is **persisted in a PostgreSQL database** and survives server restarts.
- The server keeps the full history of each session; however, when the number of messages exceeds `HISTORY_MAX_MESSAGES` (configurable via environment variable), older messages are automatically summarized into a single system message to reduce length. The summary and a notification message appear in the history.
- The agent supports multiple context management strategies:
  - **summary** (default): Summarizes older messages when history length exceeds a limit.
  - **sliding_window**: Keeps only the most recent N messages (configurable via `SLIDING_WINDOW_SIZE`).
  - **sticky_facts**: Maintains a sliding window of recent messages and extracts key facts from the conversation, which are sent as a system message at the start of each request. Facts are updated after each turn.
- To start a fresh conversation, provide a new `session_id`.
- The web interface automatically generates a UUID for the session and stores it in the browser's local storage, allowing the user to resume the same conversation across page reloads.
- The new landing page (`/`) provides a list of previous sessions and the ability to create a new session.

## Database

The application uses PostgreSQL to store sessions and messages. The database schema is created automatically on first launch. The following tables are used:

- `sessions` – stores session metadata (id, created_at, updated_at).
- `messages` – stores each message with role (`user` or `assistant`), content, and timestamp.

All messages are saved immediately after they are sent or received. When a request with an existing `session_id` arrives, the agent loads the previous messages from the database and includes them in the context sent to GigaChat.

## Logging

All requests and responses are logged with the following format:

```
[YYYY-MM-DD HH:MM:SS.mmm] [LEVEL] [message] [key=value ...]
```

Log categories:

- `HTTP_REQUEST` – Incoming HTTP request (method, path, headers, body).
- `HTTP_RESPONSE` – Outgoing HTTP response (status, headers, body).
- `GIGACHAT_REQUEST` – Request sent to GigaChat API (URL, headers, body).
- `GIGACHAT_RESPONSE` – Response from GigaChat API (status, headers, body).
- `ERROR` – Any error with stack trace.

## Error Handling

- **Client errors (4xx)** – Invalid input, missing fields, etc.
- **Server errors (5xx)** – GigaChat API failure, internal processing errors.

All errors are logged with details.

## Rate Limiting

Currently not implemented. The server processes requests sequentially per session but can handle multiple sessions concurrently.

## Testing the API

You can use the provided web interface or tools like `curl` or Postman.

### Example with `curl`

```bash
# Send a message
curl -X POST http://localhost:8080/api/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"What is the capital of France?", "session_id":"test-1"}'

# Response
{"response":"The capital of France is Paris.","session_id":"test-1"}

# List sessions
curl http://localhost:8080/api/sessions

# Get session history
curl http://localhost:8080/api/sessions/test-1

# Delete a session
curl -X DELETE http://localhost:8080/api/sessions/test-1