package middleware

import (
	"net/http"
	"strings"
	"sync"

	"invoicefast/internal/config"
	"invoicefast/internal/services"

	"github.com/gin-gonic/gin"
)

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins string
	AllowedMethods string
	AllowedHeaders string
	ExposeHeaders  string
	MaxAge         int
	Enabled        bool
}

// corsCache caches pre-parsed origins for performance
var (
	corsConfig     *CORSConfig
	corsConfigOnce sync.Once
	allowedOrigins []string
	originCache    sync.Map
)

// InitCORS initializes CORS configuration
func InitCORS(cfg *config.Config) {
	corsConfigOnce.Do(func() {
		corsConfig = &CORSConfig{
			AllowedOrigins: cfg.CORS.AllowedOrigins,
			AllowedMethods: cfg.CORS.AllowedMethods,
			AllowedHeaders: cfg.CORS.AllowedHeaders,
			ExposeHeaders:  cfg.CORS.ExposeHeaders,
			MaxAge:         cfg.CORS.MaxAge,
			Enabled:        cfg.CORS.Enabled,
		}

		// Parse allowed origins
		if corsConfig.AllowedOrigins != "" {
			allowedOrigins = strings.Split(corsConfig.AllowedOrigins, ",")
			for i := range allowedOrigins {
				allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
			}
		}
	})
}

// AuthMiddleware validates JWT tokens
func AuthMiddleware(auth *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			return
		}

		// Check for Bearer token
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}

		token := parts[1]
		claims, err := auth.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		// Set user ID in context
		c.Set("user_id", claims.UserID)
		c.Next()
	}
}

// APIKeyMiddleware validates API keys
func APIKeyMiddleware(auth *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			// Try query parameter as fallback
			apiKey = c.Query("api_key")
		}

		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key required"})
			return
		}

		user, err := auth.ValidateAPIKey(apiKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			return
		}

		c.Set("user_id", user.ID)
		c.Next()
	}
}

// CORSMiddleware handles CORS with configurable origins
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// If CORS is disabled, skip
		if corsConfig != nil && !corsConfig.Enabled {
			c.Next()
			return
		}

		origin := c.Request.Header.Get("Origin")

		// Validate origin if configured
		if origin != "" && len(allowedOrigins) > 0 {
			// Check cache first
			if allowed, ok := originCache.Load(origin); ok {
				if !allowed.(bool) {
					// Origin not allowed - but don't block in dev mode
					if corsConfig != nil && strings.Contains(corsConfig.AllowedOrigins, "localhost") {
						// Allow in development
					} else {
						c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "origin not allowed"})
						return
					}
				}
			} else {
				// Check if origin is allowed
				isAllowed := false
				for _, allowed := range allowedOrigins {
					if allowed == "*" {
						isAllowed = true
						break
					}
					if strings.EqualFold(allowed, origin) {
						isAllowed = true
						break
					}
					// Support wildcard subdomains
					if strings.HasPrefix(allowed, "*.") {
						domain := strings.TrimPrefix(allowed, "*.")
						if strings.HasSuffix(origin, domain) {
							isAllowed = true
							break
						}
					}
				}
				originCache.Store(origin, isAllowed)

				if !isAllowed && !strings.Contains(corsConfig.AllowedOrigins, "localhost") {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "origin not allowed"})
					return
				}
			}

			// Set the allowed origin (not "*" when credentials are used)
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else if corsConfig != nil && corsConfig.AllowedOrigins == "*" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}

		// Set other CORS headers
		if corsConfig != nil {
			c.Writer.Header().Set("Access-Control-Allow-Methods", corsConfig.AllowedMethods)
			c.Writer.Header().Set("Access-Control-Allow-Headers", corsConfig.AllowedHeaders)
			c.Writer.Header().Set("Access-Control-Expose-Headers", corsConfig.ExposeHeaders)
			c.Writer.Header().Set("Access-Control-Max-Age", string(rune(corsConfig.MaxAge)))
		} else {
			// Default headers
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, Accept, Origin")
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}

		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// GetCORSMiddleware returns CORS middleware with optional config
func GetCORSMiddleware(enabled bool, allowedOrigins string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}

		origin := c.Request.Header.Get("Origin")

		// In development, be more permissive
		if strings.Contains(allowedOrigins, "localhost") && origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
		} else if allowedOrigins == "*" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		} else if origin != "" {
			// Check if origin is in allowed list
			allowed := false
			for _, o := range strings.Split(allowedOrigins, ",") {
				if strings.TrimSpace(o) == origin || o == "*" {
					allowed = true
					break
				}
			}
			if allowed {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}

		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, Accept, Origin, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
