#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
cd "$PROJECT_ROOT"

echo '==> Go'
go version

echo '==> gofmt'
unformatted=$(gofmt -l ./cmd ./internal)
if [ -n "$unformatted" ]; then
  echo "$unformatted"
  echo 'ERROR: Hay archivos Go sin formato.' >&2
  exit 1
fi

echo '==> go mod tidy'
go mod tidy

echo '==> go test'
go test ./...

echo '==> go vet'
go vet ./...

echo '==> build'
go build -o /tmp/personalcloud-test ./cmd/server
rm -f /tmp/personalcloud-test

echo 'OK'
