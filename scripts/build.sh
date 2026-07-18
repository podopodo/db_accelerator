#!/usr/bin/env sh
set -eu

version="${VERSION:-dev}"
commit="${COMMIT:-unknown}"
build_date="${BUILD_DATE:-unknown}"
output="${OUTPUT:-bin/accelerator}"

mkdir -p "$(dirname "$output")"

go build -trimpath \
  -ldflags "-s -w -X github.com/podopodo/db_accelerator/internal/buildinfo.Version=$version -X github.com/podopodo/db_accelerator/internal/buildinfo.Commit=$commit -X github.com/podopodo/db_accelerator/internal/buildinfo.BuildDate=$build_date" \
  -o "$output" ./cmd/accelerator
