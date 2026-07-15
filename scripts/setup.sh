#! /usr/bin/env bash

set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
RESET='\033[0m'

echo "Running \"make tools\" and \"make go/install\"..."
make tools
make go/install

# Verify installation
if command -v tfctl >/dev/null 2>&1; then
  echo -e "${GREEN}✓${RESET} tfctl CLI installed: $(tfctl --version 2>&1)"
else
  echo -e "${RED}✗${RESET} tfctl installed to $(go env GOPATH)/bin/ but that's not on PATH"
  echo "  Add to shell config: export PATH=\"\$(go env GOPATH)/bin:\$PATH\""
  exit 1
fi