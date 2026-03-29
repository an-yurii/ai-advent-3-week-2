// Generate a UUID for session ID
function generateSessionId() {
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
        const r = Math.random() * 16 | 0;
        const v = c === 'x' ? r : (r & 0x3 | 0x8);
        return v.toString(16);
    });
}

// Get session ID from URL query parameter, else localStorage, else generate
function getSessionIdFromUrl() {
    const params = new URLSearchParams(window.location.search);
    return params.get('session');
}

// Get strategy from URL query parameter, else localStorage, else default 'summary'
function getStrategy() {
    const params = new URLSearchParams(window.location.search);
    const urlStrategy = params.get('strategy');
    if (urlStrategy && (urlStrategy === 'summary' || urlStrategy === 'sliding_window' || urlStrategy === 'sticky_facts')) {
        return urlStrategy;
    }
    const saved = localStorage.getItem('contextStrategy');
    if (saved && (saved === 'summary' || saved === 'sliding_window' || saved === 'sticky_facts')) {
        return saved;
    }
    return 'summary';
}

let sessionId = getSessionIdFromUrl();
if (!sessionId) {
    sessionId = localStorage.getItem('sessionId');
    if (!sessionId) {
        sessionId = generateSessionId();
        localStorage.setItem('sessionId', sessionId);
    }
} else {
    // If session ID from URL, store it for future
    localStorage.setItem('sessionId', sessionId);
}

// Update session display
document.getElementById('session-id').innerHTML = `Session: <code>${sessionId}</code>`;

// DOM elements
const chatMessages = document.getElementById('chat-messages');
const messageInput = document.getElementById('message-input');
const sendButton = document.getElementById('send-button');
const clearButton = document.getElementById('clear-button');
const copySessionButton = document.getElementById('copy-session');
const copyDialogueButton = document.getElementById('copy-dialogue');
const newSessionButton = document.getElementById('new-session');
const backToLandingButton = document.getElementById('back-to-landing');
const fabNewSession = document.getElementById('fab-new-session');
const toast = document.getElementById('toast');
const toastMessage = document.getElementById('toast-message');

// Add a message to the chat UI
function addMessage(role, content) {
    const messageDiv = document.createElement('div');
    messageDiv.className = `message ${role}`;
    let icon = 'fas fa-user';
    let name = 'You';
    if (role === 'assistant') {
        icon = 'fas fa-robot';
        name = 'Assistant';
    } else if (role === 'system') {
        icon = 'fas fa-info-circle';
        name = 'System';
    }
    messageDiv.innerHTML = `
        <div class="avatar"><i class="${icon}"></i></div>
        <div class="content">
            <strong>${name}</strong>
            <p>${content}</p>
        </div>
    `;
    chatMessages.appendChild(messageDiv);
    chatMessages.scrollTop = chatMessages.scrollHeight;
}

// Load session history from server
async function loadSessionHistory() {
    try {
        const response = await fetch(`/api/sessions/${sessionId}`);
        if (!response.ok) {
            if (response.status === 404) {
                // Session doesn't exist yet, that's fine
                return;
            }
            throw new Error(`HTTP ${response.status}: ${await response.text()}`);
        }
        const session = await response.json();
        console.log('Loaded session:', session);
        if (session.history && session.history.length > 0) {
            // Clear the default system message
            chatMessages.innerHTML = '';
            // Add each historical message
            session.history.forEach(msg => addMessage(msg.role, msg.content));
            // Compute token totals from history
            let lastPrompt = 0, lastCompletion = 0, lastTotal = 0;
            for (const msg of session.history) {
                if (msg.role === 'assistant' && msg.prompt_tokens) {
                    lastPrompt = msg.prompt_tokens;
                    lastCompletion = msg.completion_tokens;
                    lastTotal = msg.total_tokens; // total_tokens of the last assistant message is cumulative
                }
            }
            // Update token counters
            updateTokenCounts(lastPrompt, lastCompletion, lastTotal);
        }
    } catch (error) {
        console.error('Failed to load session history:', error);
        addMessage('system', `Note: Could not load previous messages (${error.message})`);
    }
}

// Update token counters in the UI
function updateTokenCounts(prompt, completion, total) {
    document.getElementById('token-prompt').textContent = prompt;
    document.getElementById('token-completion').textContent = completion;
    document.getElementById('token-total').textContent = total;
}

