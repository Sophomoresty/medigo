#!/usr/bin/env bash
# orchestra_status.sh — one-screen status of all v2 batch 1 workers.
set -uo pipefail

REPO=~/code/medigo
DISPATCH="$REPO/.codex/tasks/medigo-extractors-v2/dispatch.json"

if [[ ! -f $DISPATCH ]]; then
    echo "missing $DISPATCH" >&2
    exit 1
fi

echo "============= MediGo v2 Orchestra Status =============  $(date '+%H:%M:%S')"

# Aggregate: full alignment in main repo
PASS_TOTAL=$(cd "$REPO" && python3 scripts/verify_full_alignment.py 2>/dev/null | grep "PASS:" | awk '{print $2}')
STUB_TOTAL=$(cd "$REPO" && python3 scripts/verify_full_alignment.py 2>/dev/null | grep "STUB:" | awk '{print $2}')
echo "main repo: PASS=$PASS_TOTAL STUB=$STUB_TOTAL"
echo

printf "%-8s %-30s %-15s %-30s %s\n" "WORKER" "SITES" "BRANCH-AHEAD" "LATEST COMMIT" "PASS/STUB-IN-WT"
echo "------------------------------------------------------------------------------------------------------"

python3 -c "
import json, os, subprocess
workers = json.load(open('$DISPATCH'))
for w in workers:
    wt = w['worktree']
    sites = ','.join(s['name'] for s in w['sites'])
    if not os.path.isdir(wt):
        print(f'{w[\"worker\"]:<8} {sites:<30} {\"(no worktree)\":<15}')
        continue
    try:
        ahead = subprocess.check_output(['git', '-C', wt, 'rev-list', '--count', 'main..HEAD'], text=True).strip()
    except Exception:
        ahead = '?'
    try:
        commit = subprocess.check_output(['git', '-C', wt, 'log', '-1', '--oneline'], text=True).strip()[:30]
    except Exception:
        commit = '(none)'
    pass_cnt = 0
    stub_cnt = 0
    for s in w['sites']:
        site_go = os.path.join(wt, 'internal', 'extractor', s['name'], s['name'] + '.go')
        if not os.path.exists(site_go):
            continue
        src = open(site_go, errors='replace').read()
        has_http = any(p in src for p in ('.GetString(', '.PostForm(', '.GetBytes(', '.Get(', '.Post('))
        has_parse = 'json.Unmarshal(' in src or 'FindStringSubmatch(' in src
        if has_http and has_parse:
            pass_cnt += 1
        elif 'not yet implemented' in src:
            stub_cnt += 1
    progress = f'{pass_cnt}/{len(w[\"sites\"])}'
    print(f'{w[\"worker\"]:<8} {sites:<30} +{ahead:<14} {commit:<30} pass={pass_cnt}/{len(w[\"sites\"])} stub={stub_cnt}')
"

echo
echo "tmux windows:"
tmux list-windows -t medigo-orch 2>/dev/null || echo "  (orchestra not running)"
