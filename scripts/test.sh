#!/usr/bin/env sh
set -eu

echo '==> gofmt'
unformatted=$(gofmt -l ./cmd ./internal ./web)
if [ -n "$unformatted" ]; then
  echo "$unformatted"
  echo 'Hay archivos Go sin formato.' >&2
  exit 1
fi

echo '==> go test'
go test ./...

echo '==> go vet'
go vet ./...

echo 'OK'
