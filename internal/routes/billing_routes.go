package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func BillingRoutes(app *fiber.App, h *handlers.BillingHandler, authService *services.AuthService, db *database.DB, webhookVerifier *middleware.WebhookVerifierMiddleware, idempotencySvc *services.IdempotencyService) fiber.Router {
	group := app.Group("/api/v1/tenant/billing")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/subscription", h.GetSubscription)
	group.Post("/subscription", h.CreateSubscription)
	group.Delete("/subscription", h.CancelSubscription)
	group.Post("/subscription/reactivate", h.ReactivateSubscription)
	group.Put("/subscription/plan", h.ChangePlan)

	group.Get("/plans", h.GetPlans)

	group.Post("/checkout", h.CreateCheckoutSession)

	group.Get("/history", h.GetBillingHistory)

	group.Post("/mpesa", h.InitiateMpesaPayment)

	group.Get("/payment-methods", h.GetPaymentMethods)
	group.Delete("/payment-methods/:id", h.DeletePaymentMethod)
	group.Put("/payment-methods/:id/default", h.SetDefaultPaymentMethod)

	group.Get("/limits/:resource", h.CheckLimits)

	group.Get("/usage", h.GetUsage)

	// BILLING WEBHOOKS - with signature verification + idempotency
	group.Post("/webhook/mpesa",
		middleware.IdempotencyMiddleware(idempotencySvc),
		webhookVerifier.MpesaVerification(),
		h.HandleMpesaWebhook)
	
	group.Post("/webhook/intasend",
		middleware.IdempotencyMiddleware(idempotencySvc),
		webhookVerifier.IntasendVerification(),
		h.HandleIntasendWebhook)

	return group
}
