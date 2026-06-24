#!/usr/bin/env bash
# merge_batch1.sh — cherry-pick all 6 worker branches back to main.
#
# Run from ~/code/medigo. Stops on first conflict so you can resolve and resume.
# Conflicts most likely come from cmd/medigo/main.go (each worker adds its
# 3 site imports). Just keep all of them.

set -euo pipefail

REPO=~/code/medigo
cd "$REPO"

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [[ $CURRENT_BRANCH != "main" && $CURRENT_BRANCH != "dev-v2" ]]; then
    echo "switch to main or dev-v2 first (currently on $CURRENT_BRANCH)" >&2
    exit 1
fi

for w in 1 2 3 4 5 6; do
    BRANCH="work/v2-batch1-w$w"
    if ! git show-ref --verify --quiet "refs/heads/$BRANCH"; then
        echo "branch $BRANCH not found, skipping"
        continue
    fi
    AHEAD=$(git rev-list --count "main..$BRANCH" 2>/dev/null || echo 0)
    echo
    echo "======= worker-$w ($BRANCH, $AHEAD commits ahead) ======="
    if [[ $AHEAD -eq 0 ]]; then
        echo "  no commits, skipping"
        continue
    fi
    git log "main..$BRANCH" --oneline
    echo
    read -p "cherry-pick these into main? [y/N] " yn
    if [[ $yn != "y" && $yn != "Y" ]]; then
        echo "  skipped"
        continue
    fi
    if ! git cherry-pick "main..$BRANCH"; then
        echo
        echo "CONFLICT — resolve, then 'git cherry-pick --continue' and re-run this script"
        exit 1
    fi
done

echo
echo "======= post-merge verify ======="
go build ./... && echo "build: OK" || echo "build: FAILED"
go vet ./... 2>&1 | head -5 && echo "vet: OK"
python3 scripts/verify_full_alignment.py 2>&1 | grep -E "PASS:|STUB:"
