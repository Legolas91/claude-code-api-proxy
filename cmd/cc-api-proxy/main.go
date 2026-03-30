package main

import (
	"fmt"
	"os"

	"github.com/claude-code-proxy/proxy/internal/config"
	"github.com/claude-code-proxy/proxy/internal/daemon"
	"github.com/claude-code-proxy/proxy/internal/server"
	"github.com/claude-code-proxy/proxy/internal/version"
)

func main() {
	// Parse command and flags
	debug := false
	simpleLog := false
	command := ""

	if len(os.Args) > 1 {
		for i := 1; i < len(os.Args); i++ {
			arg := os.Args[i]
			switch arg {
			case "-d", "--debug":
				debug = true
			case "-s", "--simple":
				simpleLog = true
			case "stop", "status", "version", "help", "-h", "--help":
				command = arg
			}
		}

		// Handle commands
		switch command {
		case "stop":
			daemon.Stop()
			return
		case "status":
			daemon.Status()
			return
		case "version":
			fmt.Printf("cc-api-proxy %s\n", version.Version)
			return
		case "help", "-h", "--help":
			printHelp()
			return
		}
	}

	// Load configuration with debug mode
	var cfg *config.Config
	var err error
	if debug {
		cfg, err = config.LoadWithDebug(true)
		fmt.Println("🐛 Debug mode enabled - full request/response logging active")
	} else {
		cfg, err = config.Load()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Enable simple logging if requested
	if simpleLog {
		cfg.SimpleLog = true
		fmt.Println("📊 Simple log mode enabled - one-line summaries per request")
	}

	// Check if already running
	if daemon.IsRunning() {
		fmt.Println("Proxy is already running")
		os.Exit(0)
	}

	// Daemonize (run in background)
	if err := daemon.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
		os.Exit(1)
	}

	// Start HTTP server (blocks)
	// Note: No need to pre-fetch reasoning models - adaptive per-model detection
	// handles all models automatically through retry mechanism
	if err := server.Start(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`cc-api-proxy - OpenAI-compatible API proxy for Claude Code

Usage:
  cc-api-proxy [-d|--debug] [-s|--simple]  Start the proxy daemon
  cc-api-proxy stop                        Stop the proxy daemon
  cc-api-proxy status                      Check if proxy is running
  cc-api-proxy version                     Show version
  cc-api-proxy help                        Show this help

Flags:
  -d, --debug     Enable debug mode (logs full requests/responses)
  -s, --simple    Enable simple log mode (one-line summary per request)

Configuration:
  Config file locations (checked in order):
    1. ./.env
    2. ~/.claude/proxy.env
    3. ~/.cc-api-proxy

  Required:
    OPENAI_API_KEY              Default API key (fallback for all tiers)

  Model routing (per-tier URL, key and model):
    ANTHROPIC_DEFAULT_OPUS_BASE_URL    Override base URL for Opus tier
    ANTHROPIC_DEFAULT_OPUS_API_KEY     Override API key for Opus tier
    ANTHROPIC_DEFAULT_OPUS_MODEL       Override model name for Opus tier
    ANTHROPIC_DEFAULT_SONNET_BASE_URL  Override base URL for Sonnet tier
    ANTHROPIC_DEFAULT_SONNET_API_KEY   Override API key for Sonnet tier
    ANTHROPIC_DEFAULT_SONNET_MODEL     Override model name for Sonnet tier
    ANTHROPIC_DEFAULT_HAIKU_BASE_URL   Override base URL for Haiku tier
    ANTHROPIC_DEFAULT_HAIKU_API_KEY    Override API key for Haiku tier
    ANTHROPIC_DEFAULT_HAIKU_MODEL      Override model name for Haiku tier
    OPENAI_BASE_URL                    Default base URL (fallback for all tiers)

  Enterprise HTTP proxy:
    CLAUDE_HTTP_PROXY       HTTP proxy URL (e.g. http://proxy.company.com:8080)
    CLAUDE_HTTPS_PROXY      HTTPS proxy URL
    CLAUDE_NO_PROXY         Comma-separated list of hosts to bypass
    CLAUDE_PROXY_FROM_ENV   Use system HTTP_PROXY/HTTPS_PROXY (default: true)

  Tool reliability (v1.5.15+):
    PROXY_AUGMENT_TOOL_PROMPT   Inject tool-use instructions into system prompt
                                (default: auto — enabled for unknown providers only;
                                 set true/false to force on/off)
    PROXY_TOOL_PROMPT_TEMPLATE  Override per-model tool guidance text
    PROXY_REPAIR_TOOL_CALLS     Fix malformed tool call arguments (default: true)
    PROXY_MAX_LOOP_LEVEL        Cap loop escalation: 1=nudge, 2=strong nudge,
                                3=disable tools (default: 3)

  Loop detection:
    PROXY_MAX_IDENTICAL_RETRIES  Identical tool calls before nudge injection
                                 (default: 3, 0=disabled)

  Server:
    HOST                    Server host (default: 0.0.0.0)
    PORT                    Server port (default: 8082)

Examples:
  # Start proxy
  cc-api-proxy

  # Start with debug logging
  cc-api-proxy -d

  # Use with Claude Code
  ANTHROPIC_BASE_URL=http://localhost:8082 claude -p "Hello"`)
}
