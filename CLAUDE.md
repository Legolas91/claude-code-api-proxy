# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`cc-api-proxy` is an HTTP proxy that translates Claude API requests to OpenAI-compatible format, enabling Claude Code to work with any OpenAI-compatible backend — OpenRouter, OpenAI Direct, Ollama, Mammouth.ai, or any enterprise API gateway. Each Claude tier (Opus/Sonnet/Haiku) can be independently routed to a different provider URL and API key. The proxy runs as a daemon, performs bidirectional API format conversion, and maintains full Claude Code feature compatibility including tool calling, extended thinking blocks, and streaming.

## Build Commands

```bash
# Build the binary
go build -o cc-api-proxy cmd/cc-api-proxy/main.go
# Or use make
make build

# Build for all platforms (creates dist/ folder)
make build-all

# Run tests
go test ./...

# Run specific test file
go test -v ./internal/converter

# Run single test
go test -v ./internal/converter -run TestConvertMessagesWithComplexContent

# Run tests with coverage
make test-coverage

# Format code
go fmt ./...

# Compile and start proxy in simple log mode
go build -o cc-api-proxy cmd/cc-api-proxy/main.go && ./cc-api-proxy -s
```

## Architecture

### Core Request Flow

1. **Claude Code** → sends Claude API format request to `localhost:8082`
2. **handlers.go** → receives `/v1/messages` POST request
3. **converter.go** → transforms Claude format → OpenAI format
   - Detects provider type (OpenRouter/OpenAI/Ollama) via `cfg.DetectProvider()`
   - Applies provider-specific parameters (reasoning format, tool_choice)
   - Maps Claude model name to target provider model using pattern-based routing
4. **handlers.go** → forwards OpenAI request to configured provider
5. **Provider** → returns OpenAI-format response (streaming or non-streaming)
6. **converter.go** → transforms OpenAI format → Claude format
7. **handlers.go** → returns Claude-format response to Claude Code

### Provider-Specific Behavior

The proxy applies different request parameters based on `OPENAI_BASE_URL`:

**OpenRouter** (`https://openrouter.ai/api/v1`):
- Adds `reasoning: {enabled: true}` for thinking support
- Uses `usage: {include: true}` for token tracking
- Extracts `reasoning_details` array → converts to Claude `thinking` blocks

**OpenAI Direct** (`https://api.openai.com/v1`):
- Adds `reasoning_effort: "medium"` for GPT-5 reasoning models
- Uses standard `stream_options: {include_usage: true}`

**Ollama** (`http://localhost:*`):
- Sets `tool_choice: "required"` when tools are present (forces tool usage)
- No API key validation (localhost endpoints skip auth)

### Format Conversion Details

**Tool Calling** (`convertMessages` in converter.go):
- Claude `tool_use` content blocks → OpenAI `tool_calls` array
- OpenAI `tool_calls` → Claude `tool_use` blocks
- Maintains `tool_use.id` ↔ `tool_result.tool_use_id` correspondence
- Preserves JSON arguments as strings during conversion

**Thinking Blocks** (`ConvertResponse` in converter.go):
- OpenRouter `reasoning_details` → Claude `thinking` block with `signature` field
- `signature` field is REQUIRED for Claude Code to hide/show thinking properly
- Without signature, thinking appears as regular text in chat

**Streaming** (`streamOpenAIToClaude` in handlers.go):
- Converts OpenAI SSE chunks (`data: {...}`) → Claude SSE events
- Generates proper event sequence: `message_start`, `content_block_start`, `content_block_delta`, `content_block_stop`, `message_delta`, `message_stop`
- Tracks content block indices to maintain proper ordering
- Handles tool call deltas by accumulating function arguments across chunks

### Pattern-Based Model Routing

The `mapModel()` function in converter.go implements intelligent routing:

```go
// Haiku tier → lightweight models
"*haiku*" → gpt-5-mini (or ANTHROPIC_DEFAULT_HAIKU_MODEL)

// Sonnet tier → version-aware
"*sonnet-4*" or "*sonnet-5*" → gpt-5
"*sonnet-3*" → gpt-4o
(or ANTHROPIC_DEFAULT_SONNET_MODEL)

// Opus tier → flagship models
"*opus*" → gpt-5 (or ANTHROPIC_DEFAULT_OPUS_MODEL)
```

Override via environment variables to route to alternative models (Grok, Gemini, DeepSeek-R1, etc.).

###  Adaptive Per-Model Capability Detection

**Core Philosophy**: Support all provider quirks automatically - never burden users with advance configs.

