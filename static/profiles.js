// Profile Management JavaScript

// DOM Elements
const profilesList = document.getElementById('profiles-list');
const profileEditor = document.getElementById('profile-editor');
const newProfileBtn = document.getElementById('new-profile-btn');
const profileFormOverlay = document.getElementById('profile-form-overlay');
const closeFormBtn = document.getElementById('close-form-btn');
const cancelBtn = document.getElementById('cancel-btn');
const profileForm = document.getElementById('profile-form');
const formTitle = document.getElementById('form-title');
const profileIdInput = document.getElementById('profile-id');
const profileNameInput = document.getElementById('profile-name');
const profileStyleInput = document.getElementById('profile-style');
const profileConstraintsInput = document.getElementById('profile-constraints');
const profileContextInput = document.getElementById('profile-context');
const profileDefaultInput = document.getElementById('profile-default');
const saveBtn = document.getElementById('save-btn');
const deleteBtn = document.getElementById('delete-btn');
const toast = document.getElementById('toast');
const toastMessage = document.getElementById('toast-message');

let profiles = [];
let selectedProfileId = null;

// Show toast notification
function showToast(message, type = 'success') {
    toastMessage.textContent = message;
    toast.className = 'toast';
    toast.classList.add(type);
    toast.style.display = 'block';
    
    setTimeout(() => {
        toast.style.display = 'none';
    }, 3000);
}

