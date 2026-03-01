# Claude Code API Proxy (Go)

[![Latest Release](https://img.shields.io/github/v/release/Legolas91/claude-code-api-proxy)](https://github.com/Legolas91/claude-code-api-proxy/releases/latest)
[![Go 1.25](https://img.shields.io/badge/go-1.25-00ADD8?logo=go)](https://go.dev/doc/go1.25)
[![CI](https://img.shields.io/github/actions/workflow/status/Legolas91/claude-code-api-proxy/ci.yml?branch=main)](https://github.com/Legolas91/claude-code-api-proxy/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/github/license/Legolas91/claude-code-api-proxy)](LICENSE)
[![Issues](https://img.shields.io/github/issues/Legolas91/claude-code-api-proxy)](https://github.com/Legolas91/claude-code-api-proxy/issues)

> **Fork** of [nielspeter/claude-code-proxy](https://github.com/nielspeter/claude-code-proxy) — extended with per-tier multi-URL/multi-key routing, enterprise HTTP proxy support, adaptive model detection, and security fixes.

`cc-api-proxy` translates Claude API requests into OpenAI-compatible format, enabling Claude Code to work with any OpenAI-compatible backend. Designed for enterprises operating in air-gapped environments or where access to public LLMs is restricted — primarily for use with models running on internal infrastructure — it also supports public providers such as OpenRouter, Ollama, Mammouth.ai, etc. Each Claude tier (Opus/Sonnet/Haiku) can be independently routed to a different provider URL and API key.

> **⚠️ Beta Software** — Core functionality works. Edge cases may have issues. Feedback welcome at https://github.com/Legolas91/claude-code-api-proxy/issues

## Features

- ✅ **Full Claude Code Compatibility** - Complete support for all Claude Code features
  - Tool calling (read, write, edit, glob, grep, bash, etc.)
  - Extended thinking blocks with proper hiding/showing
  - Streaming responses with real-time token tracking
  - Proper SSE event formatting
- ✅ **Any OpenAI-Compatible Provider** - Works with any provider out of the box
  - **OpenRouter**: 200+ models (GPT, Grok, Gemini, Codestral, etc.)
  - **OpenAI Direct**: Native GPT-5 reasoning model support
  - **Ollama**: Free local inference (DeepSeek-R1, Llama3, Qwen, etc.)
  - **Custom providers**: Mammouth.ai, enterprise API gateways, any OpenAI-compatible endpoint
- ✅ **Per-Tier Multi-URL/Multi-Key Routing** *(v1.5.0+)* - Route each Claude tier independently
  - Different provider URL per tier (Opus/Sonnet/Haiku)
  - Different API key per tier
  - Automatic fallback to `OPENAI_BASE_URL` / `OPENAI_API_KEY`
- ✅ **Enterprise HTTP Proxy Support** *(v1.5.5+)* - Corporate environment ready
  - `CLAUDE_HTTP_PROXY` / `CLAUDE_HTTPS_PROXY` for outbound proxy
  - `CLAUDE_NO_PROXY` bypass list with pattern matching
  - System proxy auto-detection (`HTTP_PROXY` / `HTTPS_PROXY`)
- ✅ **Adaptive Per-Model Detection** - Zero-config provider compatibility
  - Automatically learns which parameters each model supports
  - No hardcoded model patterns - works with any future model/provider
  - Per-model capability caching for instant subsequent requests
- ✅ **Pattern-based routing** - Auto-detects Claude model tier and routes to configured backend
- ✅ **Zero dependencies** - Single ~10MB binary, no runtime needed
- ✅ **Daemon mode** - Runs in background, serves multiple Claude Code sessions
- ✅ **Fast startup** - < 10ms cold start
- ✅ **Config flexibility** - Loads from `~/.claude/proxy.env` or `.env`

## Quick Start

### Build

```bash
# Install dependencies
go mod download

# Build binary
go build -o cc-api-proxy cmd/cc-api-proxy/main.go

# Or use make
make build
```

### Install

**Option 1: System-wide installation (recommended)**

```bash
# Install binary and ccp wrapper to /usr/local/bin
make install

# This installs:
#   - cc-api-proxy (main binary)
#   - ccp (wrapper script for easy usage)
```

**Option 2: Manual installation**

```bash
# Copy binary to PATH
sudo cp cc-api-proxy /usr/local/bin/

# Copy wrapper script (optional but recommended)
sudo cp scripts/ccp /usr/local/bin/
sudo chmod +x /usr/local/bin/ccp
```

After installation, `cc-api-proxy` and `ccp` will be available system-wide.

### Configuration

The proxy supports three provider types. Choose the one that fits your needs:

**Option 1: OpenRouter (Recommended)**
```bash
mkdir -p ~/.claude
cat > ~/.claude/proxy.env << 'EOF'
OPENAI_BASE_URL=https://openrouter.ai/api/v1
OPENAI_API_KEY=sk-or-v1-your-openrouter-key

# Model routing
ANTHROPIC_DEFAULT_SONNET_MODEL=x-ai/grok-code-fast-1
ANTHROPIC_DEFAULT_HAIKU_MODEL=google/gemini-2.5-flash

# Optional: Better rate limits
OPENROUTER_APP_NAME=Claude-Code-API-Proxy
OPENROUTER_APP_URL=https://github.com/yourname/repo
EOF
```

**Option 2: OpenAI Direct**
```bash
mkdir -p ~/.claude
cat > ~/.claude/proxy.env << 'EOF'
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=sk-proj-your-openai-key

# Model routing
ANTHROPIC_DEFAULT_SONNET_MODEL=gpt-5
ANTHROPIC_DEFAULT_HAIKU_MODEL=gpt-5-mini
ANTHROPIC_DEFAULT_OPUS_MODEL=gpt-5  # Reasoning model
EOF
```

**Option 3: Ollama (Local)**
```bash
mkdir -p ~/.claude
cat > ~/.claude/proxy.env << 'EOF'
OPENAI_BASE_URL=http://localhost:11434/v1
# No API key needed!

# Model routing
ANTHROPIC_DEFAULT_SONNET_MODEL=deepseek-r1:70b
ANTHROPIC_DEFAULT_HAIKU_MODEL=llama3.1:8b
EOF
```

## Provider Comparison

| Feature | OpenRouter | OpenAI Direct | Ollama |
|---------|-----------|---------------|--------|
| **Cost** | Pay-per-use | Pay-per-use | Free |
| **Setup** | Easy | Easy | Requires local install |
| **Models** | 200+ | OpenAI only | Open source only |
| **Reasoning** | Yes (via GPT/Grok/etc) | Yes (GPT-5) | Yes (DeepSeek-R1) |
| **Tool Calling** | Yes | Yes | Model dependent |
| **Privacy** | Cloud | Cloud | 100% local |
| **Speed** | Fast | Fast | Very fast (local) |
| **API Key** | Required | Required | Not needed |

### Run

**Commands:**

```bash
./cc-api-proxy              # Start daemon
./cc-api-proxy status       # Check if running
./cc-api-proxy stop         # Stop daemon
./cc-api-proxy version      # Show version
./cc-api-proxy help         # Show help
```

**Flags:**

```bash
-d, --debug     # Enable debug mode (full request/response logging)
-s, --simple    # Enable simple log mode (one-line summaries)
```

**Examples:**

```bash
# Start with debug logging
./cc-api-proxy -d

# Start with simple one-line summaries
./cc-api-proxy -s

# Combine flags
./cc-api-proxy -d -s
```

**Option 1: Use ccp wrapper (recommended)**

If you installed via `make install`, the `ccp` wrapper is already available:

```bash
# Use ccp instead of claude
ccp chat
ccp --version
ccp code /path/to/project
```

The `ccp` wrapper automatically:
- Starts the proxy daemon (if not running)
- Sets `ANTHROPIC_BASE_URL`
- Execs `claude` with your arguments

**No installation needed** - `ccp` is installed system-wide with `make install`.

**Option 2: Use with Claude Code directly**

```bash
# Start the proxy
./cc-api-proxy

# Configure Claude Code to use the proxy
export ANTHROPIC_BASE_URL=http://localhost:8082
claude chat
```

## Pattern-Based Routing

The proxy auto-detects Claude model names:

| Claude Model Pattern | Default OpenAI Model |
|---------------------|---------------------|
| `*opus*` | `gpt-5` |
| `*sonnet*` | `gpt-5` |
| `*haiku*` | `gpt-5-mini` |

Override with env vars:
```bash
ANTHROPIC_DEFAULT_OPUS_MODEL=gpt-5
ANTHROPIC_DEFAULT_SONNET_MODEL=gpt-5
ANTHROPIC_DEFAULT_HAIKU_MODEL=gpt-5-mini
```

## Build for Distribution

```bash
# Build for all platforms
make build-all

# Output:
# dist/cc-api-proxy-darwin-amd64
# dist/cc-api-proxy-darwin-arm64
# dist/cc-api-proxy-linux-amd64
# dist/cc-api-proxy-linux-arm64
# dist/cc-api-proxy-windows-amd64.exe
```

## Configuration Reference

**Required:**
- `OPENAI_API_KEY` - Your API key (not needed for Ollama/localhost)

**Optional - API Configuration:**
- `OPENAI_BASE_URL` - API base URL (default: `https://api.openai.com/v1`)
  - For OpenRouter: `https://openrouter.ai/api/v1`
  - For Ollama: `http://localhost:11434/v1`
  - For other providers: Use their OpenAI-compatible endpoint

**Optional - Model Routing:**
- `ANTHROPIC_DEFAULT_OPUS_MODEL` - Override opus routing (default: `gpt-5`)
- `ANTHROPIC_DEFAULT_SONNET_MODEL` - Override sonnet routing (default: `gpt-5`)
- `ANTHROPIC_DEFAULT_HAIKU_MODEL` - Override haiku routing (default: `gpt-5-mini`)

Examples with OpenRouter:
```bash
ANTHROPIC_DEFAULT_SONNET_MODEL=x-ai/grok-code-fast-1
ANTHROPIC_DEFAULT_HAIKU_MODEL=google/gemini-2.5-flash
ANTHROPIC_DEFAULT_OPUS_MODEL=openai/gpt-5
```

**Optional - Multi-Provider Routing (v1.5.0+):**

Route different Claude tiers to different backend providers with different API keys.

- `ANTHROPIC_DEFAULT_OPUS_BASE_URL` - Base URL for opus tier (fallback: `OPENAI_BASE_URL`)
- `ANTHROPIC_DEFAULT_OPUS_API_KEY` - API key for opus tier (fallback: `OPENAI_API_KEY`)
- `ANTHROPIC_DEFAULT_SONNET_BASE_URL` - Base URL for sonnet tier (fallback: `OPENAI_BASE_URL`)
- `ANTHROPIC_DEFAULT_SONNET_API_KEY` - API key for sonnet tier (fallback: `OPENAI_API_KEY`)
- `ANTHROPIC_DEFAULT_HAIKU_BASE_URL` - Base URL for haiku tier (fallback: `OPENAI_BASE_URL`)
- `ANTHROPIC_DEFAULT_HAIKU_API_KEY` - API key for haiku tier (fallback: `OPENAI_API_KEY`)

Example - Multi-provider with cost optimization:
```bash
# Opus → OpenRouter (GPT-5 for complex reasoning)
ANTHROPIC_DEFAULT_OPUS_BASE_URL=https://openrouter.ai/api/v1
ANTHROPIC_DEFAULT_OPUS_API_KEY=sk-or-v1-xxx
ANTHROPIC_DEFAULT_OPUS_MODEL=openai/gpt-5

# Sonnet → Custom API (Specialized code model)
ANTHROPIC_DEFAULT_SONNET_BASE_URL=https://api.provider.com/v1
ANTHROPIC_DEFAULT_SONNET_API_KEY=sk-provider-yyy
ANTHROPIC_DEFAULT_SONNET_MODEL=codestral-2508

# Haiku → Ollama local (Free, offline)
ANTHROPIC_DEFAULT_HAIKU_BASE_URL=http://localhost:11434/v1
ANTHROPIC_DEFAULT_HAIKU_MODEL=qwen2.5:14b

# Fallback for non-Claude models
OPENAI_BASE_URL=https://openrouter.ai/api/v1
OPENAI_API_KEY=sk-or-v1-xxx
```

**Optional - Enterprise HTTP Proxy (v1.5.5+):**

Route outbound requests through a corporate HTTP/HTTPS proxy.

- `CLAUDE_HTTP_PROXY` - HTTP proxy URL (e.g., `http://proxy.company.com:8080`)
- `CLAUDE_HTTPS_PROXY` - HTTPS proxy URL
- `CLAUDE_NO_PROXY` - Comma-separated list of hosts to bypass (e.g., `localhost,127.0.0.1,.internal`)
- `CLAUDE_PROXY_FROM_ENV` - Use system `HTTP_PROXY`/`HTTPS_PROXY` env vars (default: `true`)

**Optional - OpenRouter Specific:**
- `OPENROUTER_APP_NAME` - App name for OpenRouter dashboard tracking
- `OPENROUTER_APP_URL` - App URL for better rate limits (higher quotas)

**Optional - Security:**
- `ANTHROPIC_API_KEY` - Client API key validation (optional)
  - If set, clients must provide this exact key
  - Leave unset to disable validation

**Optional - Server Settings:**
- `HOST` - Server host (default: `0.0.0.0`)
- `PORT` - Server port (default: `8082`)
- `PASSTHROUGH_MODE` - Direct proxy to Anthropic API (default: `false`)

## Project Structure

```
claude-code-api-proxy/
├── cmd/
│   └── cc-api-proxy/
│       └── main.go           # Entry point
├── internal/
│   ├── config/               # Config loading
│   ├── daemon/               # Process management
│   ├── server/               # HTTP server (Fiber)
│   └── converter/            # Claude ↔ OpenAI conversion
├── pkg/
│   └── models/               # Shared types
├── scripts/
│   └── ccp                   # Shell wrapper
└── Makefile                  # Build automation
```

## Supported Claude Code Features

The proxy fully supports all Claude Code features:

- **Tool Calling** - Complete support for all Claude Code tools
  - File operations: `read`, `write`, `edit`
  - Search operations: `glob`, `grep`
  - Shell execution: `bash`
  - Task management: `todowrite`, `todoread`
  - And all other Claude Code tools

- **Extended Thinking** - Proper thinking block support
  - Thinking blocks are properly formatted and hidden in Claude Code UI
  - Shows "Thought for Xs" indicator instead of full content
  - Can be revealed with Ctrl+O in Claude Code
  - Supports signature_delta events for authentication

- **Streaming** - Real-time streaming responses
  - Proper SSE (Server-Sent Events) formatting
  - Accurate token usage tracking
  - Low latency streaming from backend models

- **Token Tracking** - Full usage metrics
  - Input tokens counted accurately
  - Output tokens tracked in real-time
  - Cache metrics supported (when using Anthropic backend)

## Development

```bash
# Run in dev mode
go run cmd/cc-api-proxy/main.go

# Run tests
go test ./...
# Or with verbose output
go test -v ./internal/converter

# Run specific test
go test -v ./internal/converter -run TestConvertMessagesWithComplexContent

# Format code
go fmt ./...

# Lint (requires golangci-lint)
golangci-lint run
```

## Testing

### Unit Tests

The project includes comprehensive unit tests:

```bash
# Run all tests
go test ./...

# Run converter tests (includes tool calling tests)
go test -v ./internal/converter

# Run with coverage
go test -cover ./...
```

### Manual Testing

Test the proxy with Claude Code CLI:

```bash
# Start proxy in background
./cc-api-proxy -s &

# Test with different model tiers
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model opus -p "hi"
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model sonnet -p "hi"
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model haiku -p "hi"

# Check proxy logs
./cc-api-proxy status
tail -f /tmp/cc-api-proxy.log

# Stop proxy
./cc-api-proxy stop
```

See [CLAUDE.md](CLAUDE.md#manual-testing) for detailed testing instructions including tool calling, streaming, and provider-specific tests.

## How It Works

1. **Request Flow**:
   - Claude Code sends Claude API format request to proxy
   - Proxy converts Claude format → OpenAI format
   - Proxy routes to OpenRouter/OpenAI/other provider
   - Provider returns OpenAI format response
   - Proxy converts back to Claude format
   - Claude Code receives properly formatted response

2. **Format Conversion**:
   - Claude's `tool_use` blocks → OpenAI's `tool_calls` format
   - OpenAI's `reasoning_details` → Claude's `thinking` blocks
   - Maintains proper tool_use ↔ tool_result correspondence
   - Preserves all metadata and signatures

3. **Streaming**:
   - Converts OpenAI SSE chunks to Claude SSE events
   - Generates proper event sequence (message_start, content_block_start, deltas, etc.)
   - Tracks content block indices for proper Claude Code rendering

## Adaptive Per-Model Detection

The proxy uses a fully adaptive system that automatically learns what parameters each model supports, eliminating the need for hardcoded model patterns or provider-specific configuration.

### How It Works

**Philosophy:** Support all provider quirks automatically - never burden users with configurations they don't understand.

1. **First Request** (Cache Miss):
   ```
   [DEBUG] Cache MISS: gpt-5 → will auto-detect (try max_completion_tokens)
   ```
   - Proxy tries sending `max_completion_tokens` (correct for reasoning models)
   - If provider returns "unsupported parameter" error, automatically retries without it
   - Result is cached per `(provider, model)` combination

2. **Subsequent Requests** (Cache Hit):
   ```
   [DEBUG] Cache HIT: gpt-5 → max_completion_tokens=true
   ```
   - Proxy uses cached knowledge immediately
   - No trial-and-error needed
   - Instant parameter selection

### Benefits

- **Zero Configuration** - No need to know which parameters each provider supports
- **Future-Proof** - Works with any new model/provider without code changes
- **Fast** - Only 1-2 second penalty on first request, instant thereafter
- **Provider-Agnostic** - Automatically adapts to OpenRouter, OpenAI Direct, Ollama, OpenWebUI, or any OpenAI-compatible provider
- **Per-Model Granularity** - Same model name on different providers cached separately

### Cache Details

**What's Cached:**
```go
CacheKey{
    BaseURL: "https://openwebui.example.com/api",  // Provider
    Model:   "gpt-5"                      // Model name
}
→ ModelCapabilities{
    UsesMaxCompletionTokens: false,       // Learned capability
    LastChecked:             time.Now()   // Timestamp
}
```

**Cache Scope:**
- In-memory only (cleared on proxy restart)
- Thread-safe (protected by `sync.RWMutex`)
- Per (provider, model) combination
- Visible in debug logs (`-d` flag)

### Example: OpenWebUI

When using OpenWebUI (which has a quirk with `max_completion_tokens`):

| Request | What Happens | Duration |
|---------|--------------|----------|
| 1st | Try max_completion_tokens → Error → Retry without it | ~2 seconds |
| 2nd+ | Use cached knowledge (no retry) | < 100ms |

**No configuration needed** - the proxy learns and adapts automatically.

### Debug Logging

Enable debug mode to see cache activity:

```bash
./cc-api-proxy -d -s

# Logs show:
# [DEBUG] Cache MISS: gpt-5 → will auto-detect (try max_completion_tokens)
# [DEBUG] Cached: model gpt-5 supports max_completion_tokens
# [DEBUG] Cache HIT: gpt-5 → max_completion_tokens=true
```

## License

[MIT License](LICENSE)
