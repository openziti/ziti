#!/usr/bin/env bash
#
# Builds a fresh, HA/SPIFFE-capable test PKI under testdata/pki, modeled on doc/ha/create-pki.sh.
# Separate from the existing testdata/ca PKI, which is left untouched. Can be run from any
# directory; it anchors itself to the script's own location.

set -euo pipefail

# Anchor to the script's own directory (tests/testdata) so paths resolve regardless of cwd.
cd "$(dirname "${BASH_SOURCE[0]}")"

PKI_ROOT="pki"
TRUST_DOMAIN="ziti.test"
# Epoch math (-d @SECONDS) so this works with both GNU date and BusyBox date.
NOW="$(date -u +%s)"
ROOT_NOT_BEFORE="$(date -u -d "@$((NOW - 86400))" +%Y-%m-%dT%H:%M:%SZ)"          # 24h before now
INTERMEDIATE_NOT_BEFORE="$(date -u -d "@$((NOW - 43200))" +%Y-%m-%dT%H:%M:%SZ)"   # 12h before now

# Self-signed root CA. --trust-domain makes every cert issued under it SPIFFE-capable.
ziti pki create ca \
  --pki-root "$PKI_ROOT" \
  --trust-domain "$TRUST_DOMAIN" \
  --ca-file root \
  --ca-name 'Ziti Test Root CA' \
  --not-before "$ROOT_NOT_BEFORE"

# Controller 1: intermediate (signing) cert
ziti pki create intermediate --pki-root "$PKI_ROOT" --ca-name root --intermediate-file ctrl1 --intermediate-name 'Controller One Signing Cert' --not-before "$INTERMEDIATE_NOT_BEFORE"

# Controller 1: server cert
ziti pki create server --pki-root "$PKI_ROOT" --ca-name ctrl1 --dns localhost --ip 127.0.0.1 --server-name ctrl1 --spiffe-id 'controller/ctrl1'

# Controller 1: client cert
ziti pki create client --pki-root "$PKI_ROOT" --ca-name ctrl1 --client-name ctrl1 --spiffe-id 'controller/ctrl1'

# Controller 2: intermediate (signing) cert
ziti pki create intermediate --pki-root "$PKI_ROOT" --ca-name root --intermediate-file ctrl2 --intermediate-name 'Controller Two Signing Cert' --not-before "$INTERMEDIATE_NOT_BEFORE"

# Controller 2: server cert
ziti pki create server --pki-root "$PKI_ROOT" --ca-name ctrl2 --dns localhost --ip 127.0.0.1 --server-name ctrl2 --spiffe-id 'controller/ctrl2'

# Controller 2: client cert
ziti pki create client --pki-root "$PKI_ROOT" --ca-name ctrl2 --client-name ctrl2 --spiffe-id 'controller/ctrl2'

# Controller 3: intermediate (signing) cert
ziti pki create intermediate --pki-root "$PKI_ROOT" --ca-name root --intermediate-file ctrl3 --intermediate-name 'Controller Three Signing Cert' --not-before "$INTERMEDIATE_NOT_BEFORE"

# Controller 3: server cert
ziti pki create server --pki-root "$PKI_ROOT" --ca-name ctrl3 --dns localhost --ip 127.0.0.1 --server-name ctrl3 --spiffe-id 'controller/ctrl3'

# Controller 3: client cert
ziti pki create client --pki-root "$PKI_ROOT" --ca-name ctrl3 --client-name ctrl3 --spiffe-id 'controller/ctrl3'

# Routers are signed by controller 1's intermediate. Each router uses a single key
# shared by its server cert (link/edge listeners) and client cert (ctrl channel).

# Router 001: key
ziti pki create key --pki-root "$PKI_ROOT" --ca-name ctrl1 --key-file 001

# Router 001: server cert
ziti pki create server --pki-root "$PKI_ROOT" --ca-name ctrl1 --key-file 001 --server-file 001-server --server-name 001 --dns localhost --ip 127.0.0.1 --spiffe-id 'router/001'

# Router 001: client cert
ziti pki create client --pki-root "$PKI_ROOT" --ca-name ctrl1 --key-file 001 --client-file 001-client --client-name 001 --spiffe-id 'router/001'

# Router 002: key
ziti pki create key --pki-root "$PKI_ROOT" --ca-name ctrl1 --key-file 002

# Router 002: server cert
ziti pki create server --pki-root "$PKI_ROOT" --ca-name ctrl1 --key-file 002 --server-file 002-server --server-name 002 --dns localhost --ip 127.0.0.1 --spiffe-id 'router/002'

# Router 002: client cert
ziti pki create client --pki-root "$PKI_ROOT" --ca-name ctrl1 --key-file 002 --client-file 002-client --client-name 002 --spiffe-id 'router/002'
