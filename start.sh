#!/bin/bash
# Starts the Go server and the web page together. Press Ctrl+C once to stop both.
export PATH="$HOME/tools/node-v22.14.0-linux-x64/bin:$PATH"
cd "$(dirname "$0")"

pkill -x shopmanager 2>/dev/null
sleep 1

(cd backend && go run .) &
trap 'pkill -x shopmanager 2>/dev/null; kill 0' EXIT

cd frontend && npm run dev
