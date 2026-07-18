#!/usr/bin/env sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
go generate ./...
git diff --exit-code -- . ':(exclude)plans/**'
