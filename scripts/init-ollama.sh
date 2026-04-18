#!/bin/sh
set -e

echo "Starting Ollama initialization..."

# Start Ollama in the background
/bin/ollama serve &
OLLAMA_PID=$!

# Wait for Ollama to be ready (simple sleep, could be improved)
echo "Waiting for Ollama to be ready..."
sleep 5

echo "Checking for model: $OLLAMA_EMBEDDING_MODEL"

# Check if model exists
if ollama list | grep -q "$OLLAMA_EMBEDDING_MODEL"; then
    echo "Model $OLLAMA_EMBEDDING_MODEL already exists."
else
    echo "Pulling model $OLLAMA_EMBEDDING_MODEL..."
    ollama pull "$OLLAMA_EMBEDDING_MODEL"
    echo "Model $OLLAMA_EMBEDDING_MODEL pulled successfully."
fi

echo "Ollama initialization complete."

# Keep the container running by waiting on the Ollama process
wait $OLLAMA_PID