// Load profiles from server
async function loadProfiles() {
    try {
        profilesList.innerHTML = '<div class="loading">Loading profiles...</div>';
        
        const response = await fetch('/api/profiles');
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${await response.text()}`);
        }
        
        profiles = await response.json();
        renderProfilesList();
        
        // If no profile is selected but we have profiles, select the first one
        if (!selectedProfileId && profiles.length > 0) {
            selectProfile(profiles[0].id);
        }
    } catch (error) {
        console.error('Error loading profiles:', error);
        profilesList.innerHTML = `
            <div class="empty-state">
                <i class="fas fa-exclamation-triangle"></i>
                <p>Failed to load profiles</p>
                <button class="btn btn-secondary" onclick="loadProfiles()">
                    <i class="fas fa-redo"></i> Retry
                </button>
            </div>
        `;
    }
}

// Render profiles list
function renderProfilesList() {
    if (profiles.length === 0) {
        profilesList.innerHTML = `
            <div class="empty-state">
                <i class="fas fa-users"></i>
                <p>No profiles yet. Create your first profile!</p>
            </div>
        `;
        return;
    }
    
    profilesList.innerHTML = profiles.map(profile => `
        <div class="profile-card ${selectedProfileId === profile.id ? 'selected' : ''}" 
             data-profile-id="${profile.id}">
            <div class="profile-card-header">
                <h3 class="profile-name">${escapeHtml(profile.name)}</h3>
                ${profile.is_default ? 
                    '<span class="profile-default-badge"><i class="fas fa-star"></i> Default</span>' : 
                    ''}
            </div>
            <div class="profile-preview">
                ${escapeHtml(profile.style || 'No style defined').substring(0, 100)}...
            </div>
            <div class="profile-actions">
                <button class="btn-edit" onclick="editProfile('${profile.id}')">
                    <i class="fas fa-edit"></i> Edit
                </button>
                ${profile.id !== '00000000-0000-0000-0000-000000000000' ? 
                    `<button class="btn-delete-profile" onclick="deleteProfile('${profile.id}')">
                        <i class="fas fa-trash"></i> Delete
                    </button>` : 
                    ''}
            </div>
        </div>
    `).join('');
    
    // Add click event to profile cards
    document.querySelectorAll('.profile-card').forEach(card => {
        card.addEventListener('click', (e) => {
            if (!e.target.closest('button')) {
                const profileId = card.dataset.profileId;
                selectProfile(profileId);
            }
        });
    });
}

// Select a profile to view details
function selectProfile(profileId) {
    selectedProfileId = profileId;
    const profile = profiles.find(p => p.id === profileId);
    
    if (!profile) {
        profileEditor.innerHTML = `
            <div class="editor-placeholder">
                <i class="fas fa-exclamation-circle"></i>
                <p>Profile not found</p>
            </div>
        `;
        return;
    }
    
    profileEditor.innerHTML = `
        <div class="profile-details">
            <div class="detail-header">
                <h3>${escapeHtml(profile.name)}</h3>
                ${profile.is_default ? 
                    '<span class="profile-default-badge"><i class="fas fa-star"></i> Default Profile</span>' : 
                    ''}
            </div>
            
            <div class="detail-section">
                <h4><i class="fas fa-paint-brush"></i> Style</h4>
                <div class="detail-content">${formatText(profile.style) || '<em>No style defined</em>'}</div>
            </div>
            
            <div class="detail-section">
                <h4><i class="fas fa-ban"></i> Constraints</h4>
                <div class="detail-content">${formatText(profile.constraints) || '<em>No constraints defined</em>'}</div>
            </div>
            
            <div class="detail-section">
                <h4><i class="fas fa-info-circle"></i> Context</h4>
                <div class="detail-content">${formatText(profile.context) || '<em>No context defined</em>'}</div>
            </div>
            
            <div class="detail-meta">
                <p><i class="fas fa-calendar"></i> Created: ${new Date(profile.created_at).toLocaleString()}</p>
                <p><i class="fas fa-clock"></i> Updated: ${new Date(profile.updated_at).toLocaleString()}</p>
            </div>
            
            <div class="detail-actions">
                <button class="btn btn-primary" onclick="editProfile('${profile.id}')">
                    <i class="fas fa-edit"></i> Edit Profile
                </button>
                ${profile.id !== '00000000-0000-0000-0000-000000000000' ? 
                    `<button class="btn btn-danger" onclick="deleteProfile('${profile.id}')">
                        <i class="fas fa-trash"></i> Delete Profile
                    </button>` : 
                    ''}
            </div>
        </div>
    `;
    
    renderProfilesList();
}

// Format text for display (preserve line breaks)
function formatText(text) {
    if (!text) return '';
    return escapeHtml(text).replace(/\n/g, '<br>');
}

// Escape HTML to prevent XSS
function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
}

// Show profile form for editing or creating
function showProfileForm(profile = null) {
    if (profile) {
        // Edit mode
        formTitle.textContent = 'Edit Profile';
        profileIdInput.value = profile.id;
        profileNameInput.value = profile.name;
        profileStyleInput.value = profile.style || '';
        profileConstraintsInput.value = profile.constraints || '';
        profileContextInput.value = profile.context || '';
        profileDefaultInput.checked = profile.is_default || false;
        deleteBtn.style.display = profile.id !== '00000000-0000-0000-0000-000000000000' ? 'block' : 'none';
    } else {
        // Create mode
        formTitle.textContent = 'New Profile';
        profileIdInput.value = '';
        profileNameInput.value = '';
        profileStyleInput.value = '';
        profileConstraintsInput.value = '';
        profileContextInput.value = '';
        profileDefaultInput.checked = false;
        deleteBtn.style.display = 'none';
    }
    
    profileFormOverlay.classList.add('active');
}

// Hide profile form
function hideProfileForm() {
    profileFormOverlay.classList.remove('active');
    profileForm.reset();
}

// Edit a profile
function editProfile(profileId) {
    const profile = profiles.find(p => p.id === profileId);
    if (profile) {
        showProfileForm(profile);
    }
}

// Delete a profile
async function deleteProfile(profileId) {
    if (profileId === '00000000-0000-0000-0000-000000000000') {
        showToast('Cannot delete the default profile', 'error');
        return;
    }
    
    if (!confirm('Are you sure you want to delete this profile? This action cannot be undone.')) {
        return;
    }
    
    try {
        const response = await fetch(`/api/profiles/${profileId}`, {
            method: 'DELETE'
        });
        
        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }
        
        showToast('Profile deleted successfully');
        
        // Reload profiles
        await loadProfiles();
        
        // Clear editor if deleted profile was selected
        if (selectedProfileId === profileId) {
            selectedProfileId = null;
            profileEditor.innerHTML = `
                <div class="editor-placeholder">
                    <i class="fas fa-user-edit"></i>
                    <p>Select a profile to edit or create a new one</p>
                </div>
            `;
        }
    } catch (error) {
        console.error('Error deleting profile:', error);
        showToast(`Failed to delete profile: ${error.message}`, 'error');
    }
}

// Save profile (create or update)
async function saveProfile(event) {
    event.preventDefault();
    
    const profileData = {
        id: profileIdInput.value || undefined,
        name: profileNameInput.value,
        style: profileStyleInput.value,
        constraints: profileConstraintsInput.value,
        context: profileContextInput.value,
        is_default: profileDefaultInput.checked
    };
    
    try {
        let response;
        let isNew = !profileIdInput.value;
        
        if (isNew) {
            // Create new profile
            response = await fetch('/api/profiles', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(profileData)
            });
        } else {
            // Update existing profile
            response = await fetch(`/api/profiles/${profileIdInput.value}`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(profileData)
            });
        }
        
        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(`HTTP ${response.status}: ${errorText}`);
        }
        
        const savedProfile = await response.json();
        
        showToast(isNew ? 'Profile created successfully' : 'Profile updated successfully');
        hideProfileForm();
        
        // Reload profiles
        await loadProfiles();
        
        // Select the saved profile
        selectProfile(savedProfile.id);
        
    } catch (error) {
        console.error('Error saving profile:', error);
        showToast(`Failed to save profile: ${error.message}`, 'error');
    }
}

// Set a profile as default
async function setDefaultProfile(profileId) {
    try {
        const response = await fetch(`/api/profiles/${profileId}/set-default`, {
            method: 'POST'
        });
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${await response.text()}`);
        }
        
        showToast('Default profile updated');
        await loadProfiles();
    } catch (error) {
        console.error('Error setting default profile:', error);
        showToast(`Failed to set default profile: ${error.message}`, 'error');
    }
}

