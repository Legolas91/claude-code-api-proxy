# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.5.21] - 2026-04-04

### Fixed
- **Cache key includes provider base URL** — `ComputeKey` now takes the resolved `baseURL` as a parameter, ensuring that the same request routed to different providers (via per-tier multi-URL config) produces distinct cache keys. Previously, switching model tier (e.g. `/model haiku` → `/model sonnet`) could return a cached response from the wrong provider
- **Cache lookup moved after tier resolution** — the provider tier (`GetProviderForTier`) is now resolved before the cache check, so `baseURL` is available for the cache key

### Added
- **`--help` documents response cache** — added `PROXY_CACHE_ENABLED`, `PROXY_CACHE_MAX_ENTRIES`, and `PROXY_CACHE_MAX_TEMPERATURE` to the CLI help output
- **`TestComputeKey_DifferentBaseURL`** — new unit test verifying that different base URLs produce different cache keys

## [1.5.20] - 2026-04-04

### Added
- **Response cache** (`internal/cache/`) — opt-in in-memory LRU cache for non-streaming, deterministic responses
  - `Store` interface enables future backend swap (Redis, SQLite) without touching handler code
  - `MemoryStore`: LRU eviction using `container/list` (stdlib), thread-safe via `sync.Mutex`, O(1) get/set
  - `ComputeKey`: SHA-256 of canonical request fields (model, system, messages, tools, tool_choice, temperature, max_tokens, top_p, stop_sequences) — excludes `stream`
  - Streaming requests and requests with `temperature > PROXY_CACHE_MAX_TEMPERATURE` are never cached
  - `X-Cache: HIT` / `X-Cache: MISS` debug headers on eligible responses
  - `[CACHE] HIT key=...` / `[CACHE] STORE key=...` log lines in simple log mode (`-s`)
  - ProviderClaudeCode non-streaming path is also cacheable (key computed before provider routing)
  - Zero new external dependencies

### Configuration
- `PROXY_CACHE_ENABLED=false` — opt-in (default: disabled)
- `PROXY_CACHE_MAX_ENTRIES=100` — LRU eviction threshold
- `PROXY_CACHE_MAX_TEMPERATURE=0` — only cache requests with `temperature <= value`

### Fixed
- **Config test isolation** — added `TestMain` in `internal/config/config_test.go` that redirects `HOME` to a tmpdir, preventing `godotenv.Overload()` from loading `~/.claude/proxy.env` and corrupting test env vars

## [1.5.19] - 2026-04-03

### Fixed
- **ProviderClaudeCode: pass prompt via stdin** — avoids `ARG_MAX` limit on long prompts by piping the prompt through stdin instead of passing it as a CLI argument
- **Streaming: add `--verbose` flag** — `stream-json` output format requires `--verbose` for structured event output

## [1.5.18] - 2026-04-01

### Added
- **Build commit hash in API responses** — `GET /` and `GET /health` now include a `commit` field with the short git SHA injected at build time via `-ldflags`
  - Eliminates ambiguity when rebuilding a binary with the same version tag
  - Injected via `internal/version.Commit` (defaults to `"unknown"` if not set)
  - Updated `Makefile` and `release.yml` to inject `Commit` alongside `Version`

## [1.5.17] - 2026-04-01

### Added
- **`claude -p` backend** (`internal/server/claudecode.go`) — new backend type that spawns `claude -p` instead of calling an HTTP API, enabling use of a Pro/Max Claude subscription without an API key
  - Activated automatically when `OPENAI_BASE_URL=https://api.anthropic.com` and no API key is set
  - New `ProviderClaudeCode` provider type in `config.go`
  - Supports both streaming and non-streaming modes
  - Parses structured JSON output from `claude --output-format json`
  - Renamed throughout from `claudep` → `claudecode` (files, types, functions) for clarity

## [1.5.16] - 2026-03-30

