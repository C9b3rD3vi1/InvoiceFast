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
        
        // Handle empty responses
        const text = await res.text();
        if (!text) return null;
        try {
            return JSON.parse(text);
        } catch {
            return text;
        }
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
            return InvoiceFastAPI.request('/tenant/activity?limit=' + limit);
        },
    },
    
    // Activity Feed
    activity: {
        async getRecent(limit = 20) {
            return InvoiceFastAPI.request('/tenant/activity?limit=' + limit);
        },
    },
    
    // Item Library
    itemLibrary: {
        async list() {
            return InvoiceFastAPI.request('/tenant/items');
        },
        
        async get(id) {
            return InvoiceFastAPI.request('/tenant/items/' + id);
        },
        
        async create(data) {
            return InvoiceFastAPI.request('/tenant/items', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async update(id, data) {
            return InvoiceFastAPI.request('/tenant/items/' + id, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async delete(id) {
            return InvoiceFastAPI.request('/tenant/items/' + id, {
                method: 'DELETE',
            });
        },
    },
    
    // Recurring Invoices
    recurring: {
        async list() {
            return InvoiceFastAPI.request('/tenant/recurring');
        },
        
        async enable(invoiceID, frequency) {
            return InvoiceFastAPI.request('/tenant/recurring/' + invoiceID + '/enable', {
                method: 'POST',
                body: JSON.stringify({ frequency }),
            });
        },
        
        async disable(invoiceID) {
            return InvoiceFastAPI.request('/tenant/recurring/' + invoiceID + '/disable', {
                method: 'POST',
            });
        },
        
        async process() {
            return InvoiceFastAPI.request('/tenant/recurring/process', {
                method: 'POST',
            });
        },
    },
    
    // Late Fees
    lateFees: {
        async getConfig() {
            return InvoiceFastAPI.request('/tenant/late-fees/config');
        },
        
        async updateConfig(data) {
            return InvoiceFastAPI.request('/tenant/late-fees/config', {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async calculate(invoiceID) {
            return InvoiceFastAPI.request('/tenant/late-fees/invoice/' + invoiceID + '/calculate');
        },
        
        async saveConfig(data) {
            return InvoiceFastAPI.request('/tenant/late-fees/config', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async getInvoiceFees(invoiceID) {
            return InvoiceFastAPI.request('/tenant/late-fees/invoice/' + invoiceID);
        },
        
        async waive(lateFeeID) {
            return InvoiceFastAPI.request('/tenant/late-fees/' + lateFeeID + '/waive', {
                method: 'POST',
            });
        },
    },
    
    // Expenses
    expenses: {
        async list(filters = {}) {
            const params = new URLSearchParams(filters).toString();
            return InvoiceFastAPI.request('/tenant/expenses' + (params ? '?' + params : ''));
        },
        
        async get(id) {
            return InvoiceFastAPI.request('/tenant/expenses/' + id);
        },
        
        async create(data) {
            return InvoiceFastAPI.request('/tenant/expenses', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async update(id, data) {
            return InvoiceFastAPI.request('/tenant/expenses/' + id, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async delete(id) {
            return InvoiceFastAPI.request('/tenant/expenses/' + id, {
                method: 'DELETE',
            });
        },
        
        async getCategories() {
            return InvoiceFastAPI.request('/tenant/expenses/categories');
        },
        
        async createCategory(data) {
            return InvoiceFastAPI.request('/tenant/expenses/categories', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async getSummary(startDate, endDate) {
            let params = '';
            if (startDate || endDate) {
                params = '?';
                if (startDate) params += 'start_date=' + startDate;
                if (endDate) params += (startDate ? '&' : '') + 'end_date=' + endDate;
            }
            return InvoiceFastAPI.request('/tenant/expenses/summary' + params);
        },
        
        async uploadAttachment(expenseId, file) {
            const formData = new FormData();
            formData.append('file', file);
            const token = InvoiceFastAPI.getToken();
            const response = await fetch('/api/v1/tenant/expenses/' + expenseId + '/attachments', {
                method: 'POST',
                headers: { 'Authorization': 'Bearer ' + token },
                body: formData,
            });
            if (!response.ok) throw new Error('Failed to upload attachment');
            return response.json();
        },
    },
    
    // Reminder Sequences
    reminderSequences: {
        async list() {
            return InvoiceFastAPI.request('/tenant/reminder-sequences');
        },
        
        async create(data) {
            return InvoiceFastAPI.request('/tenant/reminder-sequences', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async update(id, data) {
            return InvoiceFastAPI.request('/tenant/reminder-sequences/' + id, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        
        async delete(id) {
            return InvoiceFastAPI.request('/tenant/reminder-sequences/' + id, {
                method: 'DELETE',
            });
        },
    },
    
    // Bulk Actions
    bulk: {
        async sendOverdueReminders() {
            return InvoiceFastAPI.request('/tenant/bulk/overdue-reminders', {
                method: 'POST',
            });
        },
    },
    
    // Payment Matching
    paymentMatching: {
        async getUnallocated() {
            return InvoiceFastAPI.request('/tenant/payments/unallocated');
        },
        
        async match(paymentID, invoiceID) {
            return InvoiceFastAPI.request('/tenant/payments/' + paymentID + '/match', {
                method: 'POST',
                body: JSON.stringify({ invoice_id: invoiceID }),
            });
        },
        
        async manualMatch(data) {
            return InvoiceFastAPI.request('/tenant/payments/manual-match', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
    },
    
    // Settlement Reports
    settlement: {
        async getDaily(date) {
            const params = date ? '?date=' + date : '';
            return InvoiceFastAPI.request('/tenant/settlement/daily' + params);
        },
        
        async export(date) {
            const token = InvoiceFastAPI.getToken();
            if (token) {
                const params = date ? '?date=' + date : '';
                window.location.href = '/api/v1/tenant/settlement/export' + params;
            }
        },
    },
    
    // Reports - Extended
    reports: {
        async getDashboard(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/dashboard?period=' + period);
        },
        
        async getOverview(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/overview?period=' + period);
        },
        
        async getRevenue(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/revenue?period=' + period);
        },
        
        async getProfit(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/profit?period=' + period);
        },
        
        async getCashFlow(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/cashflow?period=' + period);
        },
        
        async getExpenses(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/expenses?period=' + period);
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
        
        async getVAT(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/vat?period=' + period);
        },
        
        async getAging() {
            return InvoiceFastAPI.request('/tenant/reports/aging');
        },
        
        async getAgingDetailed() {
            return InvoiceFastAPI.request('/tenant/reports/aging-detailed');
        },
        
        async getIncomeStatement(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/income-statement?period=' + period);
        },
        
        async getClientStatement(clientID, startDate, endDate) {
            let params = '?client_id=' + clientID;
            if (startDate) params += '&start_date=' + startDate;
            if (endDate) params += '&end_date=' + endDate;
            return InvoiceFastAPI.request('/tenant/reports/client/' + clientID + '/statement' + params);
        },

        async getClientRevenue(period = '30', limit = 10) {
            return InvoiceFastAPI.request('/tenant/reports/clients/revenue?period=' + period + '&limit=' + limit);
        },

        async getFraudRisk(period = '30') {
            return InvoiceFastAPI.request('/tenant/reports/fraud?period=' + period);
        },

        async getPaymentVerification(invoiceID) {
            return InvoiceFastAPI.request('/tenant/reports/verification/' + invoiceID);
        },

        async exportData(format = 'csv', reportType = 'overview', period = '30') {
            const token = InvoiceFastAPI.getToken();
            if (token) {
                window.location.href = '/api/v1/tenant/reports/export?format=' + format + '&type=' + reportType + '&period=' + period;
            }
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
        
        async sendWhatsApp(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/whatsapp', {
                method: 'POST',
            });
        },
        
        async submitToKRA(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/kra/submit', {
                method: 'POST',
            });
        },
        
        async getKRAStats() {
            return InvoiceFastAPI.request('/tenant/invoices/kra-stats');
        },
        
        async getKRAActivity(limit = 50) {
            return InvoiceFastAPI.request('/tenant/invoices/kra/activity?limit=' + limit);
        },
        
        async submitAllPendingToKRA() {
            return InvoiceFastAPI.request('/tenant/invoices/kra/submit-all', { method: 'POST' });
        },
        
        async requestPayment(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/payment-request', {
                method: 'POST',
            });
        },
        
        async getPdf(id) {
            const token = this.getToken() || localStorage.getItem('token');
            if (!token) {
                throw new Error('No token available');
            }
            try {
                console.log('Downloading PDF for invoice:', id);
                const response = await fetch('/api/v1/tenant/invoices/' + id + '/pdf?token=' + encodeURIComponent(token));
                console.log('Response status:', response.status);
                console.log('Response headers:', response.headers.get('content-type'));
                
                if (!response.ok) {
                    const text = await response.text();
                    console.error('Download error response:', text);
                    throw new Error(text || 'Download failed with status ' + response.status);
                }
                
                const blob = await response.blob();
                console.log('Blob size:', blob.size, 'Blob type:', blob.type);
                
                if (blob.size === 0) {
                    throw new Error('Downloaded file is empty');
                }
                
                const url = window.URL.createObjectURL(blob);
                const a = document.createElement('a');
                a.href = url;
                a.download = 'invoice-' + id + '.pdf';
                document.body.appendChild(a);
                a.click();
                document.body.removeChild(a);
                setTimeout(() => window.URL.revokeObjectURL(url), 100);
                return true;
            } catch(err) {
                console.error('PDF download failed:', err);
                throw err;
            }
        },
        
        async duplicate(id) {
            return InvoiceFastAPI.request('/tenant/invoices/' + id + '/duplicate', {
                method: 'POST',
            });
        },
        
        async getAttachments(invoiceId) {
            return InvoiceFastAPI.request('/tenant/invoices/' + invoiceId + '/attachments');
        },
        
        async uploadAttachment(invoiceId, file) {
            const formData = new FormData();
            formData.append('file', file);
            const token = InvoiceFastAPI.getToken();
            const response = await fetch('/api/v1/tenant/invoices/' + invoiceId + '/attachments', {
                method: 'POST',
                headers: { 'Authorization': 'Bearer ' + token },
                body: formData,
            });
            if (!response.ok) throw new Error('Failed to upload attachment');
            return response.json();
        },
        
        async deleteAttachment(invoiceId, attachmentId) {
            return InvoiceFastAPI.request('/tenant/invoices/' + invoiceId + '/attachments/' + attachmentId, {
                method: 'DELETE',
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
        
        async getStatement(clientID, startDate, endDate) {
            let params = '';
            if (startDate) params = '?start_date=' + startDate;
            if (endDate) params += '&end_date=' + endDate;
            return InvoiceFastAPI.request('/tenant/reports/client/' + clientID + '/statement' + params);
        },
        
        async getInvoices(clientID, limit = 20) {
            return InvoiceFastAPI.request('/tenant/clients/' + clientID + '/invoices?limit=' + limit);
        },
        
        async getPayments(clientID, limit = 20) {
            return InvoiceFastAPI.request('/tenant/clients/' + clientID + '/payments?limit=' + limit);
        },
        
        async getActivity(clientID, limit = 20) {
            return InvoiceFastAPI.request('/tenant/clients/' + clientID + '/activity?limit=' + limit);
        },
        
        async getStats(clientID) {
            return InvoiceFastAPI.request('/tenant/clients/' + clientID + '/stats', { method: 'POST' });
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
        
        async stats() {
            return InvoiceFastAPI.request('/tenant/payments/stats');
        },
        
        async reconcile() {
            return InvoiceFastAPI.request('/tenant/payments/reconcile', {
                method: 'POST',
            });
        },
        
        async create(data) {
            return InvoiceFastAPI.request('/tenant/payments', {
                method: 'POST',
                body: JSON.stringify(data),
            });
        },
        
        async request(data) {
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
        
        async createPayment(invoiceId, data) {
            return InvoiceFastAPI.request('/tenant/invoices/' + invoiceId + '/payments', {
                method: 'POST',
                body: JSON.stringify(data),
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
        
        async getUnmatched() {
            return InvoiceFastAPI.request('/tenant/payments/unmatched');
        },
        
        async getUnpaidInvoices() {
            return InvoiceFastAPI.request('/tenant/invoices?status=sent,viewed,partially_paid,overdue&unpaid=true');
        },
        
        async matchPayment(paymentId, invoiceId, amount) {
            return InvoiceFastAPI.request('/tenant/payments/' + paymentId + '/match', {
                method: 'POST',
                body: JSON.stringify({ invoice_id: invoiceId, amount }),
            });
        },
        
        async autoMatch() {
            return InvoiceFastAPI.request('/tenant/payments/auto-match', {
                method: 'POST',
            });
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
        
        async updateBusiness(data) {
            return InvoiceFastAPI.request('/tenant/settings/', {
                method: 'PUT',
                body: JSON.stringify({ branding: data }),
            });
        },
        
        async updateProfile(data) {
            return InvoiceFastAPI.request('/tenant/settings/', {
                method: 'PUT',
                body: JSON.stringify({ profile: data }),
            });
        },
        
        async updateInvoice(data) {
            return InvoiceFastAPI.request('/tenant/settings/', {
                method: 'PUT',
                body: JSON.stringify({ invoice: data }),
            });
        },
        
        async updatePayments(data) {
            return InvoiceFastAPI.request('/tenant/settings/', {
                method: 'PUT',
                body: JSON.stringify({ payments: data }),
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

        async updateNotifications(data) {
            return InvoiceFastAPI.request('/tenant/settings/notifications', {
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

        async setup2FA() {
            return InvoiceFastAPI.request('/tenant/2fa/setup', { method: 'POST' });
        },
        async verify2FA(code) {
            return InvoiceFastAPI.request('/tenant/2fa/verify', {
                method: 'POST',
                body: JSON.stringify({ code: code }),
            });
        },
        async disable2FA(password, code) {
            return InvoiceFastAPI.request('/tenant/2fa/disable', {
                method: 'POST',
                body: JSON.stringify({ password, code }),
            });
        },

        async getSessions() {
            return InvoiceFastAPI.request('/tenant/sessions');
        },
        async revokeSession(id) {
            return InvoiceFastAPI.request('/tenant/session/' + id, { method: 'DELETE' });
        },
        async revokeAllSessions() {
            return InvoiceFastAPI.request('/tenant/sessions/revoke-all', { method: 'POST' });
        },

        async getSecurityStatus() {
            return InvoiceFastAPI.request('/tenant/security-status');
        },
        async getLoginHistory(limit) {
            return InvoiceFastAPI.request('/tenant/login-history?limit=' + (limit || 20));
        },
        async updateLoginAlerts(enabled) {
            return InvoiceFastAPI.request('/tenant/login-alerts', {
                method: 'PUT',
                body: JSON.stringify({ enabled }),
            });
        },
    },

    // Notification Preferences
    notifications: {
        async getPreferences() {
            return InvoiceFastAPI.request('/tenant/notifications/preferences');
        },
        async updatePreferences(data) {
            return InvoiceFastAPI.request('/tenant/notifications/preferences', {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },
        async getTemplates() {
            return InvoiceFastAPI.request('/tenant/notifications/templates');
        },
        async getLogs(status) {
            const params = status ? '?status=' + status : '';
            return InvoiceFastAPI.request('/tenant/notifications/logs' + params);
        },
    },

    // Integrations
    integrations: {
        async getAll() {
            return InvoiceFastAPI.request('/tenant/integrations/');
        },

        async get(provider) {
            return InvoiceFastAPI.request('/tenant/integrations/' + provider);
        },

        async getConfig(provider) {
            return InvoiceFastAPI.request('/tenant/integrations/' + provider + '/config');
        },

        async save(provider, data) {
            return InvoiceFastAPI.request('/tenant/integrations/' + provider, {
                method: 'PUT',
                body: JSON.stringify(data),
            });
        },

        async delete(id) {
            return InvoiceFastAPI.request('/tenant/integrations/' + id, {
                method: 'DELETE',
            });
        },

        async toggle(id, active) {
            return InvoiceFastAPI.request('/tenant/integrations/' + id + '/toggle', {
                method: 'POST',
                body: JSON.stringify({ active }),
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
    
    // Automations - Enterprise Edition
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
        
        // Recurring Invoices
        recurring: {
            async list() {
                return InvoiceFastAPI.request('/tenant/automations/recurring/');
            },
            async get(id) {
                return InvoiceFastAPI.request('/tenant/automations/recurring/' + id);
            },
            async create(data) {
                return InvoiceFastAPI.request('/tenant/automations/recurring/', {
                    method: 'POST',
                    body: JSON.stringify(data),
                });
            },
            async update(id, data) {
                return InvoiceFastAPI.request('/tenant/automations/recurring/' + id, {
                    method: 'PUT',
                    body: JSON.stringify(data),
                });
            },
            async pause(id) {
                return InvoiceFastAPI.request('/tenant/automations/recurring/' + id + '/pause', { method: 'POST' });
            },
            async resume(id) {
                return InvoiceFastAPI.request('/tenant/automations/recurring/' + id + '/resume', { method: 'POST' });
            },
            async delete(id) {
                return InvoiceFastAPI.request('/tenant/automations/recurring/' + id, { method: 'DELETE' });
            },
        },
        
        // Reminder Rules
        reminders: {
            async list() {
                return InvoiceFastAPI.request('/tenant/automations/reminders/');
            },
            async get(id) {
                return InvoiceFastAPI.request('/tenant/automations/reminders/' + id);
            },
            async create(data) {
                return InvoiceFastAPI.request('/tenant/automations/reminders/', {
                    method: 'POST',
                    body: JSON.stringify(data),
                });
            },
            async update(id, data) {
                return InvoiceFastAPI.request('/tenant/automations/reminders/' + id, {
                    method: 'PUT',
                    body: JSON.stringify(data),
                });
            },
            async delete(id) {
                return InvoiceFastAPI.request('/tenant/automations/reminders/' + id, { method: 'DELETE' });
            },
            async stats() {
                return InvoiceFastAPI.request('/tenant/automations/reminders/stats');
            },
        },
        
        // Workflows
        workflows: {
            async list() {
                return InvoiceFastAPI.request('/tenant/automations/workflows/');
            },
            async get(id) {
                return InvoiceFastAPI.request('/tenant/automations/workflows/' + id);
            },
            async create(data) {
                return InvoiceFastAPI.request('/tenant/automations/workflows/', {
                    method: 'POST',
                    body: JSON.stringify(data),
                });
            },
            async update(id, data) {
                return InvoiceFastAPI.request('/tenant/automations/workflows/' + id, {
                    method: 'PUT',
                    body: JSON.stringify(data),
                });
            },
            async delete(id) {
                return InvoiceFastAPI.request('/tenant/automations/workflows/' + id, { method: 'DELETE' });
            },
            async stats() {
                return InvoiceFastAPI.request('/tenant/automations/workflows/stats');
            },
        },
        
        // Job Queue
        jobs: {
            async list(status, limit = 50, offset = 0) {
                const params = new URLSearchParams({ limit, offset });
                if (status) params.set('status', status);
                return InvoiceFastAPI.request('/tenant/automations/jobs/?' + params);
            },
            async get(id) {
                return InvoiceFastAPI.request('/tenant/automations/jobs/' + id);
            },
            async stats() {
                return InvoiceFastAPI.request('/tenant/automations/jobs/stats');
            },
            async failed(limit = 50, offset = 0) {
                return InvoiceFastAPI.request(`/tenant/automations/jobs/failed?limit=${limit}&offset=${offset}`);
            },
            async recent(limit = 20) {
                return InvoiceFastAPI.request('/tenant/automations/jobs/recent?limit=' + limit);
            },
            async retry(id) {
                return InvoiceFastAPI.request('/tenant/automations/jobs/' + id + '/retry', { method: 'POST' });
            },
            async cancel(id) {
                return InvoiceFastAPI.request('/tenant/automations/jobs/' + id + '/cancel', { method: 'POST' });
            },
        },
        
        // Monitoring
        monitoring: {
            async stats() {
                return InvoiceFastAPI.request('/admin/automation/stats');
            },
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

        // 2FA Setup
        async setup2FA() {
            return InvoiceFastAPI.request('/tenant/2fa/setup', { method: 'POST' });
        },
        async verify2FA(code) {
            return InvoiceFastAPI.request('/tenant/2fa/verify', {
                method: 'POST',
                body: JSON.stringify({ code: code }),
            });
        },
        async disable2FA(password, code) {
            return InvoiceFastAPI.request('/tenant/2fa/disable', {
                method: 'POST',
                body: JSON.stringify({ password, code }),
            });
        },

        // Sessions
        async getSessions() {
            return InvoiceFastAPI.request('/tenant/sessions');
        },
        async revokeSession(id) {
            return InvoiceFastAPI.request('/tenant/session/' + id, { method: 'DELETE' });
        },
        async revokeAllSessions() {
            return InvoiceFastAPI.request('/tenant/sessions/revoke-all', { method: 'POST' });
        },

        // Security
        async getSecurityStatus() {
            return InvoiceFastAPI.request('/tenant/security-status');
        },
        async getLoginHistory(limit) {
            return InvoiceFastAPI.request('/tenant/login-history?limit=' + (limit || 20));
        },
        async updateLoginAlerts(enabled) {
            return InvoiceFastAPI.request('/tenant/login-alerts', {
                method: 'PUT',
                body: JSON.stringify({ enabled }),
            });
        },

        team: {
            getMembers() {
                return InvoiceFastAPI.request('/tenant/team/members');
            },
            invite(data) {
                return InvoiceFastAPI.request('/tenant/team/invite', {
                    method: 'POST',
                    body: JSON.stringify(data)
                });
            },
            remove(id) {
                return InvoiceFastAPI.request('/tenant/team/member/' + id, {
                    method: 'DELETE'
                });
            },
            updateRole(id, data) {
                return InvoiceFastAPI.request('/tenant/team/member/' + id + '/role', {
                    method: 'PUT',
                    body: JSON.stringify(data)
                });
            },
            getInvitations() {
                return InvoiceFastAPI.request('/tenant/team/invitations');
            },
            cancelInvitation(id) {
                return InvoiceFastAPI.request('/tenant/team/invitation/' + id, {
                    method: 'DELETE'
                });
            }
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