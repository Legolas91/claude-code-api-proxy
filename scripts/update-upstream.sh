#!/bin/bash
# Review new upstream commits for potential cherry-pick.
#
# Strategy: cherry-pick only — NO rebase.
# Our fork diverged from upstream at 61c92a3 (first functional commit).
# We have 48+ commits and 5070+ lines more than upstream.
# A rebase would produce ~14 high-severity conflicts across core files.
#
# Usage:
#   ./scripts/update-upstream.sh          # List new commits + file stats
#   ./scripts/update-upstream.sh --full   # Include full diff for each commit
#   ./scripts/update-upstream.sh --update # Update pinned commit after review
#
# After review, cherry-pick selectively:
#   git cherry-pick -n <sha>              # Stage only (edit before committing)
#   git cherry-pick <sha>                 # Apply directly
#
# See docs/UPSTREAM-SYNC.md for decision criteria and full past analysis.

set -e

UPSTREAM_FILE=".upstream-commit"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

echo "=== Upstream Cherry-Pick Review ==="
echo ""

# Read pinned commit
PINNED_COMMIT=$(grep -v "^#" "$UPSTREAM_FILE" | tr -d '[:space:]')
PINNED_MSG=$(git log --format="%s" -1 "$PINNED_COMMIT" 2>/dev/null || echo "not in local history — run: git fetch upstream")
echo "Last reviewed:   $PINNED_COMMIT"
echo "                 $PINNED_MSG"

# Fetch upstream
echo "Fetching upstream..."
git fetch upstream --quiet

# Get latest upstream commit
LATEST_COMMIT=$(git rev-parse upstream/main)
LATEST_MSG=$(git log upstream/main -1 --format="%s")
echo "Latest upstream: $LATEST_COMMIT"
echo "                 $LATEST_MSG"
echo ""

# Handle --update flag
if [ "$1" = "--update" ]; then
    {
        grep "^#" "$UPSTREAM_FILE"
        echo ""
        echo "$LATEST_COMMIT"
    } > "${UPSTREAM_FILE}.tmp" && mv "${UPSTREAM_FILE}.tmp" "$UPSTREAM_FILE"
    echo -e "${GREEN}Pinned commit updated to $LATEST_COMMIT.${NC}"
    echo "Don't forget to: git add .upstream-commit && git commit -m 'chore: Update upstream review pointer to <version>'"
    exit 0
fi

# Compare
if [ "$PINNED_COMMIT" = "$LATEST_COMMIT" ]; then
    echo -e "${GREEN}Upstream has not moved since last review. Nothing to do.${NC}"
    exit 0
fi

# Count new commits
NEW_COUNT=$(git log --oneline "$PINNED_COMMIT..upstream/main" | wc -l | tr -d ' ')
echo -e "${YELLOW}${NEW_COUNT} new commit(s) to review (oldest first):${NC}"
echo ""

# List commits from oldest to newest
mapfile -t COMMITS < <(git log --oneline --reverse "$PINNED_COMMIT..upstream/main")

for line in "${COMMITS[@]}"; do
    SHA=$(echo "$line" | cut -d' ' -f1)
    MSG=$(echo "$line" | cut -d' ' -f2-)

    echo -e "${CYAN}── $SHA  $MSG${NC}"
    git show --stat "$SHA" | grep -v "^commit\|^Author\|^Date\|^$\|^    " | head -10

    if [ "$1" = "--full" ]; then
        echo ""
        git show "$SHA" --unified=3
    fi

    echo ""
done

echo "=== How to Evaluate Each Commit ==="
echo ""
echo "For each commit, check docs/UPSTREAM-SYNC.md for decision criteria, then:"
echo ""
echo "  1. Already in our fork?     grep -rn 'keyword' internal/"
echo "  2. Target code still exist? grep -n 'func_name' internal/server/handlers.go"
echo "  3. Obsolete (we did more)?  Compare diff with our implementation"
echo ""
echo "Categories (from past analysis):"
echo "  SKIP  — already integrated in our fork"
echo "  SKIP  — made obsolete by our adaptive detection"
echo "  SKIP  — repo-specific (CI, docs not relevant to us)"
echo "  APPLY — genuine fix not yet in our code"
echo ""
echo "To apply a commit:"
echo "  git cherry-pick -n <sha>    # Stage only, review diff, then commit"
echo "  git cherry-pick <sha>       # Apply directly"
echo ""
echo "After completing review:"
echo "  ./scripts/update-upstream.sh --update"
