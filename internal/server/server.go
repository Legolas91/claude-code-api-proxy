// Package server implements the HTTP proxy server that translates between
// Claude API format and OpenAI-compatible providers (OpenRouter, OpenAI Direct, Ollama).
//
// The server receives Claude API requests on /v1/messages, converts them to OpenAI format,
// forwards them to the configured provider, and converts responses back to Claude format.
// It handles both streaming (SSE) and non-streaming responses, including tool calls and
// thinking blocks from reasoning models.
package server

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/claude-code-proxy/proxy/internal/config"
	"github.com/claude-code-proxy/proxy/internal/converter"
	"github.com/claude-code-proxy/proxy/internal/daemon"
	"github.com/claude-code-proxy/proxy/internal/version"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// Start initializes and starts the HTTP server
func Start(cfg *config.Config) error {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
		ServerHeader:          "Claude-Code-Proxy",
		AppName:               "Claude Code Proxy v" + version.Version,
	})

	// Middleware
	app.Use(recover.New())

	// Custom CORS middleware - restrictive security policy
	// Only allows localhost origins to prevent cross-origin API key exfiltration
	app.Use(func(c *fiber.Ctx) error {
		origin := c.Get("Origin")

		// If no Origin header, allow (same-origin request)
		if origin == "" {
			return c.Next()
		}

		// Check if origin is localhost
		isLocalhost := strings.HasPrefix(origin, "http://localhost:") ||
			strings.HasPrefix(origin, "https://localhost:") ||
			strings.HasPrefix(origin, "http://127.0.0.1:") ||
			strings.HasPrefix(origin, "https://127.0.0.1:")

		// Preflight OPTIONS request
		if c.Method() == "OPTIONS" {
			if !isLocalhost {
				return c.Status(403).JSON(fiber.Map{
					"error": "CORS: Origin not allowed",
				})
			}
			c.Set("Access-Control-Allow-Origin", origin)
			c.Set("Access-Control-Allow-Methods", "POST")
			c.Set("Access-Control-Allow-Headers", "Content-Type,x-api-key")
			c.Set("Access-Control-Max-Age", "3600")
			return c.SendStatus(204)
		}

		// Actual request
		if !isLocalhost {
			return c.Status(403).JSON(fiber.Map{
				"error": "CORS: Origin not allowed",
			})
		}

		c.Set("Access-Control-Allow-Origin", origin)
		c.Set("Vary", "Origin")
		return c.Next()
	})

	// Enable HTTP logging only when simple log mode is enabled
	if cfg.SimpleLog {
		app.Use(logger.New(logger.Config{
			Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
		}))
	}

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"version": version.Version,
		})
	})

	// Root endpoint - proxy info
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Claude Code Proxy",
			"version": version.Version,
			"status":  "running",
			"config": fiber.Map{
				"openai_base_url": cfg.OpenAIBaseURL,
				"routing_mode":    getRoutingMode(cfg),
				"opus_model":      getOpusModel(cfg),
				"sonnet_model":    getSonnetModel(cfg),
				"haiku_model":     getHaikuModel(cfg),
			},
			"endpoints": fiber.Map{
				"health":       "/health",
				"messages":     "/v1/messages",
				"count_tokens": "/v1/messages/count_tokens",
			},
		})
	})

	// Claude API endpoints
	setupClaudeEndpoints(app, cfg)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		fmt.Println("\n🛑 Shutting down...")
		daemon.Cleanup()
		_ = app.Shutdown()
	}()

	// Start server
	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	fmt.Printf("✅ Proxy running at http://localhost:%s\n", cfg.Port)

	if cfg.PassthroughMode {
		fmt.Printf("   Mode: PASSTHROUGH (direct to Anthropic API)\n")
	} else {
		fmt.Printf("   Mode: Conversion (via %s)\n", cfg.OpenAIBaseURL)
		fmt.Printf("   Model Routing: %s\n", getRoutingMode(cfg))

		// Show actual model mappings
		if cfg.OpusModel != "" || cfg.SonnetModel != "" || cfg.HaikuModel != "" {
			fmt.Printf("   Models:\n")
			if cfg.OpusModel != "" {
				fmt.Printf("     - Opus   → %s\n", cfg.OpusModel)
			}
			if cfg.SonnetModel != "" {
				fmt.Printf("     - Sonnet → %s\n", cfg.SonnetModel)
			}
			if cfg.HaikuModel != "" {
				fmt.Printf("     - Haiku  → %s\n", cfg.HaikuModel)
			}
		}
	}

	return app.Listen(addr)
}

func getRoutingMode(cfg *config.Config) string {
	if cfg.OpusModel != "" || cfg.SonnetModel != "" || cfg.HaikuModel != "" {
		return "custom (env overrides)"
	}
	return "pattern-based"
}

func getOpusModel(cfg *config.Config) string {
	if cfg.OpusModel != "" {
		return cfg.OpusModel
	}
	return converter.DefaultOpusModel + " (pattern-based)"
}

func getSonnetModel(cfg *config.Config) string {
	if cfg.SonnetModel != "" {
		return cfg.SonnetModel
	}
	return "version-aware (pattern-based)"
}

func getHaikuModel(cfg *config.Config) string {
	if cfg.HaikuModel != "" {
		return cfg.HaikuModel
	}
	return converter.DefaultHaikuModel + " (pattern-based)"
}

// limitBodySize returns a middleware that enforces a maximum request body size.
// This prevents memory exhaustion attacks via oversized payloads.
func limitBodySize(maxSize int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if len(c.Body()) > maxSize {
			return c.Status(413).JSON(fiber.Map{
				"type": "error",
				"error": fiber.Map{
					"type":    "request_too_large",
					"message": fmt.Sprintf("Request body exceeds maximum size of %d bytes (%.1f MB)", maxSize, float64(maxSize)/(1024*1024)),
				},
			})
		}
		return c.Next()
	}
}

func setupClaudeEndpoints(app *fiber.App, cfg *config.Config) {
	// Body size limit: 10MB for messages, 1MB for token counting
	const maxMessageBodySize = 10 * 1024 * 1024 // 10MB
	const maxTokenCountBodySize = 1 * 1024 * 1024 // 1MB

	// Messages endpoint - main Claude API
	app.Post("/v1/messages", limitBodySize(maxMessageBodySize), func(c *fiber.Ctx) error {
		return handleMessages(c, cfg)
	})

	// Token counting endpoint
	app.Post("/v1/messages/count_tokens", limitBodySize(maxTokenCountBodySize), func(c *fiber.Ctx) error {
		return handleCountTokens(c, cfg)
	})
}
