// Generate a UUID for session ID
function generateSessionId() {
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
        const r = Math.random() * 16 | 0;
        const v = c === 'x' ? r : (r & 0x3 | 0x8);
        return v.toString(16);
    });
}

// Get or create session ID from localStorage
let sessionId = localStorage.getItem('sessionId');
if (!sessionId) {
    sessionId = generateSessionId();
    localStorage.setItem('sessionId', sessionId);
}
document.getElementById('session-id').innerHTML = `Session: <code>${sessionId}</code>`;

// DOM elements
const chatMessages = document.getElementById('chat-messages');
const messageInput = document.getElementById('message-input');
const sendButton = document.getElementById('send-button');
const clearButton = document.getElementById('clear-button');
const copySessionButton = document.getElementById('copy-session');
const newSessionButton = document.getElementById('new-session');

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

// Start a new session
function newSession() {
    if (confirm('Start a new session? The current session history will be lost on the server.')) {
        sessionId = generateSessionId();
        localStorage.setItem('sessionId', sessionId);
        document.getElementById('session-id').innerHTML = `Session: <code>${sessionId}</code>`;
        chatMessages.innerHTML = '';
        addMessage('system', 'New session started. Previous history is cleared on the server.');
    }
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

// Focus input on load
messageInput.focus();