### Changed
- **`--help` output updated** — documents all v1.5.15 env vars introduced in the previous release but missing from the built-in help
  - New section "Tool reliability": `PROXY_AUGMENT_TOOL_PROMPT`, `PROXY_TOOL_PROMPT_TEMPLATE`, `PROXY_REPAIR_TOOL_CALLS`, `PROXY_MAX_LOOP_LEVEL`
  - New section "Loop detection": `PROXY_MAX_IDENTICAL_RETRIES`

## [1.5.15] - 2026-03-30

### Added
- **Tool choice filtering** — when `tool_choice` targets a specific tool, only that tool is sent to the backend, reducing model confusion
  - Claude `{"type":"tool","name":"X"}` → OpenAI `{"type":"function","function":{"name":"X"}}` with filtered tools list
  - Claude `"any"` → OpenAI `"required"` (forces tool usage)
  - New `filterToolsForChoice()` in converter.go with safe type assertions (no panic on malformed input)
  - Fixed: `ToolChoice` field was missing from `ClaudeRequest`, silently dropping all tool_choice directives
  - Fixed: Ollama `tool_choice = "required"` was only applied in streaming path; now applied to both streaming and non-streaming
- **Tool-result 400 retry with message flattening** — workaround for vLLM backends that reject multi-turn `role:"tool"` messages with HTTP 400
  - On 400 errors with tool_results present, retries with messages flattened to plain text (assistant tool_calls → text, tool results → user messages)
  - If flattened retry also fails, second retry strips all tools entirely
  - Applies to both streaming (`callOpenAIStream`) and non-streaming (`callOpenAI`) paths
  - Logs `[WARN] Backend 400 on tool_result` on first retry
  - Non-breaking: backends that correctly support tool_results are unaffected
- **Tool prompt augmentation** — prepends model-specific tool-use instructions to the system prompt for enterprise LLM gateways (vLLM/Mistral) that sometimes emit tool calls as plain text
  - Auto-enabled for `ProviderUnknown` (e.g. Thales GenAI), disabled for OpenRouter/OpenAI/Ollama
  - Per-model guidance for `codestral*`, `mistral-medium*`/`mistral-large*`, `mistral-small*`, default
  - `PROXY_AUGMENT_TOOL_PROMPT=true/false` overrides auto-detection
  - `PROXY_TOOL_PROMPT_TEMPLATE=<text>` overrides per-model guidance
- **JSON repair** — fixes malformed tool call arguments in non-streaming responses
  - Handles: markdown code blocks (` ```json {...} ``` `), XML `<arguments>` tags, missing IDs, extra whitespace
  - `PROXY_REPAIR_TOOL_CALLS=false` disables (default: enabled)
- **Loop breaker escalation** — extends v1.5.14 loop detection with severity levels
  - Level 1 (gentle): `NudgeMessage` — try a different approach
  - Level 2 (strong): `StrongNudgeMessage` — you must change strategy
  - Level 3 (disable tools): tools are removed from the request to force text-based approach
  - `PROXY_MAX_LOOP_LEVEL=1` or `2` caps the max level (default: 3)
- **Tool telemetry** — `[TOOL]` log line when tools are used (simple log mode)
  - Format: `[HH:MM:SS] [TOOL] model=X sent=N used=M name=tool1,tool2`
  - Emitted in both streaming and non-streaming paths

### Changed
- `streamOpenAIToClaude()` signature: added `toolsSent int` parameter for telemetry

## [1.5.14] - 2026-03-28

### Added
- **Retry loop detection** — detects and breaks infinite tool-call retry loops
  - New `internal/loop/` package: scans conversation for N consecutive identical `tool_use` calls
  - Injects a user nudge message to force the model to try a different approach
  - Configurable via `PROXY_MAX_IDENTICAL_RETRIES` env var (default: 3, 0 = disabled)
  - Logs loop detection in simple mode (`[LOOP]`) and debug mode (`[DEBUG]`)
  - 9 unit tests covering all scenarios (threshold, different inputs/tools, disabled, etc.)
- **Integration test** (Test 9) in `test-proxy.sh` — simulates a 3-call retry loop and verifies nudge injection

