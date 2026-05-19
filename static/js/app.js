// InvoiceFast - Main JavaScript

document.addEventListener('alpine:init', () => {
    // Billing Content Component
    Alpine.data('billingContent', () => ({
        loading: true,
        subscriptionStatus: 'none',
        trialDaysRemaining: 0,
        currentPlan: null,
        subscription: null,
        tenantCurrency: 'KES',
        exchangeRates: {},
        usageMetrics: [],
        usageWarning: null,
        paymentMethods: [],
        transactions: [],
        plans: [],
        upgradeModalOpen: false,
        paymentMethodModalOpen: false,
        newPaymentMethod: 'card',
        historyModalOpen: false,
        selectedPlan: null,
        paymentMethod: 'stripe',
        mpesaPhone: '',
        successModalOpen: false,
        successMessage: '',
        failureModalOpen: false,
        failureMessage: '',
        
        async init() {
            await this.loadData();
            await this.loadPlans();
        },
        
        async loadData() {
            this.loading = true;
            try {
                const subData = await InvoiceFastAPI.billing.getSubscription();
                const methodsData = await InvoiceFastAPI.billing.getPaymentMethods();
                const historyData = await InvoiceFastAPI.billing.getHistory();
                
                // Get tenant currency from API response
                this.tenantCurrency = subData?.currency || 'KES';
                
                // Load exchange rates for currency conversion
                await this.loadExchangeRates();
                
                // Determine subscription status
                const sub = subData?.subscription;
                if (!sub) {
                    this.subscriptionStatus = 'none';
                } else if (sub.status === 'trialing') {
                    this.subscriptionStatus = 'trialing';
                    this.calculateTrialDays(sub);
                } else if (sub.status === 'active') {
                    this.subscriptionStatus = 'active';
                } else if (sub.status === 'past_due') {
                    this.subscriptionStatus = 'past_due';
                } else if (sub.status === 'canceled' || sub.status === 'expired') {
                    this.subscriptionStatus = 'expired';
                } else {
                    this.subscriptionStatus = 'none';
                }
                
                this.currentPlan = subData?.plan || null;
                this.subscription = subData?.subscription || null;
                this.paymentMethods = methodsData || [];
                this.transactions = historyData?.transactions || [];
                
                this.calculateUsageMetrics(subData);
            } catch (error) {
                console.error('Failed to load billing data:', error);
                this.subscriptionStatus = 'none';
            } finally {
                this.loading = false;
            }
        },
        
        calculateTrialDays(sub) {
            if (sub && sub.current_period_end) {
                const endDate = new Date(sub.current_period_end);
                const now = new Date();
                const diffTime = endDate - now;
                this.trialDaysRemaining = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
                if (this.trialDaysRemaining < 0) {
                    this.subscriptionStatus = 'expired';
                    this.trialDaysRemaining = 0;
                }
            } else {
                this.trialDaysRemaining = 14; // Default trial
            }
        },
        
        async loadExchangeRates() {
            try {
                const data = await InvoiceFastAPI.billing.getExchangeRates();
                if (data?.rates) {
                    this.exchangeRates = data.rates;
                }
            } catch (error) {
                console.error('Failed to load exchange rates:', error);
            }
        },
        
        async loadPlans() {
            try {
                console.log('[Billing] Loading plans from API...');
                const plansData = await InvoiceFastAPI.billing.getPlans();
                console.log('[Billing] Plans response:', plansData);
                if (plansData?.plans && plansData.plans.length > 0) {
                    this.plans = plansData.plans.map(plan => this.mapPlan(plan));
                    console.log('[Billing] Mapped plans:', this.plans.length);
                } else {
                    console.log('[Billing] No plans returned from API');
                    this.plans = [];
                }
            } catch (error) {
                console.error('[Billing] Failed to load plans:', error);
                this.plans = [];
            }
        },
        
        mapPlan(plan) {
            let features = [];
            let limits = {};
            
            try {
                features = JSON.parse(plan.features_json || '[]');
            } catch (e) {}
            
            try {
                limits = JSON.parse(plan.limits_json || '{}');
            } catch (e) {}
            
            const featureLabels = {
                invoices: 'Unlimited invoices',
                clients: 'Unlimited clients',
                payments: 'Payment tracking',
                reports: 'Advanced reports',
                pdf_export: 'PDF export',
                email_reminders: 'Email reminders',
                team_members: 'Team members',
                advanced_analytics: 'Advanced analytics',
                branding: 'Branding customization',
                priority_support: 'Priority support',
                automation: 'Automation tools',
                api_access: 'API access',
                bulk_actions: 'Bulk actions',
                workflow_automation: 'Workflow automation',
                dedicated_support: 'Dedicated support',
                sla: 'SLA guarantee',
                custom_integrations: 'Custom integrations',
                whitelabel: 'White-label options'
            };
            
            const displayFeatures = features
                .map(f => featureLabels[f] || f)
                .slice(0, 6);
            
            if (plan.slug === 'starter') {
                displayFeatures.unshift(limits.clients === -1 ? 'Unlimited clients' : `${limits.clients} clients`);
            } else if (plan.slug === 'growth') {
                displayFeatures.unshift(limits.users === -1 ? 'Unlimited users' : `${limits.users} users`);
            } else if (plan.slug === 'enterprise') {
                displayFeatures.push('Dedicated account manager');
            }
            
            return {
                id: plan.id,
                name: plan.name,
                slug: plan.slug,
                price: plan.monthly_price_usd || 0,
                yearlyPrice: plan.yearly_price_usd || 0,
                description: plan.description,
                features: displayFeatures,
                popular: plan.slug === 'business',
                custom: plan.slug === 'enterprise',
                trialDays: plan.trial_days || 14
            };
        },
        
        calculateUsageMetrics(subData) {
            const limits = subData?.usage || {};
            this.usageMetrics = [
                { 
                    key: 'invoices', 
                    label: 'Invoices', 
                    used: limits.invoices_used || 0, 
                    limit: limits.max_invoices || 10,
                    get percentage() { return this.limit > 0 ? (this.used / this.limit) * 100 : 0; },
                    get remainingText() { return this.limit - this.used + ' remaining'; }
                },
                { 
                    key: 'clients', 
                    label: 'Clients', 
                    used: limits.clients_used || 0, 
                    limit: limits.max_clients || 5,
                    get percentage() { return this.limit > 0 ? (this.used / this.limit) * 100 : 0; },
                    get remainingText() { return this.limit - this.used + ' remaining'; }
                },
                { 
                    key: 'users', 
                    label: 'Team Members', 
                    used: limits.users_used || 0, 
                    limit: limits.max_users || 1,
                    get percentage() { return this.limit > 0 ? (this.used / this.limit) * 100 : 0; },
                    get remainingText() { return this.limit - this.used + ' remaining'; }
                },
                { 
                    key: 'storage', 
                    label: 'Storage', 
                    used: limits.storage_used || 0, 
                    limit: limits.max_storage || 100,
                    get percentage() { return this.limit > 0 ? (this.used / this.limit) * 100 : 0; },
                    get remainingText() { return (this.limit - this.used) + ' MB remaining'; }
                }
            ];
            
            const warnings = this.usageMetrics.filter(m => m.percentage > 80);
            if (warnings.length > 0) {
                this.usageWarning = warnings.map(w => `${w.label} at ${Math.round(w.percentage)}%`).join(', ');
            }
        },
        
        async confirmUpgrade() {
            if (!this.selectedPlan) return;
            
            const payload = { plan_id: this.selectedPlan.id };
            if (this.paymentMethod === 'mpesa') {
                payload.payment_method = 'mpesa';
                if (this.mpesaPhone) {
                    payload.phone = this.mpesaPhone;
                }
            }
            
            try {
                const result = await InvoiceFastAPI.billing.createCheckoutSession(payload);
                
                if (result?.url) {
                    window.location.href = result.url;
                } else if (result?.checkout_url) {
                    window.location.href = result.checkout_url;
                } else if (result?.checkout_id) {
                    this.successModalOpen = true;
                    this.successMessage = 'STK push sent to your phone. Please complete payment on your device.';
                } else if (result?.activated) {
                    this.upgradeModalOpen = false;
                    this.successModalOpen = true;
                    this.successMessage = result.message || 'Your subscription has been activated!';
                    this.loadData();
                } else if (result?.subscription) {
                    this.upgradeModalOpen = false;
                    this.successModalOpen = true;
                    this.successMessage = 'Subscription activated successfully!';
                    this.loadData();
                }
            } catch (error) {
                console.error('Upgrade failed:', error);
                this.failureModalOpen = true;
                this.failureMessage = error.message || 'Failed to process payment. Please try again.';
            }
        },
        
        async setDefaultPaymentMethod(methodId) {
            try {
                await InvoiceFastAPI.billing.setDefaultPaymentMethod(methodId);
                this.showToast('Default payment method updated', 'success');
                await this.loadData();
            } catch(error) {
                console.error('Set default failed:', error);
                this.showToast('Failed to update default payment method', 'error');
            }
        },
        
        async removePaymentMethod(methodId) {
            if (!confirm('Are you sure you want to remove this payment method?')) return;
            try {
                await InvoiceFastAPI.billing.deletePaymentMethod(methodId);
                this.showToast('Payment method removed', 'success');
                await this.loadData();
            } catch(error) {
                console.error('Remove payment method failed:', error);
                this.showToast('Failed to remove payment method', 'error');
            }
        },
        
        async updatePaymentMethod() {
            if (!this.newPaymentMethod) {
                this.showToast('Please select a payment method', 'warning');
                return;
            }
            
            try {
                // Determine provider based on payment method
                let provider = '';
                if (this.newPaymentMethod === 'mpesa') {
                    provider = 'mpesa';
                } else if (this.newPaymentMethod === 'card') {
                    provider = 'stripe';
                }
                
                await InvoiceFastAPI.billing.updateSubscriptionPaymentMethod(this.newPaymentMethod, provider);
                this.showToast('Payment method updated successfully', 'success');
                this.paymentMethodModalOpen = false;
                await this.loadData();
            } catch(error) {
                console.error('Update payment method failed:', error);
                this.showToast('Failed to update payment method: ' + (error.message || 'Please try again'), 'error');
            }
        },
        
        showToast(message, type = 'info') {
            this.toast = { show: true, message, type };
            setTimeout(() => this.toast.show = false, 3000);
        },
        
        toast: { show: false, message: '', type: 'info' },
        
        formatPrice(price, currency = 'KES') {
            if (!price && price !== 0) return '-';
            const symbols = { USD: '$', KES: 'KES ', EUR: '€', GBP: '£' };
            const symbol = symbols[currency] || currency + ' ';
            
            // Convert cents to actual amount (KES uses cents too)
            const displayPrice = price > 100 ? (price / 100) : price;
            
            return symbol + new Intl.NumberFormat('en-KE').format(displayPrice);
        },
        
        formatDate(date) {
            if (!date) return '-';
            return new Date(date).toLocaleDateString('en-KE', {
                year: 'numeric',
                month: 'short',
                day: 'numeric'
            });
        }
    }));
    
    // Pricing Content Component
    Alpine.data('pricingContent', () => ({
        billingCycle: 'monthly',
        plans: [],
        exchangeRates: {},
        currency: 'KES',
        selectedPlan: null,
        loading: true,
        checkoutModalOpen: false,
        paymentMethod: 'stripe',
        mpesaPhone: '',
        successModalOpen: false,
        successMessage: '',
        failureModalOpen: false,
        failureMessage: '',
        
        async init() {
            await this.loadExchangeRates();
            await this.loadPlans();
        },
        
        async loadExchangeRates() {
            try {
                const data = await InvoiceFastAPI.billing.getExchangeRates();
                if (data?.rates) {
                    this.exchangeRates = data.rates;
                }
            } catch (error) {
                console.error('Failed to load exchange rates:', error);
            }
        },
        
        setCurrency(curr) {
            this.currency = curr;
        },
        
        async loadPlans() {
            this.loading = true;
            try {
                const plansData = await InvoiceFastAPI.billing.getPlans();
                if (plansData?.plans) {
                    this.plans = plansData.plans.map(plan => this.mapPlan(plan));
                }
            } catch (error) {
                console.error('Failed to load plans:', error);
            } finally {
                this.loading = false;
            }
        },
        
        mapPlan(plan) {
            let features = [];
            try {
                features = JSON.parse(plan.features_json || '[]');
            } catch (e) {}
            
            const featureLabels = {
                invoices: 'Unlimited invoices',
                clients: 'Unlimited clients',
                payments: 'Payment tracking',
                reports: 'Advanced reports',
                pdf_export: 'PDF export',
                email_reminders: 'Email reminders',
                team_members: 'Team members',
                advanced_analytics: 'Advanced analytics',
                branding: 'Branding customization',
                priority_support: 'Priority support',
                automation: 'Automation tools',
                api_access: 'API access',
                bulk_actions: 'Bulk actions',
                workflow_automation: 'Workflow automation',
                dedicated_support: 'Dedicated support',
                sla: 'SLA guarantee',
                custom_integrations: 'Custom integrations',
                whitelabel: 'White-label options'
            };
            
            return {
                id: plan.id,
                name: plan.name,
                price: plan.monthly_price_usd || 0,
                yearlyPrice: plan.yearly_price_usd || 0,
                description: plan.description,
                features: features.map(f => featureLabels[f] || f).slice(0, 6),
                popular: plan.slug === 'business',
                custom: plan.slug === 'enterprise'
            };
        },
        
        openCheckout(plan) {
            this.selectedPlan = plan;
            this.checkoutModalOpen = true;
        },
        
async confirmCheckout() {
            if (!this.selectedPlan) return;
            
            const payload = { plan_id: this.selectedPlan.id };
            if (this.paymentMethod === 'mpesa') {
                payload.payment_method = 'mpesa';
                if (this.mpesaPhone) payload.phone = this.mpesaPhone;
            }
            
            try {
                const result = await InvoiceFastAPI.billing.createCheckoutSession(payload);
                
                if (result?.url) {
                    window.location.href = result.url;
                } else if (result?.checkout_url) {
                    window.location.href = result.checkout_url;
                } else if (result?.checkout_id) {
                    this.checkoutModalOpen = false;
                    this.successModalOpen = true;
                    this.successMessage = 'STK push sent to your phone. Please complete payment.';
                } else if (result?.activated) {
                    this.checkoutModalOpen = false;
                    this.successModalOpen = true;
                    this.successMessage = result.message || 'Subscription activated!';
                } else if (result?.subscription) {
                    this.checkoutModalOpen = false;
                    this.successModalOpen = true;
                    this.successMessage = 'Subscription activated!';
                }
            } catch (error) {
                console.error('Checkout failed:', error);
                this.failureModalOpen = true;
                this.failureMessage = error.message || 'Failed to process payment.';
            }
        },
        
        getYearlyPrice(monthlyPrice) {
            if (!monthlyPrice) return 'Custom';
            const price = monthlyPrice > 100 ? (monthlyPrice / 100) : monthlyPrice;
            
            if (this.currency === 'KES') {
                return 'KES ' + Math.round(price * 12 * 0.8);
            }
            
            // Convert KES to selected currency using exchange rates
            const kesRate = this.exchangeRates['KES'] || this.exchangeRates['KES/USD'];
            if (!kesRate) return 'KES ' + Math.round(price);
            
            const usdToKes = 1 / kesRate;
            const kesAmount = price * usdToKes;
            
            const symbols = { USD: '$', EUR: '€', GBP: '£' };
            const symbol = symbols[this.currency] || this.currency + ' ';
            
            if (this.currency === 'EUR') {
                const eurRate = this.exchangeRates['KES/EUR'];
                return eurRate ? symbol + Math.round(kesAmount * eurRate * 12 * 0.8) : 'KES ' + Math.round(price * 12 * 0.8);
            } else if (this.currency === 'GBP') {
                const gbpRate = this.exchangeRates['KES/GBP'];
                return gbpRate ? symbol + Math.round(kesAmount * gbpRate * 12 * 0.8) : 'KES ' + Math.round(price * 12 * 0.8);
            }
            
            return symbol + Math.round(kesAmount * 12 * 0.8);
        },
        
        formatPrice(price) {
            if (!price && price !== 0) return 'Custom';
            
            // Prices from API are in KES
            const kesAmount = price > 100 ? (price / 100) : price;
            
            if (this.currency === 'KES') {
                return 'KES ' + Math.round(kesAmount);
            }
            
            // Convert KES to selected currency using exchange rates
            const kesRate = this.exchangeRates['KES'] || this.exchangeRates['KES/USD'];
            if (!kesRate) return 'KES ' + Math.round(kesAmount);
            
            if (this.currency === 'USD') {
                return '$' + Math.round(kesAmount * kesRate);
            } else if (this.currency === 'EUR') {
                const eurRate = this.exchangeRates['KES/EUR'];
                return eurRate ? '€' + Math.round(kesAmount * eurRate) : 'KES ' + Math.round(kesAmount);
            } else if (this.currency === 'GBP') {
                const gbpRate = this.exchangeRates['KES/GBP'];
                return gbpRate ? '£' + Math.round(kesAmount * gbpRate) : 'KES ' + Math.round(kesAmount);
            }
            
            return 'KES ' + Math.round(kesAmount);
        }
    }));
});