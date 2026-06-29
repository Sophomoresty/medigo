#!/usr/bin/env bash
set -u

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="${MEDIAGO_E2E_BIN:-$ROOT/mediago}"
PASS=0
FAIL=0

run_test() {
    local name="$1"
    shift
    echo "[TEST] $name..."
    if "$@"; then
        echo "  PASS"
        PASS=$((PASS + 1))
    else
        echo "  FAIL"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== MediGo E2E Verification ==="
echo ""

cd "$ROOT" || exit 1

echo "[BUILD] Building mediago..."
if go build -o "$BIN" ./cmd/mediago; then
    echo "[BUILD] OK"
else
    echo "[BUILD] FAIL"
    exit 1
fi
echo ""

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

run_test "--help shows usage" bash -c '"$0" --help >"$1/help.out" 2>"$1/help.err" && grep -q "Usage:" "$1/help.out"' "$BIN" "$TMPDIR"
run_test "version command" bash -c '"$0" version >"$1/version.out" 2>"$1/version.err" && grep -q "mediago 0.1.0" "$1/version.out"' "$BIN" "$TMPDIR"
run_test "--version flag" bash -c '"$0" --version >"$1/version-flag.out" 2>"$1/version-flag.err" && grep -q "mediago 0.1.0" "$1/version-flag.out"' "$BIN" "$TMPDIR"
run_test "--list-extractors count" bash -c '"$0" --list-extractors >"$1/list.out" 2>"$1/list.err" && count=$(tail -n 1 "$1/list.out" | grep -Eo "^[0-9]+") && [ "${count:-0}" -ge 90 ]' "$BIN" "$TMPDIR"
run_test "no args shows help" bash -c '"$0" >"$1/noargs.out" 2>"$1/noargs.err" && grep -q "Usage:" "$1/noargs.out"' "$BIN" "$TMPDIR"
run_test "unsupported URL fails clearly" bash -c '! "$0" "https://www.example.com/video" >"$1/unsupported.out" 2>"$1/unsupported.err" && grep -qi "unsupported URL" "$1/unsupported.err"' "$BIN" "$TMPDIR"
run_test "auth-required extractor fails clearly without cookies" bash -c '! "$0" "https://www.icourse163.org/course/ZJICM-1449623161" >"$1/auth.out" 2>"$1/auth.err" && grep -Eqi "cookie|login|auth|require|unsupported|failed" "$1/auth.err"' "$BIN" "$TMPDIR"
run_test "-j unsupported URL fails clearly" bash -c '! "$0" -j "https://www.example.com/video" >"$1/dump-json.out" 2>"$1/dump-json.err" && grep -qi "unsupported URL" "$1/dump-json.err"' "$BIN" "$TMPDIR"
run_test "-F unsupported URL fails clearly" bash -c '! "$0" -F "https://www.example.com/video" >"$1/list-formats.out" 2>"$1/list-formats.err" && grep -qi "unsupported URL" "$1/list-formats.err"' "$BIN" "$TMPDIR"

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
exit 0
