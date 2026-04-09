package middleware

import (
	"context"
	"log"

	"invoicefast/internal/services"

	"github.com/gofiber/fiber/v2"
)

func IdempotencyMiddleware(svc *services.IdempotencyService) fiber.Handler {
	if svc == nil {
		return func(c *fiber.Ctx) error {
			return c.Next()
		}
	}

	return func(c *fiber.Ctx) error {
		key := c.Get("Idempotency-Key")
		if key == "" {
			if c.Path() == "/api/v1/webhook/intasend" {
				key = c.Get("CheckoutID")
			}
		}

		if key == "" {
			return c.Next()
		}

		ctx := context.Background()

		isProcessed, err := svc.IsProcessed(ctx, key)
		if err == nil && isProcessed {
			log.Printf("[Idempotency] Key %s already processed - returning cached response", key)
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"status":          "already_processed",
				"idempotency_key": key,
			})
		}

		c.Locals("idempotency_key", key)
		c.Locals("idempotency_svc", svc)

		return c.Next()
	}
}
