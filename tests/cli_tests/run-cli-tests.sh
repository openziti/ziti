#!/usr/bin/env bash

# raise exceptions
set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR" && git rev-parse --show-toplevel)"

cd "$ROOT_DIR"
echo "changed to root dir at: $ROOT_DIR"

if [ -z "$ZITI_CLI_TEST_ZITI_BIN" ]; then
  echo "building binary for use in tests"
  mkdir -p cli_tests_bin
  go build -o cli_tests_bin ./...
  export ZITI_CLI_TEST_ZITI_BIN="$ROOT_DIR/cli_tests_bin/ziti"
else
  echo "using pre-defined ZITI_CLI_TEST_ZITI_BIN at: $ZITI_CLI_TEST_ZITI_BIN"
fi

echo "executing cli_tests from $PWD"
go test ./tests/cli_tests/... --tags cli_tests ${ZITI_CLI_TESTS_VERBOSE:-}
echo "cli_tests complete"