#!/bin/bash
# Starts the Go server and the web page together. Press Ctrl+C once to stop both.
if [ -d "$HOME/tools/node-v22.14.0-linux-x64/bin" ]; then
    export PATH="$HOME/tools/node-v22.14.0-linux-x64/bin:$PATH"
fi
cd "$(dirname "$0")"

pkill -x shopmanager 2>/dev/null
pkill -x app 2>/dev/null
sleep 1

cleanup() {
    pkill -x shopmanager 2>/dev/null
    pkill -x app 2>/dev/null
    kill 0 2>/dev/null
}
trap cleanup EXIT INT TERM

if [ -x "./backend/app" ]; then
    (cd backend && ./app) &
elif command -v go >/dev/null 2>&1; then
    (cd backend && go run .) &
else
    echo "Error: Neither prebuilt './backend/app' nor 'go' compiler found." >&2
    exit 1
fi

if command -v npm >/dev/null 2>&1; then
    cd frontend && npm run dev
else
    echo "Notice: npm is not available in PATH. Backend running in background."
    wait
fi
