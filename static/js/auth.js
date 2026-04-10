<script>
// Common authentication utilities for InvoiceFast
(function() {
    'use strict';
    
    // Token storage keys
    const ACCESS_TOKEN_KEY = 'accessToken';
    const REFRESH_TOKEN_KEY = 'refreshToken';
    
    // Get tokens from storage
    function getAccessToken() {
        return localStorage.getItem(ACCESS_TOKEN_KEY);
    }
    
    function getRefreshToken() {
        return localStorage.getItem(REFRESH_TOKEN_KEY);
    }
    
    // Store tokens
    function setTokens(accessToken, refreshToken) {
        if (accessToken) localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
        if (refreshToken) localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
    }
    
    // Clear tokens (logout)
    function clearTokens() {
        localStorage.removeItem(ACCESS_TOKEN_KEY);
        localStorage.removeItem(REFRESH_TOKEN_KEY);
    }
    
    // Check if authenticated
    function isAuthenticated() {
        return !!getAccessToken();
    }
    
    // Show/hide based on auth state
    function updateAuthUI() {
        const authenticated = isAuthenticated();
        
        // Update elements with data-auth attribute
        document.querySelectorAll('[data-auth="authenticated"]').forEach(el => {
            el.style.display = authenticated ? '' : 'none';
        });
        
        document.querySelectorAll('[data-auth="anonymous"]').forEach(el => {
            el.style.display = authenticated ? 'none' : '';
        });
    }
    
    // Make authenticated API call
    async function apiCall(url, options = {}) {
        const token = getAccessToken();
        
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers
        };
        
        if (token) {
            headers['Authorization'] = 'Bearer ' + token;
        }
        
        const response = await fetch(url, {
            ...options,
            headers
        });
        
        // Handle token refresh on 401
        if (response.status === 401 && getRefreshToken()) {
            const refreshToken = getRefreshToken();
            const refreshResponse = await fetch('/api/v1/auth/refresh', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refresh_token: refreshToken })
            });
            
            if (refreshResponse.ok) {
                const data = await refreshResponse.json();
                setTokens(data.access_token, data.refresh_token);
                
                // Retry original request
                headers['Authorization'] = 'Bearer ' + data.access_token;
                return fetch(url, {
                    ...options,
                    headers
                });
            }
            
            // Refresh failed, clear tokens
            clearTokens();
            window.location.href = '/login.html';
        }
        
        return response;
    }
    
    // Logout function
    async function logout() {
        const token = getRefreshToken();
        if (token) {
            try {
                await fetch('/api/v1/tenant/logout', {
                    method: 'POST',
                    headers: { 
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + getAccessToken()
                    },
                    body: JSON.stringify({ refresh_token: token })
                });
            } catch(e) {
                // Ignore errors
            }
        }
        clearTokens();
        window.location.href = '/login.html';
    }
    
    // Initialize on page load
    function init() {
        // Add token to all HTMX requests
        document.body.addEventListener('htmx:configRequest', function(evt) {
            const token = getAccessToken();
            if (token) {
                evt.detail.headers.set('Authorization', 'Bearer ' + token);
            }
        });
        
        // Handle auth errors
        document.body.addEventListener('htmx:afterSwap', function(evt) {
            // Check for auth error in response
            const errEl = evt.detail.target.querySelector('[data-auth-error]');
            if (errEl && errEl.dataset.authError === 'true') {
                clearTokens();
                window.location.href = '/login.html';
            }
        });
        
        // Check auth on protected pages
        if (window.location.pathname.startsWith('/dashboard')) {
            if (!isAuthenticated()) {
                window.location.href = '/login.html';
                return;
            }
            
            // Verify token is valid
            apiCall('/api/v1/tenant/me').then(res => {
                if (res.status === 401) {
                    clearTokens();
                    window.location.href = '/login.html';
                }
            }).catch(() => {
                clearTokens();
                window.location.href = '/login.html';
            });
        }
        
        updateAuthUI();
    }
    
    // Run on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
    
    // Export to window for global access
    window.InvoiceFastAuth = {
        getAccessToken,
        getRefreshToken,
        setTokens,
        clearTokens,
        isAuthenticated,
        apiCall,
        logout,
        updateAuthUI
    };
})();