## [1.5.13] - 2026-03-23

### Added
- **Gitea Actions CI/CD** for bare-metal Windows runner
  - `ci.yml`: test + build + golangci-lint on push/PR to main
  - `release.yml`: cross-platform build (5 targets) + checksums + Gitea API release on tag
- Pure PowerShell workflows — no JS actions, no bash, no Node.js dependency

### Changed
- Updated golangci-lint config: exclude gosec false positives G101/G304/G703
- golangci-lint upgraded to v2.11.3 (Go 1.26.1 compatible)

## [1.5.12] - 2026-03-18

### Added
- **Rate limiting** via `RATE_LIMIT_RPM` env var (default 0 = disabled); uses `fiber/middleware/limiter` with global bucket and Claude-format 429 response
- **Security headers** via `fiber/middleware/helmet`: X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Content-Security-Policy, X-DNS-Prefetch-Control
- Health endpoint now exposes `claude_code_version` when Claude Code CLI is detected at startup
  - Detection via `claude --version`, fallback to npm package.json
  - Field omitted if detection fails — proxy starts normally regardless
  - Example: `{"status":"ok","version":"1.5.12","claude_code_version":"2.1.78"}`

### Security
- **Timing-safe API key validation**: replaced string equality with `crypto/subtle.ConstantTimeCompare` to prevent timing attacks

### Changed
- Removed GitHub Actions workflows — migrating to Gitea Actions
- Moved `RELEASE_TEMPLATE.md` and `RELEASE_WORKFLOW.md` from `.github/` to `docs/`
- README: updated Go badge to 1.26, removed CI badge

## [1.5.11] - 2026-03-16

### Security
- **Go toolchain**: Upgraded from 1.25.5 to 1.26.1
  - Fix CVE-2025-68121: crypto/tls unexpected session resumption (CRITICAL)
  - Fix CVE-2025-61726: net/url memory exhaustion (HIGH)
  - Fix CVE-2025-61728: archive/zip excessive CPU consumption (HIGH)
  - All binaries now compiled with Go 1.26.1 stdlib

### Stats
- **Files changed**: 1 (go.mod)
- **Tests**: all non-Windows tests passed ✅

## [1.5.10] - 2026-03-15

### Security
- **gofiber/fiber/v2**: Upgraded from v2.52.9 to v2.52.11
  - Fix CVE-2025-66630: Predictable UUIDs from randomness errors
  - Only affects localhost:8082 binding — actual risk was negligible
- **klauspost/compress**: Upgraded from v1.18.1 to v1.18.4 (retracted version replaced)

### Stats
- **Files changed**: 2 (go.mod, go.sum)
- **Tests**: all non-Windows tests passed ✅

## [1.5.9] - 2026-03-01

### Fixed
- **`--help` and `version` command** - Displayed old binary name `claude-code-proxy` instead of `cc-api-proxy`
- **`.env.example`** - Legacy config path referenced `~/.claude-code-proxy` instead of `~/.cc-api-proxy`
- **`.env.example`** - OpenRouter app URL referenced `claude-code-proxy` repository instead of `claude-code-api-proxy`
- **`scripts/ccp`** - Wrapper script referenced old `claude-code-proxy` binary name instead of `cc-api-proxy`
- **`Makefile`** - Version not injected at build time (`--version` displayed `dev`); fixed via `git describe --tags`
- **`release.yml`** - Heredoc `<< 'EOF'` prevented `${VERSION}` expansion in release notes installation instructions

### Added
- **`.env.example`** - New "Per-Tier Routing (v1.5.0+)" section documenting all 9 tier-specific variables (`_BASE_URL`, `_API_KEY`, `_MODEL` for Opus/Sonnet/Haiku)
- **`CLAUDE.md`** - New "Per-Tier Routing (v1.5.0+)" section in Configuration System documenting `GetProviderForTier()` and all tier-specific variables
- **`CONTRIBUTING.md`** - Single-maintainer policy (no PRs accepted, issues welcome)
- **`SECURITY.md`** - Vulnerability reporting policy via GitHub Security Advisories
- **`.gitignore`** - Added `*.pem`, `*.key`, `*.crt`, `*.p12`, `*.pfx` to prevent accidental certificate commits
- **README badges** - Release version, Go version, CI status, License, Issues

