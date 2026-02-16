#!/usr/bin/env bash
# Pre-commit hook: detect improperly built gt binaries in the repo root.
#
# Install via: make install-hooks
#
# Checks:
# 1. If a gt binary exists in the repo root, verify it was built with make
# 2. Warn if go build/install appears in staged changes (heuristic)

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
BINARY="$REPO_ROOT/gt"

# Check 1: If a gt binary exists in repo root, verify BuiltProperly
if [ -x "$BINARY" ]; then
    # Run the binary's version command and check stderr for the WARNING
    if "$BINARY" version 2>&1 | grep -q "WARNING.*built with.*go build"; then
        echo "ERROR: Found improperly built gt binary at $BINARY"
        echo "       This binary was built with 'go build' instead of 'make build'."
        echo "       Fix: run 'make build' or remove the binary with 'make clean'"
        exit 1
    fi
fi

# Check 2: Warn if staged files contain raw go build/install commands for this repo
# This is a heuristic — won't catch everything, but catches common mistakes
STAGED_DIFF=$(git diff --cached --diff-filter=ACM -- '*.sh' '*.md' '*.yml' '*.yaml' 'Makefile' '*.go' 2>/dev/null || true)
if echo "$STAGED_DIFF" | grep -qE '^\+.*\bgo (build|install)\b.*\./cmd/gt'; then
    echo "WARNING: Staged changes contain 'go build ./cmd/gt' or 'go install ./cmd/gt'"
    echo "         Use 'make build' or 'make install' instead."
    echo "         If this is intentional (e.g., in documentation about what NOT to do), ignore this warning."
    # Warning only — don't block the commit for documentation examples
fi

exit 0
