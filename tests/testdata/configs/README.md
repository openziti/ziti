# Test Configuration Sets

This directory contains named configuration sets used by the integration tests in
`ziti/tests/`. Each subdirectory is a self-contained set of YAML config files for a
specific test scenario.

## How Configuration Sets Work

Each config set has a corresponding `ConfigSet` variable declared in `tests/configsets.go`.
Most tests use `DefaultATS` implicitly via `NewTestContext(t)`. Tests that need a different
configuration call `NewTestContextWithConfigSet(t, <ConfigSet>)`, passing one of the
package-level vars defined in `configsets.go`.

All paths inside config files are relative to the `tests/` working directory (the
standard Go test working directory for this package), so cert and key paths such as
`testdata/ca/...` resolve correctly regardless of which config set is active.

## Directory Layout

```
testdata/configs/
  <config-set-name>/
    ctrl.yml                      # controller config
    router-N.yml                  # router configs, when the set needs them
    ats-<component>.yml           # default-ats set keeps the original ats-* naming
```

## Config Sets

- **`default-ats`** (`DefaultATS`) — The standard full-stack config used by the majority of
  the integration test suite. Starts the controller on `127.0.0.1:1281` with edge, OIDC,
  management, fabric, and health-check APIs; edge router on `127.0.0.1:3022`; transit router
  on `tls:0.0.0.0:7098`. Also includes the fabric-only router pair used by link-management
  tests (router 1 on `tls:127.0.0.1:6004`, router 2 on `tls:127.0.0.1:6005`).

- **`no-explicit-oidc`** (`NoExplicitOIDC`) — Controller config identical to
  `default-ats/ctrl.yml` except the `edge-oidc` binding is omitted from the web listener.
  Used to verify that the controller's `ensureOidcOnClientApiServer` validator automatically
  adds the OIDC API when it is not explicitly configured.

- **`disabled-oidc-auto-binding`** (`DisabledOidcAutoBinding`) — Controller config with the
  `edge-oidc` binding omitted from the web listener AND `disableOidcAutoBinding: true` set in
  the `edge:` section. Used to verify that the auto-binding behaviour is suppressed when the
  operator opts out, leaving OIDC absent from the running controller.

- **`dual-oidc-servers`** (`DualOidcServers`) — Controller config with two web servers on different
  ports, each hosting the `edge-oidc` API. Used by `Test_OidcDiscoveryEndpoints_DualServers` to verify the
  OIDC discovery document returns endpoint URLs that reflect the port the client connected to.

- **`wildcard-oidc-server`** (`WildcardOidcServer`) — Ordinary primary `server_cert` plus an
  `alt_server_certs` entry whose only SAN is the wildcard `*.wildcard.test`. OIDC issuers come from the SANs
  of all active server certs, so the wildcard becomes an issuer. Used by
  `Test_OidcDiscoveryEndpoints_WildcardIssuer` to verify a concrete host under the wildcard gets a
  request-host-derived issuer (not a literal-wildcard issuer, and not a 404). Cert regen: see below.

## Regenerating the wildcard server cert

`ctrl-server-wildcard.cert.pem` is the alt cert for `wildcard-oidc-server`. Its only DNS SAN is
`*.wildcard.test` (no `localhost`/IP SANs, to avoid ambiguous-SNI conflicts with the primary cert). It reuses
the existing controller key (`ctrl.key.pem`) and test intermediate CA, and expires in 2036. Regenerate from
the repo root (drop `MSYS_NO_PATHCONV=1` outside Git Bash on Windows):

```bash
cat > /tmp/wildcard-san.cnf <<'EOF'
[v3_req]
basicConstraints=CA:FALSE
keyUsage=digitalSignature,nonRepudiation,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:*.wildcard.test
EOF
MSYS_NO_PATHCONV=1 openssl req -new \
  -key tests/testdata/ca/intermediate/private/ctrl.key.pem \
  -subj "/C=US/ST=North Carolina/L=Charlotte/O=NetFoundry/OU=Ziti Fabric/CN=ctrl" \
  -out /tmp/wildcard.csr
MSYS_NO_PATHCONV=1 openssl x509 -req -in /tmp/wildcard.csr \
  -CA tests/testdata/ca/intermediate/certs/intermediate.cert.pem \
  -CAkey tests/testdata/ca/intermediate/private/intermediate.key.decrypted.pem \
  -CAserial /tmp/wildcard.srl -CAcreateserial \
  -days 3650 -sha256 -extfile /tmp/wildcard-san.cnf -extensions v3_req \
  -out tests/testdata/ca/intermediate/certs/ctrl-server-wildcard.cert.pem
```

## Adding a New Config Set

1. Create a subdirectory with a short, descriptive name (kebab-case).
2. Add a `ctrl.yml` and/or router configs as needed, using paths relative to `tests/`.
3. Add a `ConfigSet` var for it in `tests/configsets.go`.
4. Write your test using `NewTestContextWithConfigSet(t, <YourNewConfigSet>)`.
5. Add an entry for the new set to the list above.
