#!/bin/bash
# Update upstream with regression testing
# Usage: ./scripts/update-upstream.sh [--apply]

set -e

UPSTREAM_FILE=".upstream-commit"
CURRENT_BRANCH=$(git branch --show-current)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "=== Upstream Update Check ==="
echo ""

# Read pinned commit
PINNED_COMMIT=$(grep -v "^#" "$UPSTREAM_FILE" | tr -d '[:space:]')
echo "Pinned commit:  $PINNED_COMMIT"

# Fetch upstream
echo "Fetching upstream..."
git fetch upstream --quiet

# Get latest upstream commit
LATEST_COMMIT=$(git rev-parse upstream/main)
LATEST_MSG=$(git log upstream/main -1 --format="%s")
echo "Latest commit:  $LATEST_COMMIT"
echo "                $LATEST_MSG"
echo ""

# Compare
if [ "$PINNED_COMMIT" = "$LATEST_COMMIT" ]; then
    echo -e "${GREEN}Already up to date with upstream.${NC}"
    exit 0
fi

# Show commits between pinned and latest
echo -e "${YELLOW}New commits available:${NC}"
git log --oneline "$PINNED_COMMIT..upstream/main"
echo ""

# Check for --apply flag
if [ "$1" != "--apply" ]; then
    echo "Run with --apply to test and update"
    exit 0
fi

echo "=== Testing rebase on latest upstream ==="
echo ""

# Create test branch
TEST_BRANCH="test-upstream-$(date +%Y%m%d-%H%M%S)"
echo "Creating test branch: $TEST_BRANCH"
git checkout -b "$TEST_BRANCH"

# Try rebase
echo "Rebasing on upstream/main..."
if ! git rebase upstream/main; then
    echo -e "${RED}Rebase failed! Conflicts detected.${NC}"
    echo "Aborting rebase..."
    git rebase --abort
    git checkout "$CURRENT_BRANCH"
    git branch -D "$TEST_BRANCH"
    exit 1
fi

# Run tests
echo ""
echo "Running tests..."
if ! go test ./...; then
    echo -e "${RED}Tests failed! Regression detected.${NC}"
    git checkout "$CURRENT_BRANCH"
    git branch -D "$TEST_BRANCH"
    exit 1
fi

# Build check
echo ""
echo "Building..."
if ! go build -o /dev/null cmd/claude-code-proxy/main.go; then
    echo -e "${RED}Build failed!${NC}"
    git checkout "$CURRENT_BRANCH"
    git branch -D "$TEST_BRANCH"
    exit 1
fi

echo ""
echo -e "${GREEN}All tests passed!${NC}"
echo ""

# Ask to apply
read -p "Apply update to $CURRENT_BRANCH? [y/N] " -n 1 -r
echo ""

if [[ $REPLY =~ ^[Yy]$ ]]; then
    # Update the main branch
    git checkout "$CURRENT_BRANCH"
    git rebase upstream/main

    # Update pinned commit
    echo "# Upstream commit pinned from nielspeter/claude-code-proxy" > "$UPSTREAM_FILE"
    echo "# This file tracks the exact upstream version we're based on" >> "$UPSTREAM_FILE"
    echo "# Update only after tests pass with: ./scripts/update-upstream.sh" >> "$UPSTREAM_FILE"
    echo "" >> "$UPSTREAM_FILE"
    echo "$LATEST_COMMIT" >> "$UPSTREAM_FILE"

    # Cleanup test branch
    git branch -D "$TEST_BRANCH"

    echo ""
    echo -e "${GREEN}Updated to upstream commit: $LATEST_COMMIT${NC}"
    echo "Don't forget to: git push origin $CURRENT_BRANCH --force-with-lease"
else
    git checkout "$CURRENT_BRANCH"
    git branch -D "$TEST_BRANCH"
    echo "Update cancelled."
fi