The proxy uses a fully adaptive system that automatically learns what parameters each model supports through error-based retry and caching. This eliminates ALL hardcoded model patterns (~100 lines removed in v1.3.0).

**How It Works:**

1. **First Request (Cache Miss)**:
   - `ShouldUseMaxCompletionTokens()` checks cache for `CacheKey{BaseURL, Model}`
   - Cache miss → defaults to trying `max_completion_tokens` (correct for reasoning models)
   - If provider returns "unsupported parameter" error, `retryWithoutMaxCompletionTokens()` is called
   - Retry succeeds → cache `{UsesMaxCompletionTokens: false}`
   - Original request succeeds → cache `{UsesMaxCompletionTokens: true}`

2. **Subsequent Requests (Cache Hit)**:
   - `ShouldUseMaxCompletionTokens()` returns cached value immediately
   - No trial-and-error needed
   - ~1-2 second first request penalty, instant thereafter

**Cache Structure** (`internal/config/config.go:29-48`):

```go
type CacheKey struct {
    BaseURL string  // Provider base URL (e.g., "https://api.mammouth.ai/v1")
    Model   string  // Model name (e.g., "gpt-5")
}

type ModelCapabilities struct {
    UsesMaxCompletionTokens bool      // Learned via adaptive retry
    LastChecked             time.Time // Timestamp
}

// Global cache: map[CacheKey]*ModelCapabilities
// Protected by sync.RWMutex for thread-safety
```

**Error Detection** (`internal/server/handlers.go:895-913`):

```go
func isMaxTokensParameterError(errorMessage string) bool {
    errorLower := strings.ToLower(errorMessage)

    // Broad keyword matching (no status code restriction)
    hasParamIndicator := strings.Contains(errorLower, "parameter") ||
                        strings.Contains(errorLower, "unsupported") ||
                        strings.Contains(errorLower, "invalid")

    hasOurParam := strings.Contains(errorLower, "max_tokens") ||
                   strings.Contains(errorLower, "max_completion_tokens")

    return hasParamIndicator && hasOurParam
}
```

**Debug Logging**:

Start proxy with `-d` flag to see cache activity:

```bash
./cc-api-proxy -d -s

# Console output shows:
[DEBUG] Cache MISS: gpt-5 → will auto-detect (try max_completion_tokens)
[DEBUG] Cached: model gpt-5 supports max_completion_tokens (streaming)
[DEBUG] Cache HIT: gpt-5 → max_completion_tokens=true
```

**Key Benefits**:

- **Future-proof**: Works with any new model/provider without code changes
- **Zero user config**: No need to know which parameters each provider supports
- **Per-model granularity**: Same model name on different providers cached separately
- **Thread-safe**: Protected by `sync.RWMutex` for concurrent requests
- **In-memory**: Cleared on restart (first request re-detects)

**What Was Removed** (v1.3.0):

- `IsReasoningModel()` function (30 lines) - checked for gpt-5/o1/o2/o3/o4 patterns
- `FetchReasoningModels()` function (56 lines) - OpenRouter API calls
- `ReasoningModelCache` struct (11 lines) - per-provider reasoning model lists
- Provider-specific hardcoding for Unknown provider type
- ~100 lines total removed, replaced with ~30 lines of adaptive detection

## Configuration System

Config loading priority (see `internal/config/config.go`):
1. `./.env` (local project override)
2. `~/.claude/proxy.env` (recommended location)
3. `~/.cc-api-proxy` (legacy location)

Uses `godotenv.Overload()` to allow later files to override earlier ones.

### Per-Tier Routing (v1.5.0+)

Each Claude tier (Opus/Sonnet/Haiku) can be independently routed to a different provider URL and API key. If a tier-specific variable is not set, it falls back to `OPENAI_BASE_URL` / `OPENAI_API_KEY`.

```bash
# Tier-specific base URL (v1.4.0+)
ANTHROPIC_DEFAULT_OPUS_BASE_URL=https://openrouter.ai/api/v1
ANTHROPIC_DEFAULT_SONNET_BASE_URL=https://api.mammouth.ai/v1
ANTHROPIC_DEFAULT_HAIKU_BASE_URL=http://localhost:11434/v1

# Tier-specific API key (v1.5.0+)
ANTHROPIC_DEFAULT_OPUS_API_KEY=sk-or-v1-...
ANTHROPIC_DEFAULT_SONNET_API_KEY=sk-mmai-...
ANTHROPIC_DEFAULT_HAIKU_API_KEY=  # not needed for Ollama

# Tier-specific model override
ANTHROPIC_DEFAULT_OPUS_MODEL=openai/gpt-5
ANTHROPIC_DEFAULT_SONNET_MODEL=codestral-2508
ANTHROPIC_DEFAULT_HAIKU_MODEL=llama3.1:8b
```

