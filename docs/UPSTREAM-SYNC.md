# Upstream Synchronization Strategy

## Fork relationship

| | This fork | Upstream |
|---|---|---|
| Repo | `Legolas91/claude-code-api-proxy` | `nielspeter/claude-code-proxy` |
| Divergence point | `61c92a3` (first functional commit) | same |
| Commits ahead | 48+ | 19 |
| Lines added | +5070 | +2649 |

## Strategy: cherry-pick only — no rebase

A `git rebase upstream/main` is not viable. Our fork has diverged too deeply:

- **14 files modified on both sides**, including `handlers.go`, `config.go`, `converter.go`, and all CI workflows
- High-severity conflicts in `handlers.go` (threading baseURL/apiKey vs upstream retry logic) and `config.go` (multi-URL/key routing vs upstream adaptive detection structs)
- Our rename `cmd/claude-code-proxy/` → `cmd/cc-api-proxy/` breaks all upstream patches that touch that path

**Approach**: run `./scripts/update-upstream.sh` when upstream moves, evaluate each new commit manually, cherry-pick only what applies.

## How to evaluate a new upstream commit

Ask these questions in order:

1. **Already in our fork?** Search for the key symbol or pattern:
   ```bash
   grep -rn "function_name" internal/
   ```
2. **Does the target code still exist?** Upstream may patch code we restructured or renamed.
3. **Made obsolete by our implementation?** E.g., hardcoded model lists are obsolete because we have adaptive detection.
4. **Repo-specific?** CI workflows, beta disclaimers, and upstream-specific docs are irrelevant.

Categories:

| Label | Meaning |
|-------|---------|
| `SKIP – already integrated` | Our fork already contains this fix |
| `SKIP – obsolete` | Our implementation supersedes this approach |
| `SKIP – repo-specific` | GitHub Actions, README badges, beta disclaimers |
| `APPLY` | Genuine fix or improvement not yet in our code |

## Full analysis — commits reviewed up to `2e82b12` (2025-11-24)

Last reviewed: **2025-11-24** — upstream HEAD `2e82b12ed6aaf5112d96175841f5ea72aa630f16`

Next review: run `./scripts/update-upstream.sh` and start from the commit after `2e82b12`.

### Commits analyzed (19 total, divergence point `61c92a3`)

| SHA | Title | Decision | Reason |
|-----|-------|----------|--------|
| `409ce26` | Add Claude Code Review GitHub Actions workflow | `SKIP – repo-specific` | CI that auto-reviews PRs; not relevant |
| `92fd72f` | Merge PR #1 (test PR for Claude review) | `SKIP – repo-specific` | Merge commit for above |
| `a9384ba` | docs: Add env variable override documentation to constants | `SKIP – already integrated` | Our `converter.go` already documents `ANTHROPIC_DEFAULT_*_MODEL` |
| `e1284f0` | docs: Add complete CLI command and flag documentation | `SKIP – already integrated` | `cmd/cc-api-proxy/main.go` already has full help text |
| `c74433d` | docs: Add beta software disclaimer | `SKIP – repo-specific` | We are at v1.5.11, not beta |
| `8d89f26` | feat: Improve ccp wrapper — use `command -v` instead of `./` | `SKIP – already integrated` | Our `scripts/ccp` already uses `command -v cc-api-proxy` |
| `032f3e6` | test: Add comprehensive test coverage (config, daemon, server) | `SKIP – already integrated` | All 8+9+9 upstream tests already present in our fork; we have 20+18+20 tests |
| `2117e6b` | Improve code quality — extract `addOpenRouterHeaders()`, linting | `SKIP – already integrated` | `addOpenRouterHeaders()` is at `handlers.go:56`; `.golangci.yml` already v2 |
| `277cf59` | refactor: Remove "matches Python implementation" comments | `SKIP – already integrated` | No such comments in our `handlers.go` |
| `1788b00` | docs: Add comprehensive manual testing instructions | `SKIP – already integrated` | Our `CLAUDE.md` has an equivalent "Manual Testing" section |
| `aa7e1a9` | docs: Update CHANGELOG for v1.2.0 | `SKIP – repo-specific` | Upstream release notes; we are at v1.5.11 |
| `a200118` | docs: Add changelog and release documentation system | `SKIP – already integrated` | Our release pipeline (5 platforms, checksums) is more advanced |
| `4d77643` | feat: Add robust reasoning model detection (`isReasoningModel`) | `SKIP – obsolete` | Replaced by adaptive per-model detection (our v1.3.0); hardcoding anti-pattern |
| `8342b7f` | fix: Add OpenWebUI GPT-5 reasoning model support (workaround) | `SKIP – obsolete` | Replaced by adaptive retry; the workaround is not needed with our approach |
| `f4a2c45` | test: Remove obsolete reasoning model detection tests | `SKIP – obsolete` | Clean-up linked to `4d77643`/`8342b7f`, which we don't have |
| `d699a60` | feat: Adaptive per-model capability detection | `SKIP – already integrated` | Core of our system since v1.3.0; `CacheKey`, `ModelCapabilities`, retry logic all present |
| `d1efa2c` | test: Add comprehensive adaptive detection tests | `SKIP – already integrated` | `adaptive_detection_test.go` exists in our fork with equivalent coverage |
| `3a4ada1` | Merge PR #7 (adaptive detection) | `SKIP – already integrated` | Merge of `d699a60` + `d1efa2c` |
| `2e82b12` | fix: Remove duplicate `isMaxTokensParameterError` from tests | `SKIP – already integrated` | Our test file has the NOTE comment, no redeclaration |

**Result: 0 cherry-picks applied.** Our fork is a strict superset of the upstream at this point.

## Decision criteria for future commits

Commits that are most likely to be `APPLY`:

- Bug fixes in `internal/converter/converter.go` for edge cases in Claude ↔ OpenAI format conversion
- New provider support that doesn't conflict with our per-tier routing
- Security fixes (dependency upgrades, unsafe patterns)

Commits that are almost always `SKIP`:

- Documentation that assumes upstream's simple single-URL config
- CI/GitHub Actions changes (our CI is independent and more advanced)
- Hardcoded model lists or provider-specific workarounds (our adaptive detection handles these)
- Test additions that duplicate tests we already have
