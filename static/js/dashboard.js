function dashboard() {
    return {
        loading: true,
        lastUpdated: '',
        tenantName: '',
        userName: '',
        metrics: {
            totalUnpaid: 'KES 0',
            monthlyRevenue: 'KES 0',
            activeClients: '0',
            kraSuccessRate: '0'
        },
        recentInvoices: [],

        async init() {
            // Check auth first
            if (!InvoiceFastAuth.isAuthenticated()) {
                window.location.href = '/login.html';
                return;
            }

            // Set default time
            const now = new Date();
            this.lastUpdated = now.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });

            // Fetch user and dashboard data
            await this.fetchUser();
            await this.fetchDashboard();
        },

        async fetchUser() {
            const token = InvoiceFastAuth.getAccessToken();
            if (!token) return;

            try {
                const response = await fetch('/api/v1/tenant/me', {
                    headers: {
                        'Authorization': 'Bearer ' + token,
                        'Content-Type': 'application/json'
                    }
                });

                if (response.status === 401) {
                    InvoiceFastAuth.clearTokens();
                    window.location.href = '/login.html';
                    return;
                }

                const user = await response.json();
                this.userName = user.name || 'User';
                this.tenantName = user.companyName || 'My Business';
            } catch (e) {
                console.error('Failed to fetch user:', e);
            }
        },

        async fetchDashboard() {
            const token = InvoiceFastAuth.getAccessToken();
            if (!token) return;

            this.loading = true;

            try {
                const response = await fetch('/api/v1/tenant/dashboard', {
                    headers: {
                        'Authorization': 'Bearer ' + token,
                        'Content-Type': 'application/json'
                    }
                });

                if (response.ok) {
                    const data = await response.json();
                    
                    // Update metrics
                    if (data.stats) {
                        this.metrics = {
                            totalUnpaid: data.stats.totalUnpaid || 'KES 0',
                            monthlyRevenue: data.stats.monthlyRevenue || 'KES 0',
                            activeClients: data.stats.activeClients || '0',
                            kraSuccessRate: data.stats.kraSuccessRate || '0'
                        };
                    }

                    // Update recent invoices
                    if (data.recentInvoices) {
                        this.recentInvoices = data.recentInvoices;
                    }
                }

                // Update time
                const now = new Date();
                this.lastUpdated = now.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
            } catch (e) {
                console.error('Failed to fetch dashboard:', e);
            } finally {
                this.loading = false;
            }
        },

        async logout() {
            await InvoiceFastAuth.logout();
        }
    };
}