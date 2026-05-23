package handlers

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var safeErrors = []string{
	"invalid",
	"required",
	"not found",
	"already exists",
	"already in use",
	"too many requests",
	"unauthorized",
	"forbidden",
	"not supported",
	"not configured",
	"cannot",
	"expired",
	"missing",
	"unsupported",
	"already verified",
	"not verified",
	"does not meet",
	"too long",
	"too short",
	"weak",
	"compromised",
	"mismatch",
	"does not match",
	"already used",
	"already processed",
}

func isSafeError(msg string) bool {
	lower := strings.ToLower(msg)
	for _, safe := range safeErrors {
		if strings.HasPrefix(lower, safe) || strings.Contains(lower, safe) {
			return true
		}
	}
	return false
}

func safeError(err error) string {
	if err == nil {
		return "internal server error"
	}
	msg := err.Error()
	if isSafeError(msg) {
		return msg
	}
	return "internal server error"
}

func sendError(c *fiber.Ctx, status int, err error) error {
	return c.Status(status).JSON(fiber.Map{"error": safeError(err)})
}

func sendBadRequest(c *fiber.Ctx, err error) error {
	return sendError(c, fiber.StatusBadRequest, err)
}

func sendInternalError(c *fiber.Ctx, err error) error {
	return sendError(c, fiber.StatusInternalServerError, err)
}

func sendUnauthorized(c *fiber.Ctx, err error) error {
	return sendError(c, fiber.StatusUnauthorized, err)
}

func sendNotFound(c *fiber.Ctx, err error) error {
	return sendError(c, fiber.StatusNotFound, err)
}

func sendConflict(c *fiber.Ctx, err error) error {
	return sendError(c, fiber.StatusConflict, err)
}

func sendTooManyRequests(c *fiber.Ctx, err error) error {
	return sendError(c, fiber.StatusTooManyRequests, err)
}

var ErrInternal = errors.New("internal server error")
