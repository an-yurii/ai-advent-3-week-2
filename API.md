# API Documentation

## Base URL

When running locally: `http://localhost:8080`

## Endpoints

### `GET /`

Serves the web interface (HTML, CSS, JavaScript).

**Response:** HTML page.

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
  "session_id": "string, required"
}
```

- `message`: The user's input text.
- `session_id`: Unique identifier for the conversation session. If a new session ID is provided, a new conversation history is started.

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
- History is stored in memory (not persisted across server restarts).
- The server keeps up to the last N messages (currently unlimited) per session.
- To start a fresh conversation, provide a new `session_id`.

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