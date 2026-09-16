#!/usr/bin/env bash
# Copyright IBM Corp. 2026
# SPDX-License-Identifier: MPL-2.0


set -euo pipefail

# End-to-end test for tfctl
#
# Runs dist/tfctl through some basic test cases. Presumes the default profile
# is already configured. 'setup' creates $organization and all cases should
# use it.
# 
# System prerequisites are:
#   tar
#.  curl

root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tfctl_bin="${TFCTL_BIN:-$root_dir/dist/tfctl}"
run_id="${GITHUB_RUN_ID:-$(date +%s)}-${GITHUB_RUN_ATTEMPT:-$RANDOM}"
organization="tfctl-e2e-${run_id}"
organization_created=false

teardown() {
  local status=$?

  if [ "$organization_created" = true ]; then
    "$tfctl_bin" harness exec --allow-delete=organizations -- \
      "$tfctl_bin" api "/organizations/$organization" -X DELETE || status=1
  fi

  exit "$status"
}
trap teardown EXIT

setup() {
  if [ ! -x "$tfctl_bin" ]; then
    printf 'tfctl binary not found at %s\n' "$tfctl_bin" >&2
    exit 1
  fi

  for command in curl tar; do
    if ! command -v "$command" >/dev/null 2>&1; then
      printf '%s is required to run the end-to-end test\n' "$command" >&2
      exit 1
    fi
  done

  printf 'Creating organization %s\n' "$organization"
  "$tfctl_bin" api "/organizations" -X POST -a "name=$organization" -a "email=tfctl-e2e@example.com" --quiet
  organization_created=true
}

run_case() {
  local name=$1
  printf '\n=== %s ===\n' "$name"
  "$name"
}

create_auto_apply_workspace() {
  local workspace="tfctl-e2e-$RANDOM"

  "$tfctl_bin" create workspace --organization "$organization" --jq '.data.id' \
    -a "name=$workspace" -a auto-apply=true
}

upload_configuration() {
  local workspace_id=$1
  local archive=$2
  local configuration_id configuration_status upload_url

  upload_url="$("$tfctl_bin" api "/workspaces/$workspace_id/configuration-versions" --no-redact --jq '.data.attributes["upload-url"]' -i \
    '{"data":{"type":"configuration-versions","attributes":{"auto-queue-runs":false}}}')"

  printf 'Uploading configuration\n'
  curl --fail --silent --show-error --request PUT --upload-file "$archive" "$upload_url"

  configuration_id="$("$tfctl_bin" api "/workspaces/$workspace_id" --jq \
    '.data.relationships["current-configuration-version"].data.id')"

  for _ in $(seq 1 60); do
    configuration_status="$("$tfctl_bin" api "/configuration-versions/$configuration_id" --jq '.data.attributes.status')"
    if [ "$configuration_status" = "uploaded" ]; then
      return
    fi
    if [ "$configuration_status" = "errored" ]; then
      printf 'current configuration version %s failed to upload\n' "$configuration_id" >&2
      return 1
    fi
    sleep 2
  done

  printf 'current configuration version %s did not finish uploading\n' "$configuration_id" >&2
  return 1
}

case_create_and_apply_workspace() (
  local archive workspace_id
  archive="$(mktemp)"
  trap 'rm -f "$archive"' EXIT

  printf 'Creating auto-apply workspace\n'
  workspace_id="$(create_auto_apply_workspace)"
  tar -C "$root_dir/e2e" -czf "$archive" main.tf
  upload_configuration "$workspace_id" "$archive"

  printf 'Starting and waiting for the auto-apply run\n'
  "$tfctl_bin" run start "$workspace_id" --wait --timeout 20m
)

setup
run_case case_create_and_apply_workspace