// Send message to server
async function sendMessage() {
    const text = messageInput.value.trim();
    if (!text) return;

    // Add user message to UI
    addMessage('user', text);
    messageInput.value = '';
    sendButton.disabled = true;

    try {
        const response = await fetch('/api/chat', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                message: text,
                session_id: sessionId,
                strategy: getStrategy()
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${await response.text()}`);
        }

        const data = await response.json();
        addMessage('assistant', data.response);
        // Update token counts if available
        if (data.usage) {
            updateTokenCounts(
                data.usage.prompt_tokens,
                data.usage.completion_tokens,
                data.usage.total_tokens
            );
        }
    } catch (error) {
        console.error('Error sending message:', error);
        addMessage('system', `Error: ${error.message}`);
    } finally {
        sendButton.disabled = false;
        messageInput.focus();
    }
}

// Clear chat UI (does not clear server-side history)
function clearChat() {
    if (confirm('Clear all messages from this view? (Session history remains on server)')) {
        chatMessages.innerHTML = '';
        addMessage('system', 'Chat cleared. Session history is still stored on the server.');
    }
}

// Copy session ID to clipboard
function copySessionId() {
    navigator.clipboard.writeText(sessionId).then(() => {
        const original = copySessionButton.innerHTML;
        copySessionButton.innerHTML = '<i class="fas fa-check"></i>';
        setTimeout(() => {
            copySessionButton.innerHTML = original;
        }, 2000);
    });
}

// Copy dialogue (create a branch)
async function copyDialogue() {
    try {
        const response = await fetch(`/api/sessions/${sessionId}/copy`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
        });
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${await response.text()}`);
        }
        const data = await response.json();
        const newSessionId = data.new_session_id;
        // Update session ID
        sessionId = newSessionId;
        localStorage.setItem('sessionId', sessionId);
        document.getElementById('session-id').innerHTML = `Session: <code>${sessionId}</code>`;
        // Add a system message
        addMessage('system', `Dialogue copied to new session (${newSessionId}). You are now in the copied session.`);
        // Update URL
        const url = new URL(window.location);
        url.searchParams.set('session', sessionId);
        const strategy = getStrategy();
        url.searchParams.set('strategy', strategy);
        window.history.pushState({}, '', url);
        // Show toast
        showToast('Dialogue copied to new session!', 'info');
    } catch (error) {
        console.error('Error copying dialogue:', error);
        addMessage('system', `Error copying dialogue: ${error.message}`);
    }
}

// Show a toast notification
function showToast(message, type = 'info') {
    toastMessage.textContent = message;
    toast.className = 'toast ' + type;
    toast.classList.add('show');
    setTimeout(() => {
        toast.classList.remove('show');
    }, 3000);
}

// Start a new session (creates new ID and navigates)
function newSession() {
    if (confirm('Start a new session? The current session history will be lost on the server.')) {
        sessionId = generateSessionId();
        localStorage.setItem('sessionId', sessionId);
        document.getElementById('session-id').innerHTML = `Session: <code>${sessionId}</code>`;
        chatMessages.innerHTML = '';
        addMessage('system', 'New session started. Previous history is cleared on the server.');
        // Reset token counters
        updateTokenCounts(0, 0, 0);
        // Update URL without reload, include current strategy
        const url = new URL(window.location);
        url.searchParams.set('session', sessionId);
        const strategy = getStrategy();
        url.searchParams.set('strategy', strategy);
        window.history.pushState({}, '', url);
        showToast('New chat session created!', 'info');
    }
}

// Go back to landing page
function goToLanding() {
    window.location.href = '/';
}

// Event listeners
sendButton.addEventListener('click', sendMessage);
messageInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
    }
});
clearButton.addEventListener('click', clearChat);
copySessionButton.addEventListener('click', copySessionId);
copyDialogueButton.addEventListener('click', copyDialogue);
newSessionButton.addEventListener('click', newSession);
if (fabNewSession) {
    fabNewSession.addEventListener('click', newSession);
}
if (backToLandingButton) {
    backToLandingButton.addEventListener('click', goToLanding);
}

// Load history on page load
document.addEventListener('DOMContentLoaded', () => {
    loadSessionHistory();
    messageInput.focus();
});