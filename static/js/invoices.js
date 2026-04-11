function invoices() {
    return {
        loading: true,
        invoices: [],
        total: 0,
        search: '',
        statusFilter: '',
        offset: 0,
        limit: 20,
        selectedInvoice: null,
        showCreateModal: false,

        async init() {
            if (!InvoiceFastAuth.isAuthenticated()) {
                window.location.href = '/login.html';
                return;
            }
            await this.fetchInvoices();
        },

        async fetchInvoices() {
            this.loading = true;
            const token = InvoiceFastAuth.getAccessToken();

            const params = new URLSearchParams({
                offset: this.offset,
                limit: this.limit
            });
            if (this.search) params.append('search', this.search);
            if (this.statusFilter) params.append('status', this.statusFilter);

            try {
                const response = await fetch(`/api/v1/invoices?${params}`, {
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

                const data = await response.json();
                this.invoices = data.invoices || [];
                this.total = data.total || 0;
            } catch (e) {
                console.error('Failed to fetch invoices:', e);
            } finally {
                this.loading = false;
            }
        },

        getStatusClass(status) {
            const classes = {
                'draft': 'bg-gray-100 text-gray-700',
                'pending': 'bg-yellow-100 text-yellow-700',
                'paid': 'bg-green-100 text-green-700',
                'overdue': 'bg-red-100 text-red-700'
            };
            return classes[status] || 'bg-gray-100 text-gray-700';
        },

        async viewInvoice(invoice) {
            this.selectedInvoice = invoice;
            // Could open a modal or redirect to detail page
        },

        async sendInvoice(invoice) {
            const token = InvoiceFastAuth.getAccessToken();
            try {
                const response = await fetch(`/api/v1/invoices/${invoice.id}/send`, {
                    method: 'POST',
                    headers: {
                        'Authorization': 'Bearer ' + token
                    }
                });
                if (response.ok) {
                    alert('Invoice sent successfully');
                }
            } catch (e) {
                console.error('Failed to send invoice:', e);
            }
        },

        async prevPage() {
            this.offset = Math.max(0, this.offset - this.limit);
            await this.fetchInvoices();
        },

        async nextPage() {
            this.offset += this.limit;
            await this.fetchInvoices();
        },

        async logout() {
            await InvoiceFastAuth.logout();
        }
    };
}