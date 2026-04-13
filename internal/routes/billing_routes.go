package routes

import (
	"invoicefast/internal/database"
	"invoicefast/internal/handlers"
	"invoicefast/internal/middleware"
	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func BillingRoutes(app *fiber.App, h *handlers.BillingHandler, authService *services.AuthService, db *database.DB) fiber.Router {
	group := app.Group("/api/v1/tenant/billing")
	group.Use(middleware.TenantMiddleware(authService, db))

	group.Get("/subscription", h.GetSubscription)
	group.Post("/subscription", h.CreateSubscription)
	group.Delete("/subscription", h.CancelSubscription)
	group.Post("/subscription/reactivate", h.ReactivateSubscription)
	group.Put("/subscription/plan", h.ChangePlan)

	group.Get("/plans", h.GetPlans)

	group.Get("/history", h.GetBillingHistory)

	group.Post("/mpesa", h.InitiateMpesaPayment)

	group.Get("/payment-methods", h.GetPaymentMethods)
	group.Delete("/payment-methods/:id", h.DeletePaymentMethod)
	group.Put("/payment-methods/:id/default", h.SetDefaultPaymentMethod)

	group.Get("/limits/:resource", h.CheckLimits)

	group.Get("/usage", h.GetUsage)

	group.Post("/webhook/mpesa", h.HandleMpesaWebhook)
	group.Post("/webhook/intasend", h.HandleIntasendWebhook)

	return group
}
