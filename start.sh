#!/bin/bash
# Starts the Go server and the web page together. Press Ctrl+C once to stop both.
if [ -d "$HOME/tools/node-v22.14.0-linux-x64/bin" ]; then
    export PATH="$HOME/tools/node-v22.14.0-linux-x64/bin:$PATH"
fi
cd "$(dirname "$0")"

backend_pid=
frontend_pid=
use_process_groups=false
if command -v setsid >/dev/null 2>&1; then
    use_process_groups=true
fi

cleanup() {
    trap - EXIT INT TERM
    for pid in "$frontend_pid" "$backend_pid"; do
        [ -n "$pid" ] || continue
        if [ "$use_process_groups" = true ]; then
            kill -TERM -- "-$pid" 2>/dev/null || true
        else
            kill -TERM "$pid" 2>/dev/null || true
        fi
    done
    for pid in "$frontend_pid" "$backend_pid"; do
        [ -n "$pid" ] || continue
        wait "$pid" 2>/dev/null || true
    done
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ -x "./backend/app" ]; then
    if [ "$use_process_groups" = true ]; then
        setsid bash -c 'cd backend && exec ./app' &
    else
        (cd backend && exec ./app) &
    fi
    backend_pid=$!
elif command -v go >/dev/null 2>&1; then
    if [ "$use_process_groups" = true ]; then
        setsid bash -c 'cd backend && exec go run .' &
    else
        (cd backend && exec go run .) &
    fi
    backend_pid=$!
else
    echo "Error: Neither prebuilt './backend/app' nor 'go' compiler found." >&2
    exit 1
fi

if command -v npm >/dev/null 2>&1; then
    if [ "$use_process_groups" = true ]; then
        setsid bash -c 'cd frontend && exec npm run dev' &
    else
        (cd frontend && exec npm run dev) &
    fi
    frontend_pid=$!
    wait "$frontend_pid"
else
    echo "Notice: npm is not available in PATH. Backend running in background."
    wait "$backend_pid"
fi
