#!/usr/bin/env bash
# launch_orchestra.sh — set up 6 git worktrees + tmux session for medigo v2 batch 1.
#
# After this runs, attach with:  tmux attach -t medigo-orch
# Each worker window is paused at a shell, waiting for you to launch codex with
# the prompt file path printed at the top of the window.

set -euo pipefail

REPO=~/code/medigo
SESSION=medigo-orch
DISPATCH="$REPO/.codex/tasks/medigo-extractors-v2/dispatch.json"

if [[ ! -f $DISPATCH ]]; then
    echo "missing $DISPATCH; run scripts/orchestra/dispatch_v2.py first" >&2
    exit 1
fi

# Kill old session if exists
tmux kill-session -t "$SESSION" 2>/dev/null || true

# Read worker info
mapfile -t WORKERS < <(python3 -c "
import json
for w in json.load(open('$DISPATCH')):
    sites = '|'.join(s['name'] for s in w['sites'])
    print(f\"{w['worker']}|{w['branch']}|{w['worktree']}|{sites}\")
")

# Create monitor window first
tmux new-session -d -s "$SESSION" -n monitor -c "$REPO"
tmux send-keys -t "$SESSION:monitor" "watch -n 5 'cd $REPO && bash scripts/orchestra/orchestra_status.sh'" C-m

for entry in "${WORKERS[@]}"; do
    IFS='|' read -r WID BRANCH WT SITES <<< "$entry"

    # Create worktree if missing
    if [[ ! -d $WT ]]; then
        echo "Creating worktree $WT (branch $BRANCH)..."
        cd "$REPO"
        git worktree add "$WT" -b "$BRANCH"
    else
        echo "Worktree $WT already exists, skipping create"
    fi

    PROMPT_FILE="$REPO/.codex/tasks/medigo-extractors-v2/prompts/worker-$WID.md"

    # New tmux window for this worker
    tmux new-window -t "$SESSION" -n "w$WID-${SITES//|/+}" -c "$WT"
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "clear" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "echo '======================================='" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "echo 'WORKER $WID: $SITES'" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "echo 'WORKTREE: $WT'" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "echo 'BRANCH:   $BRANCH'" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "echo 'PROMPT:   $PROMPT_FILE'" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "echo '======================================='" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "echo 'launch your codex agent here, e.g.:'" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "echo '  codex exec \"\\$(cat $PROMPT_FILE)\"'" C-m
    tmux send-keys -t "$SESSION:w$WID-${SITES//|/+}" "cat $PROMPT_FILE | head -20" C-m
done

echo
echo "tmux session '$SESSION' ready."
echo "attach with: tmux attach -t $SESSION"
echo
echo "windows:"
tmux list-windows -t "$SESSION"