Routing logic is implemented in `config.GetProviderForTier(tier)` which returns `(baseURL, apiKey, model)` with automatic fallback.

Provider detection via URL pattern matching in `DetectProvider()`:
- Contains `openrouter.ai` → ProviderOpenRouter
- Contains `api.openai.com` → ProviderOpenAI
- Contains `localhost` or `127.0.0.1` → ProviderOllama
- Otherwise → ProviderUnknown

### Enterprise HTTP Proxy Support

The proxy supports HTTP/HTTPS proxy configuration for corporate environments where internet access requires going through a proxy.

**Configuration variables:**

```bash
# Custom proxy (override system proxy)
CLAUDE_HTTP_PROXY=http://proxy.company.com:8080
CLAUDE_HTTPS_PROXY=http://proxy.company.com:8080
CLAUDE_NO_PROXY=localhost,127.0.0.1,.internal,.local

# Control system proxy usage (default: true)
CLAUDE_PROXY_FROM_ENV=true  # Use HTTP_PROXY/HTTPS_PROXY if set
CLAUDE_PROXY_FROM_ENV=false # Ignore system proxy completely
```

**Priority order:**
1. `CLAUDE_HTTP_PROXY` / `CLAUDE_HTTPS_PROXY` (highest - custom override)
2. `HTTP_PROXY` / `HTTPS_PROXY` if `CLAUDE_PROXY_FROM_ENV=true` (default)
3. No proxy if all disabled

**NO_PROXY patterns:**
- Exact match: `localhost`, `127.0.0.1`
- Domain suffix: `.company.com` matches `api.company.com`
- Suffix without dot: `internal` matches `api.internal`
- Wildcard: `*` bypasses all hosts

**Implementation:**
- `Config.GetHTTPTransport()` creates `http.Transport` with proxy function
- `shouldBypassProxy()` implements NO_PROXY pattern matching
- `makeOpenAIHTTPRequest()` uses configured transport for all provider requests
- Uses `req.URL.Hostname()` to extract hostname without port for matching

**Use cases:**
- Corporate proxy required for all internet access
- Different proxy for LLM providers (less restrictive proxy)
- Bypass proxy for local services (Ollama on localhost)
- Hybrid setup: cloud providers via proxy, local models direct

**Testing:**
- `TestHTTPProxyConfiguration` validates config loading
- `TestGetHTTPTransport` validates proxy routing logic
- `TestShouldBypassProxy` validates NO_PROXY pattern matching

## Testing Strategy

The test suite has two main categories:

**Provider Tests** (`internal/converter/provider_test.go`):
- Verify provider-specific request parameters are correct
- Ensure OpenRouter gets `reasoning: {enabled: true}` not `reasoning_effort`
- Ensure OpenAI Direct gets `reasoning_effort` not `reasoning` object
- Ensure Ollama gets `tool_choice: "required"` when tools present
- Test provider isolation (no cross-contamination of parameters)

**Conversion Tests** (`internal/converter/converter_test.go`):
- Test Claude → OpenAI message conversion
- Test tool calling format conversion
- Test thinking block extraction from reasoning_details
- Test streaming chunk aggregation

When adding new provider support, create tests in `provider_test.go` following the existing pattern.

## Manual Testing

To manually test the proxy with Claude Code CLI:

### 1. Start the proxy in background

```bash
# Build first
go build -o cc-api-proxy cmd/cc-api-proxy/main.go

# Start in simple log mode (recommended for testing)
./cc-api-proxy -s &

# Or with debug logging
./cc-api-proxy -d &

# Check it's running
./cc-api-proxy status
```

### 2. Test with different Claude model tiers

The proxy routes Claude model names to your configured backend models:

```bash
# Test with Opus tier (routes to ANTHROPIC_DEFAULT_OPUS_MODEL or gpt-5)
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model opus -p "hi"

# Test with Sonnet tier (routes to ANTHROPIC_DEFAULT_SONNET_MODEL or gpt-5)
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model sonnet -p "hi"

# Test with Haiku tier (routes to ANTHROPIC_DEFAULT_HAIKU_MODEL or gpt-5-mini)
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model haiku -p "hi"
```

### 3. Verify routing

Check the proxy logs to see which backend model was used:

