-- Migration 003: Add task_context field to sessions table
-- Adds JSONB field for storing finite state machine context

-- Add task_context column as JSONB type
ALTER TABLE sessions 
ADD COLUMN IF NOT EXISTS task_context JSONB DEFAULT NULL;

-- Create index for efficient querying on task_context fields
CREATE INDEX IF NOT EXISTS idx_sessions_task_context_state 
ON sessions USING gin ((task_context->>'state'));

CREATE INDEX IF NOT EXISTS idx_sessions_task_context_done 
ON sessions USING gin ((task_context->>'done'));

-- Add comment explaining the column
COMMENT ON COLUMN sessions.task_context IS 'Finite state machine context for task processing. Contains state, task, done flag, and metadata.';

-- Example of what task_context might contain:
-- {
--   "state": "gathering_requirements",
--   "task": "First user message content",
--   "done": false,
--   "metadata": {
--     "step_number": 1,
--     "validation_results": [],
--     "transition_history": [
--       {"from": null, "to": "gathering_requirements", "timestamp": "2024-01-01T00:00:00Z"}
--     ]
--   }
-- }