### Changed
- Repository is now **public** on GitHub (`Legolas91/claude-code-api-proxy`)

## [1.5.7] - 2026-02-28

### Changed
- Renamed source directory `cmd/claude-code-proxy/` to `cmd/cc-api-proxy/` to align with binary name
- Makefile `CMD_PATH` now derived from `BINARY` name: `cmd/$(BINARY)/main.go`

## [1.5.6] - 2026-02-28

### Security
- **Replace Fiber CORS with custom middleware** - Critical fix: previous CORS implementation allowed all origins unconditionally
  - Custom middleware validates `Origin` header against allowed origins
  - Restricts to `localhost` and `127.0.0.1` by default
  - Proper `OPTIONS` preflight handling

### Changed
- Binary renamed from `claude-code-proxy` to `cc-api-proxy`
- PID file: `/tmp/claude-code-proxy.pid` → `/tmp/cc-api-proxy.pid`
- Log file: `/tmp/claude-code-proxy.log` → `/tmp/cc-api-proxy.log`
- Legacy config path: `~/.claude-code-proxy` → `~/.cc-api-proxy`
- All release assets now named `cc-api-proxy-*`

## [1.5.5] - 2026-02-14

### Added
- **Enterprise HTTP proxy support** - Route outbound requests through corporate HTTP/HTTPS proxies
  - `CLAUDE_HTTP_PROXY` - Custom HTTP proxy URL (overrides system `HTTP_PROXY`)
  - `CLAUDE_HTTPS_PROXY` - Custom HTTPS proxy URL (overrides system `HTTPS_PROXY`)
  - `CLAUDE_NO_PROXY` - Comma-separated bypass list (exact, `.suffix`, `suffix`, `*` patterns)
  - `CLAUDE_PROXY_FROM_ENV` - Enable/disable system proxy auto-detection (default: `true`)
- `Config.GetHTTPTransport()` - Creates `http.Transport` with proxy configuration
- `shouldBypassProxy()` - NO_PROXY pattern matching (exact, domain suffix, wildcard)
- 17 unit tests: `TestHTTPProxyConfiguration`, `TestGetHTTPTransport`, `TestShouldBypassProxy`

### Changed
- `makeOpenAIHTTPRequest()` now uses `cfg.GetHTTPTransport()` for all provider requests

## [1.5.3] - 2026-02-14

### Added
- Enhanced daemon test coverage: 40% → 49%
- Edge case tests for PID handling, concurrent operations, invalid PID file scenarios
- `workflow_dispatch` trigger for manual CI runs

### Fixed
- GitHub Actions workflows updated to Go 1.25 (matching `go.mod` requirement)
- CI cache key includes Go version to prevent cache corruption

## [1.5.2] - 2026-02-14

### Security
- Fix G306: PID file permissions changed from `0644` to `0600` (owner-only read/write)
- Fix G107/G304: Added `#nosec` with justifications for controlled localhost health check and PID file path

### Fixed
- `TestStart` now skips when proxy is already running on `localhost:8082` (prevents CI failures)

## [1.5.0] - 2026-02-14

### Added
- **Per-tier API key support** - Configure different API keys for each Claude tier (Opus/Sonnet/Haiku)
  - `ANTHROPIC_DEFAULT_OPUS_API_KEY` - API key for opus tier requests
  - `ANTHROPIC_DEFAULT_SONNET_API_KEY` - API key for sonnet tier requests
  - `ANTHROPIC_DEFAULT_HAIKU_API_KEY` - API key for haiku tier requests
  - Falls back to `OPENAI_API_KEY` when tier-specific key is not configured
  - Enables multi-provider routing with different authentication per tier
