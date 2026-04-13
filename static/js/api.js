// InvoiceFast API Module
const InvoiceFastAPI = {
    baseURL: '/api/v1',
    
    // Get auth token from localStorage
    getToken() {
        return localStorage.getItem('accessToken');
    },
    
    // Check if user is authenticated
    isAuthenticated() {
        return !!this.getToken();
    },
    
    // Make authenticated request
    async request(endpoint, options = {}) {
        const token = this.getToken();
        if (!token && !options.public) {
            throw new Error('Not authenticated');
        }
        
        const headers = {
            'Content-Type': 'application/json',
            ...options.headers,
        };
        
        if (token) {
            headers['Authorization'] = 'Bearer ' + token;
        }
        
        const res = await fetch(this.baseURL + endpoint, {
            ...options,
            headers,
        });
        
        // Handle 401 - try refresh
        if (res.status === 401) {
            const refreshed = await this.refreshToken();
            if (refreshed) {
                return this.request(endpoint, options);
            }
            this.logout();
            throw new Error('Session expired');
        }
        
        if (!res.ok) {
            const data = await res.json().catch(() => ({}));
            throw new Error(data.error || 'Request failed');
        }
        
        return res.json();
    },
    
    // Auth endpoints
    auth: {
        async login(email, password) {
            const res = await fetch('/api/v1/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password }),
            });
            const data = await res.json();
            if (!res.ok) throw new Error(data.error || 'Login failed');
            return data;
        },
        
        async register(data) {
            const res = await fetch('/api/v1/auth/register', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data),
            });
            const result = await res.json();
            if (!res.ok) throw new Error(result.error || 'Registration failed');
            return result;
        },
        
        async refresh() {
            const refreshToken = localStorage.getItem('refreshToken');
            if (!refreshToken) return false;
            
            const res = await fetch('/api/v1/auth/refresh', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refresh_token: refreshToken }),
            });
            
            if (!res.ok) return false;
            
            const data = await res.json();
            if (data.access_token) {
                localStorage.setItem('accessToken', data.access_token);
                localStorage.setItem('refreshToken', data.refresh_token);
                return true;
            }
            return false;
        },
    },
    
    // Refresh token and retry
    async refreshToken() {
        return this.auth.refresh();
    },
    
    // Logout
    logout() {
        localStorage.removeItem('accessToken');
        localStorage.removeItem('refreshToken');
        localStorage.removeItem('user');
        window.location.href = '/login';
    },
    
    // Dashboard
    dashboard: {
        // Main endpoints
        async get(period = 'month') {
            return InvoiceFastAPI.request('/tenant/dashboard?period=' + period);
        },
        
        async getSummary(period = 'month') {
            return InvoiceFastAPI.request('/tenant/dashboard/summary?period=' + period);
        },
        
        async getStats(period = 'month') {
            return InvoiceFastAPI.request('/tenant/dashboard/stats?period=' + period);
        },
        
        // Recent data
        async getRecentInvoices(limit = 10) {
            return InvoiceFastAPI.request('/tenant/dashboard/invoices?limit=' + limit);
        },
        
        async getRecentClients(limit = 10) {
            return InvoiceFastAPI.request('/tenant/dashboard/clients?limit=' + limit);
        },
        
        // Charts
        async getRevenueChart(period = 'month') {
            return InvoiceFastAPI.request('/tenant/dashboard/charts/revenue?period=' + period);
        },
        
        async getStatusChart() {
            return InvoiceFastAPI.request('/tenant/dashboard/charts/status');
        },
        
        async getClientChart() {
            return InvoiceFastAPI.request('/tenant/dashboard/charts/clients');
        },
        
        // Advanced analytics
        async getRevenueTrend(months = 12) {
            return InvoiceFastAPI.request('/tenant/dashboard/trend/revenue?months=' + months);
        },
        
        async getDailyTrend(period = 'month') {
            return InvoiceFastAPI.request('/tenant/dashboard/trend/daily?period=' + period);
        },
        
        async getTopClients(limit = 5) {
            return InvoiceFastAPI.request('/tenant/dashboard/top-clients?limit=' + limit);
        },
        
        async getActivity(limit = 10) {
            return InvoiceFastAPI.request('/tenant/dashboard/activity?limit=' + limit);
        },
    },
    
    // Invoices
    invoices: {
        async list(filters = {}) {
            const params = new URLSearchParams(filters).toString();
            return InvoiceFastAPI.request('/tenant/invoices' + (params ? '?' + params : ''));
        },
        
        async get(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id);
        },
        
        async create(data) {
            return InvoiceFastAPI.request('/tenant/invoices', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async update(id, data) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async send(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/send', {
                method: 'POST',
            });
        },
        
        async cancel(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/cancel', {
                method: 'POST',
            });
        },
        
        async delete(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id, {
                method: 'DELETE',
            });
        },
        
        async remind(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/remind', {
                method: 'POST',
            });
        },
        
        async requestPayment(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/payment-request', {
                method: 'POST',
            });
        },
        
        async getPdf(id) {
            const token = InvoiceFastAPI.getToken();
            if (token) {
                window.location.href = '/api/v1/tenant/invoices/' + id + '/pdf';
            }
        },
        
        async duplicate(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/duplicate', {
                method: 'POST',
            });
        },
    },
    
    // Clients
    clients: {
        async list(filters = {}) {
            const params = new URLSearchParams(filters).toString();
            return InvoiceFastAPI.request('/tenant/clients' + (params ? '?' + params : ''));
        },
        
        async get(id) {
            return InvoiceFastAPI.request('/tenant/clients/' + id);
        },
        
        async create(data) {
            return InvoiceFastAPI.request('/tenant/clients', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async update(id, data) {
            return InvoiceFastAPI.request('/tenant/clients/' + id, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async delete(id) {
            return InvoiceFastAPI.request('/tenant/clients/' + id, {
                method: 'DELETE',
            });
        },
    },
    
    // Payments
    payments: {
        async list(filters = {}) {
            const params = new URLSearchParams(filters).toString();
            return InvoiceFastAPI.request('/tenant/payments' + (params ? '?' + params : ''));
        },
        
        async get(id) {
            return InvoiceFastAPI.request('/tenant/payments/' + id);
        },
        
        async requestPayment(data) {
            return InvoiceFastAPI.request('/tenant/payments/request', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async refund(id, reason) {
            return InvoiceFastAPI.request('/tenant/payments/' + id + '/refund', {
                method: 'POST',
                body: JSON.stringify({ reason }),
            });
        },
        
        async reconcile(id) {
            return InvoiceFastAPI.request('/tenant/payments/' + id + '/reconcile', {
                method: 'POST',
            });
        },
        
        async getReceipt(id) {
            const token = InvoiceFastAPI.getToken();
            if (token) {
                window.location.href = '/api/v1/tenant/payments/' + id + '/receipt';
            }
        },
        
        async resendReceipt(id) {
            return InvoiceFastAPI.request('/tenant/payments/' + id + '/receipt', {
                method: 'POST',
            });
        },
    },
    
    // Reports
    reports: {
        async getOverview(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/overview?period=' + period);
        },
        
        async getRevenue(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/revenue?period=' + period);
        },
        
        async getInvoices(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/invoices?period=' + period);
        },
        
        async getPayments(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/payments?period=' + period);
        },
        
        async getClients(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/clients?period=' + period);
        },
        
        async getTax(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/tax?period=' + period);
        },
        
        async export(format = 'pdf', period = '30') {
            const token = InvoiceFastAPI.getToken();
            if (token) {
                window.location.href = '/api/v1/tenant/reports/export?format=' + format + '&period=' + period;
            }
        },
    },
    
    // Settings
    settings: {
        async get() {
            return InvoiceFastAPI.request('/tenant/settings/');
        },
        
        async save(data) {
            return InvoiceFastAPI.request('/tenant/settings/', {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async getMpesa() {
            return InvoiceFastAPI.request('/tenant/settings/mpesa');
        },
        
        async saveMpesa(data) {
            return InvoiceFastAPI.request('/tenant/settings/mpesa', {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async getKRA() {
            return InvoiceFastAPI.request('/tenant/settings/kra');
        },
        
        async saveKRA(data) {
            return InvoiceFastAPI.request('/tenant/settings/kra', {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async getNotifications() {
            return InvoiceFastAPI.request('/tenant/settings/notifications');
        },
        
        async saveNotifications(data) {
            return InvoiceFastAPI.request('/tenant/settings/notifications', {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
    },
    
    // Team
    team: {
        async getMembers() {
            return InvoiceFastAPI.request('/tenant/team/members');
        },
        
        async invite(data) {
            return InvoiceFastAPI.request('/tenant/team/invite', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async removeMember(id) {
            return InvoiceFastAPI.request('/tenant/team/member/' + id, {
                method: 'DELETE',
            });
        },
        
        async updateRole(id, data) {
            return InvoiceFastAPI.request('/tenant/team/member/' + id + '/role', {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async getInvitations() {
            return InvoiceFastAPI.request('/tenant/team/invitations');
        },
        
        async cancelInvitation(id) {
            return InvoiceFastAPI.request('/tenant/team/invitation/' + id, {
                method: 'DELETE',
            });
        },
    },
    
    // Billing
    billing: {
        async getSubscription() {
            return InvoiceFastAPI.request('/tenant/billing/subscription');
        },
        
        async getPaymentMethods() {
            return InvoiceFastAPI.request('/tenant/billing/payment-methods');
        },
        
        async getHistory() {
            return InvoiceFastAPI.request('/tenant/billing/history');
        },
        
        async createCheckoutSession(planId) {
            return InvoiceFastAPI.request('/tenant/billing/checkout', {
                method: 'POST',
                body: JSON.stringify({ plan_id: planId }),
            });
        },
    },
    
    // Automations
    automations: {
        async getAll() {
            return InvoiceFastAPI.request('/tenant/automations');
        },
        
        async get(id) {
            return InvoiceFastAPI.request('/tenant/automations/' + id);
        },
        
        async create(data) {
            return InvoiceFastAPI.request('/tenant/automations', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async update(id, data) {
            return InvoiceFastAPI.request('/tenant/automations/' + id, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async delete(id) {
            return InvoiceFastAPI.request('/tenant/automations/' + id, {
                method: 'DELETE',
            });
        },
        
        async run(id) {
            return InvoiceFastAPI.request('/tenant/automations/' + id + '/run', {
                method: 'POST',
            });
        },
        
        async getLogs(id) {
            return InvoiceFastAPI.request('/tenant/automations/' + id + '/logs');
        },
    },
    
    // User/Tenant
    user: {
        async getProfile() {
            return InvoiceFastAPI.request('/tenant/me');
        },
        
        async updateProfile(data) {
            return InvoiceFastAPI.request('/tenant/me', {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async changePassword(currentPassword, newPassword) {
            return InvoiceFastAPI.request('/tenant/change-password', {
                method: 'POST',
                body: JSON.stringify({
                    current_password: currentPassword,
                    new_password: newPassword,
                }),
            });
        },
    },
};

// Format currency
function formatCurrency(amount, currency = 'KES') {
    return new Intl.NumberFormat('en-KE', {
        style: 'currency',
        currency: currency,
    }).format(amount || 0);
}

// Format date
function formatDate(date) {
    if (!date) return '-';
    return new Date(date).toLocaleDateString('en-KE', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
    });
}

// Format status badge
function formatStatus(status) {
    const statusMap = {
        draft: { class: 'bg-gray-100 text-gray-700', label: 'Draft' },
        sent: { class: 'bg-blue-100 text-blue-700', label: 'Sent' },
        viewed: { class: 'bg-indigo-100 text-indigo-700', label: 'Viewed' },
        paid: { class: 'bg-green-100 text-green-700', label: 'Paid' },
        partially_paid: { class: 'bg-yellow-100 text-yellow-700', label: 'Partial' },
        overdue: { class: 'bg-red-100 text-red-700', label: 'Overdue' },
        cancelled: { class: 'bg-slate-100 text-slate-700', label: 'Cancelled' },
    };
    
    const s = statusMap[status] || { class: 'bg-gray-100 text-gray-700', label: status };
    return `<span class="px-2 py-1 rounded-full text-xs font-medium ${s.class}">${s.label}</span>`;
}

// Check auth on page load
function requireAuth() {
    if (!InvoiceFastAPI.isAuthenticated()) {
        window.location.href = '/login?redirect=' + encodeURIComponent(window.location.pathname);
        return false;
    }
    return true;
}

// Get user from localStorage
function getUser() {
    try {
        return JSON.parse(localStorage.getItem('user'));
    } catch {
        return null;
    }
}