```bash
# Simple log mode shows:
# [REQ] https://openrouter.ai/api/v1 model=openai/gpt-5 in=20 out=5 tok/s=25.3

# Debug mode shows full request/response JSON
tail -f /tmp/cc-api-proxy.log
```

### 4. Test tool calling

```bash
# Test with a prompt that triggers tool usage
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model sonnet -p "list files in current directory"

# Should see tool_calls in debug logs
# Verify proper Claude tool_use → OpenAI tool_calls → Claude tool_result conversion
```

### 5. Test streaming and thinking blocks

```bash
# Test with reasoning model (should show thinking blocks)
# Configure .env with: ANTHROPIC_DEFAULT_SONNET_MODEL=openai/gpt-5
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model sonnet -p "solve: 2x + 5 = 15"

# Should show thinking process in Claude Code UI
# Verify reasoning_details → thinking block conversion in logs
```

### 6. Stop the proxy

```bash
./cc-api-proxy stop
```

### Testing Different Providers

**OpenRouter:**
```bash
# .env or ~/.claude/proxy.env
OPENAI_BASE_URL=https://openrouter.ai/api/v1
OPENAI_API_KEY=sk-or-v1-...
ANTHROPIC_DEFAULT_SONNET_MODEL=openai/gpt-5

# Test
./cc-api-proxy -s &
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model sonnet -p "hi"
```

**OpenAI Direct:**
```bash
# .env
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=sk-proj-...

# Test with reasoning model
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model opus -p "think through this problem"
```

**Ollama (Local):**
```bash
# .env
OPENAI_BASE_URL=http://localhost:11434/v1
ANTHROPIC_DEFAULT_SONNET_MODEL=qwen2.5:14b

# Start Ollama first
ollama serve &

# Test proxy
./cc-api-proxy -s &
ANTHROPIC_BASE_URL=http://localhost:8082 claude --model sonnet -p "hi"
```

## Simple Log Mode

The `-s` or `--simple` flag enables one-line request summaries:

```
[REQ] <base_url> model=<provider_model> in=<tokens> out=<tokens> tok/s=<rate>
```

Implementation:
- Track `startTime := time.Now()` at request start
- Extract token counts from response usage data
- Calculate throughput: `tokensPerSec = float64(outputTokens) / duration`
- Output in both streaming (`streamOpenAIToClaude`) and non-streaming handlers

Token extraction requires `float64 → int` conversion because JSON unmarshals numbers as float64.

## Common Pitfalls

1. **Tool arguments must be strings**: OpenAI expects `arguments: "{\"key\":\"value\"}"` not `arguments: {key: "value"}`

2. **Thinking blocks need signature field**: Without `signature: "..."` field, Claude Code shows thinking as plain text instead of hiding it

3. **Provider parameter isolation**: Never mix OpenRouter `reasoning` object with OpenAI `reasoning_effort` parameter - detection logic in `ConvertRequest()` ensures this

4. **Streaming index tracking**: Content blocks must maintain consistent indices across SSE events - use state struct to track current index

5. **Token count type conversion**: Always convert JSON number types to int when extracting from maps: `int(val.(float64))`

## Daemon Process

The proxy runs as a background daemon (see `internal/daemon/daemon.go`):
- Creates PID file at `/tmp/cc-api-proxy.pid`
- Redirects stdout/stderr to `/tmp/cc-api-proxy.log`
- `./cc-api-proxy status` checks if process is running
- `./cc-api-proxy stop` kills the daemon via PID file

When testing locally, use `-d` flag for debug logging to see full requests/responses.

## Package Structure

- `cmd/cc-api-proxy/main.go` - Entry point, CLI arg parsing
- `internal/config/` - Environment variable loading, provider detection
- `internal/converter/` - Claude ↔ OpenAI format conversion logic
- `internal/server/` - HTTP server (Fiber), request handlers, streaming
- `internal/daemon/` - Process management, PID file handling
- `pkg/models/` - Shared type definitions for Claude and OpenAI formats
- `scripts/ccp` - Wrapper script that starts daemon and execs Claude Code

## Key Files

- `internal/converter/converter.go:ConvertRequest()` - Claude → OpenAI request conversion with provider-specific parameters
- `internal/converter/converter.go:ConvertResponse()` - OpenAI → Claude response conversion, thinking block extraction
- `internal/server/handlers.go:streamOpenAIToClaude()` - SSE chunk conversion, event generation
- `internal/config/config.go:DetectProvider()` - URL-based provider detection
- `pkg/models/types.go` - All request/response type definitions