- `GetProviderForTier(tier)` method on Config - Returns (baseURL, apiKey, model) for a given tier with automatic fallback
- `GetTierFromModel(claudeModel)` function in converter - Extracts tier name from Claude model strings
- 7 comprehensive unit tests for `GetProviderForTier()` covering all fallback scenarios
- Support for hybrid configurations (mix tier-specific and fallback values)

### Changed
- `makeOpenAIHTTPRequest()` now accepts `baseURL` and `apiKey` as parameters instead of using `cfg.OpenAIBaseURL` and `cfg.OpenAIAPIKey` globally
- `handleMessages()` extracts tier from Claude model and calls `GetProviderForTier()` to get appropriate config
- `callOpenAI()`, `callOpenAIStream()`, `callOpenAIInternal()`, `callOpenAIStreamInternal()` all thread `baseURL` and `apiKey` parameters
- `handleStreamingMessages()` accepts `baseURL` and `apiKey` parameters for per-tier routing
- Localhost detection moved from `cfg.IsLocalhost()` to inline check in `makeOpenAIHTTPRequest()` using provided `baseURL`

### Fixed
- Variable name consistency: All tier-specific environment variables now use `ANTHROPIC_DEFAULT_*` prefix for consistency with model variables

## [1.4.0] - 2026-02-13

### Added
- **Multi-URL routing per tier** - Route each Claude tier to a different backend API endpoint
  - `ANTHROPIC_DEFAULT_OPUS_BASE_URL` - Base URL for opus tier requests
  - `ANTHROPIC_DEFAULT_SONNET_BASE_URL` - Base URL for sonnet tier requests
  - `ANTHROPIC_DEFAULT_HAIKU_BASE_URL` - Base URL for haiku tier requests
  - Falls back to `OPENAI_BASE_URL` when tier-specific URL is not configured
  - Enables routing to multiple enterprise API endpoints (LLM Large, Codestral, LLM Medium)
- `GetBaseURLForModel()` method on Config for dynamic URL resolution
- Unit tests for per-tier base URL routing and fallback behavior

### Changed
- `makeOpenAIHTTPRequest()` now uses `GetBaseURLForModel()` instead of hardcoded `OpenAIBaseURL`
- `ShouldUseMaxCompletionTokens()` cache key uses per-model base URL
- Capability cache keys (`cacheMaxCompletionTokensSupported`, `prepareRetryWithoutMaxCompletionTokens`) use per-model base URL
- Simple log and debug log now display the actual base URL used per request
- Updated `printHelp()` with new environment variables documentation

## [1.3.1] - 2026-02-13

### Added
- **Enterprise API compatibility** - Support for corporate OpenAI-compatible APIs
  - Added `oasvalidation` keyword to `isMaxTokensParameterError()` detection
  - Handles `400 OASValidation` errors from enterprise API gateways
  - New test cases for OASValidation error patterns

### Changed
- **Default token parameter strategy** - Changed default from `max_completion_tokens` to `max_tokens`
  - `ShouldUseMaxCompletionTokens()` now returns `false` on cache miss (was `true`)
  - Eliminates failed first-request for providers that only support `max_tokens`
  - More compatible with enterprise APIs (Mistral/Codestral endpoints)
  - Retry mechanism still handles the reverse case if needed

## [1.3.0] - 2025-11-13

