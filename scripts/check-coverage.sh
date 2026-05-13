#!/usr/bin/env bash
set -euo pipefail

threshold="99.0"
coverage_file="coverage.out"

go test ./... -covermode=atomic -coverprofile="${coverage_file}"

total="$(go tool cover -func="${coverage_file}" | awk '/^total:/ {gsub(/%/, "", $3); print $3}')"
if [[ -z "${total}" ]]; then
  echo "Unable to read total coverage from ${coverage_file}" >&2
  exit 1
fi

awk -v total="${total}" -v threshold="${threshold}" 'BEGIN {
  if (total + 0 < threshold + 0) {
    printf "Coverage %.1f%% is below required %.1f%%\n", total, threshold > "/dev/stderr"
    exit 1
  }

  printf "Coverage %.1f%% meets required %.1f%%\n", total, threshold
}'
