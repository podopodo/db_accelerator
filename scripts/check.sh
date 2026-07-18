#!/usr/bin/env sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"

unformatted=$(gofmt -l cmd internal)
if [ -n "$unformatted" ]; then
  echo "Go files require formatting:"
  echo "$unformatted"
  exit 1
fi

go vet ./...
go test -coverprofile=coverage.txt ./...
go test -race ./...
sh scripts/check-dependencies.sh
go build -trimpath -o tmp/accelerator-check ./cmd/accelerator