### Added
- **Adaptive Per-Model Capability Detection** - Complete refactor replacing hardcoded patterns (#7)
  - Automatically learns which parameters each `(provider, model)` combination supports
  - Per-model capability caching with `CacheKey{BaseURL, Model}` structure
  - Thread-safe in-memory cache protected by `sync.RWMutex`
  - Debug logging for cache hits/misses visible with `-d` flag
- **Zero-Configuration Provider Compatibility**
  - Works with any OpenAI-compatible provider without code changes
  - Automatic retry mechanism with error-based detection
  - Broad keyword matching for parameter error detection
  - No status code restrictions (handles misconfigured providers)
- **OpenWebUI Support** - Native support for OpenWebUI/LiteLLM backends
  - Automatically adapts to OpenWebUI's parameter quirks
  - First request detection (~1-2s penalty), instant subsequent requests
  - Tested with GPT-5 and GPT-4.1 models

### Changed
- **Removed ~100 lines of hardcoded model patterns**
  - Deleted `IsReasoningModel()` function with gpt-5/o1/o2/o3/o4 patterns
  - Deleted `FetchReasoningModels()` function and OpenRouter API calls
  - Deleted `ReasoningModelCache` struct and related code
  - Removed unused imports: `encoding/json`, `net/http` from config.go
- **Refactored capability detection system**
  - Changed from per-provider to per-model caching
  - Struct-based cache keys (zero collision risk vs string concatenation)
  - `GetProviderCapabilities()` → `GetModelCapabilities()`
  - `SetProviderCapabilities()` → `SetModelCapabilities()`
  - `ShouldUseMaxCompletionTokens()` now uses per-model cache
- **Enhanced retry logic in handlers.go**
  - `isMaxTokensParameterError()` uses broad keyword matching
  - `retryWithoutMaxCompletionTokens()` caches per-model capabilities
  - Applied to both streaming and non-streaming handlers
  - Removed status code restrictions for better provider compatibility

### Removed
- Hardcoded reasoning model patterns (gpt-5*, o1*, o2*, o3*, o4*)
- OpenRouter reasoning models API integration
- Provider-specific hardcoding for Unknown provider type
- Unused configuration imports and dead code

### Technical Details
- **Cache Structure**: `map[CacheKey]*ModelCapabilities` where `CacheKey{BaseURL, Model}`
- **Detection Flow**: Try max_completion_tokens → Error → Retry → Cache result
- **Error Detection**: Broad keyword matching (parameter + unsupported/invalid) + our param names
- **Cache Scope**: In-memory, thread-safe, cleared on restart
- **Benefits**: Future-proof, zero user config, ~70 net lines removed

### Documentation
- Added "Adaptive Per-Model Detection" section to README.md with full implementation details
- Updated CLAUDE.md with comprehensive per-model caching documentation
- Cleaned up docs/ folder - removed planning artifacts and superseded documentation

### Philosophy
This release embodies the project philosophy: "Support all provider quirks automatically - never burden users with configurations they don't understand." The adaptive system eliminates special-casing and works with any current or future OpenAI-compatible provider.

## [1.2.0] - 2025-11-01

### Added
- **Complete CHANGELOG.md** following Keep a Changelog format
  - Full history for v1.0.0, v1.1.0, and v1.2.0
  - Categorized changes (Added, Changed, Fixed, etc.)
  - Upgrade guides and release notes
- **Release documentation system**
  - `.github/RELEASE_TEMPLATE.md` with step-by-step checklist
  - `.github/RELEASE_WORKFLOW.md` with complete workflow guide
  - Conventional commits guidelines
  - Semantic versioning strategy
- **Professional README badges**
  - Version badge (links to latest release)
  - Go version badge
  - Build status badge
  - License badge
  - Open issues badge
- **Comprehensive unit tests** for dynamic reasoning model detection
  - 36 new test cases covering hardcoded fallback patterns
  - OpenRouter API cache behavior tests
  - Provider-specific detection tests
  - Edge cases and error handling tests
  - Mock HTTP server tests for OpenRouter API integration

### Changed
- **Automated release workflow** now extracts release notes from CHANGELOG.md
  - Falls back to auto-generated notes if no changelog section found
  - Cleaner workflow logic with proper error handling
- Updated reasoning model detection tests to use `cfg.IsReasoningModel()` method

### Fixed
- Linter error for unchecked `resp.Body.Close()` in `FetchReasoningModels()`

## [1.1.0] - 2025-10-31

### Added
- **Dynamic reasoning model detection** from OpenRouter API (#5)
  - Automatically fetches list of reasoning-capable models on startup
  - Caches 130+ reasoning models (DeepSeek-R1, Gemini, GPT-5, o-series, etc.)
  - Falls back to hardcoded pattern matching for OpenAI Direct and Ollama
- Robust `max_completion_tokens` parameter detection for reasoning models
- Provider-specific model detection (OpenRouter uses API cache, others use patterns)

### Changed
- Moved reasoning model detection from standalone function to `Config` method
- Improved model detection to support dynamic discovery of new reasoning models
- Enhanced `IsReasoningModel()` to check provider type before using cache

### Technical Details
- Uses OpenRouter's `supported_parameters=reasoning` endpoint (no auth required)
- Asynchronous model fetching to avoid blocking startup
- Global `ReasoningModelCache` with populated flag for fallback behavior

## [1.0.0] - 2025-10-26

### Added
- **Initial release** of Claude Code Proxy
- Bidirectional API format conversion (Claude ↔ OpenAI)
- **Multi-provider support**:
  - OpenRouter (200+ models including Grok, Gemini, DeepSeek)
  - OpenAI Direct (GPT-4, GPT-5, o1, o3)
  - Ollama (local models)
- **Full Claude Code feature compatibility**:
  - Tool calling (function calling)
  - Extended thinking blocks (from reasoning models)
  - Streaming responses with SSE
  - Token usage tracking
- **Pattern-based model routing**:
  - Haiku → lightweight models (gpt-5-mini, gemini-flash)
  - Sonnet → flagship models (gpt-5, grok)
  - Opus → premium models (gpt-5, o3)
- **Daemon mode** with background process management
- **`ccp` wrapper script** for seamless Claude Code integration
- **Simple log mode** (`-s` flag) with one-line request summaries and throughput metrics
- **Debug mode** (`-d` flag) for full request/response logging
- Environment variable configuration via `.env` files
- Provider-specific parameter injection:
  - OpenRouter: `reasoning: {enabled: true}`, `usage: {include: true}`
  - OpenAI Direct: `reasoning_effort: "medium"` for GPT-5
  - Ollama: `tool_choice: "required"` when tools present
- Comprehensive unit tests for converter and tool calling

### Fixed
- Thinking blocks now use correct `thinking` field (not `text`)
- Streaming token usage continues past `finish_reason`
- Tool calling format conversion between Claude and OpenAI
- Encrypted reasoning blocks from models like Grok (skipped, not shown)
- HTTP logging now respects simple log mode setting
- Golangci-lint errors for CI/CD pipeline

### Changed
- Replaced hardcoded model names with constants
- Removed unused configuration options (`MAX_TOKENS_LIMIT`, `REQUEST_TIMEOUT`)
- Simplified Sonnet pattern to match all versions (sonnet-3, sonnet-4, sonnet-5)
- Updated documentation to remove o1/o3 references from defaults

### Documentation
- Complete CLI command and flag documentation
- Environment variable override documentation
- CLAUDE.md for AI-assisted development
- Beta software disclaimer
- MIT License

### Infrastructure
- GitHub Actions workflows for CI/CD
- golangci-lint integration
- Automated testing pipeline
- Claude Code Review workflow
- Claude PR Assistant workflow

## [0.1.0] - Initial Development

### Added
- Manual Anthropic-to-OpenAI proxy implementation (proof of concept)

---

## Release Notes

### How to Use This Changelog

- **Unreleased**: Changes in `main` branch not yet released
- **[X.Y.Z]**: Released versions with dates
- **Categories**:
  - `Added`: New features
  - `Changed`: Changes to existing functionality
  - `Deprecated`: Soon-to-be removed features
  - `Removed`: Removed features
  - `Fixed`: Bug fixes
  - `Security`: Security fixes

### Upgrade Guide

#### From v1.0.0 to v1.1.0
- No breaking changes
- Dynamic reasoning model detection happens automatically
- Existing `.env` configurations remain compatible
- Hardcoded fallback ensures compatibility if OpenRouter API fetch fails

---

**Links:**
- [v1.1.0 Release](https://github.com/Legolas91/claude-code-api-proxy/releases/tag/v1.1.0)
- [v1.0.0 Release](https://github.com/Legolas91/claude-code-api-proxy/releases/tag/v1.0.0)
