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
        if (session.history && session.history.length > 0) {
            // Clear the default system message
            chatMessages.innerHTML = '';
            // Add each historical message
            session.history.forEach(msg => addMessage(msg.role, msg.content));
        }
    } catch (error) {
        console.error('Failed to load session history:', error);
        addMessage('system', `Note: Could not load previous messages (${error.message})`);
    }
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
                session_id: sessionId
            })
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${await response.text()}`);
        }

        const data = await response.json();
        addMessage('assistant', data.response);
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
        // Update URL without reload
        const url = new URL(window.location);
        url.searchParams.set('session', sessionId);
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