// Event Listeners
newProfileBtn.addEventListener('click', () => showProfileForm());
closeFormBtn.addEventListener('click', hideProfileForm);
cancelBtn.addEventListener('click', hideProfileForm);
profileForm.addEventListener('submit', saveProfile);

// Close form when clicking outside
profileFormOverlay.addEventListener('click', (e) => {
    if (e.target === profileFormOverlay) {
        hideProfileForm();
    }
});

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    loadProfiles();
    
    // Add CSS for profile details
    const style = document.createElement('style');
    style.textContent = `
        .profile-details {
            padding: 1rem;
        }
        .detail-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 1.5rem;
            padding-bottom: 1rem;
            border-bottom: 1px solid #334155;
        }
        .detail-header h3 {
            margin: 0;
            color: #f1f5f9;
            font-size: 1.5rem;
        }
        .detail-section {
            margin-bottom: 1.5rem;
        }
        .detail-section h4 {
            color: #cbd5e1;
            font-size: 1rem;
            margin-bottom: 0.5rem;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        .detail-content {
            background: #0f172a;
            border: 1px solid #334155;
            border-radius: 6px;
            padding: 1rem;
            color: #e2e8f0;
            line-height: 1.5;
            white-space: pre-wrap;
        }
        .detail-meta {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 1rem;
            margin-top: 2rem;
            padding-top: 1.5rem;
            border-top: 1px solid #334155;
            color: #94a3b8;
            font-size: 0.9rem;
        }
        .detail-meta p {
            margin: 0;
            display: flex;
            align-items: center;
            gap: 0.5rem;
        }
        .detail-actions {
            display: flex;
            gap: 0.75rem;
            margin-top: 2rem;
        }
    `;
    document.head.appendChild(style);
});