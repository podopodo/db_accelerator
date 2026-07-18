#!/usr/bin/env sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"

actual_file="tmp/dependencies.actual"
allowed_file="tmp/dependencies.allowed"
mkdir -p tmp
go list -m -f '{{if not .Main}}{{.Path}}{{end}}' all | sed '/^$/d' | sort -u > "$actual_file"
sed '/^$/d; /^#/d' build/dependencies.allow | sort -u > "$allowed_file"

if ! diff -u "$allowed_file" "$actual_file"; then
  echo "Dependency allowlist mismatch. Review license and update notices before accepting a module."
  exit 1
fi

echo "Dependency allowlist matches."
