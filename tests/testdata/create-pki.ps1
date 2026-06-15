# Builds a fresh, HA/SPIFFE-capable test PKI under testdata/pki, modeled on doc/ha/create-pki.sh.
# Separate from the existing testdata/ca PKI, which is left untouched. Can be run from any
# directory; it anchors itself to the script's own location.

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true

# Anchor to the script's own directory (tests/testdata) so paths resolve regardless of cwd.
Set-Location -LiteralPath $PSScriptRoot

$PkiRoot = 'pki'
$TrustDomain = 'ziti.test'
$RootNotBefore = (Get-Date).ToUniversalTime().AddHours(-24).ToString('yyyy-MM-ddTHH:mm:ss') + 'Z'
$IntermediateNotBefore = (Get-Date).ToUniversalTime().AddHours(-12).ToString('yyyy-MM-ddTHH:mm:ss') + 'Z'

# Self-signed root CA. --trust-domain makes every cert issued under it SPIFFE-capable.
ziti pki create ca --pki-root $PkiRoot --trust-domain $TrustDomain --ca-file root --ca-name 'Ziti Test Root CA' --not-before $RootNotBefore

# Controller 1: intermediate (signing) cert
ziti pki create intermediate --pki-root $PkiRoot --ca-name root --intermediate-file ctrl1 --intermediate-name 'Controller One Signing Cert' --not-before $IntermediateNotBefore

# Controller 1: server cert
ziti pki create server --pki-root $PkiRoot --ca-name ctrl1 --dns localhost --ip 127.0.0.1 --server-name ctrl1 --spiffe-id 'controller/ctrl1'

# Controller 1: client cert
ziti pki create client --pki-root $PkiRoot --ca-name ctrl1 --client-name ctrl1 --spiffe-id 'controller/ctrl1'

# Controller 1: wildcard alt server cert (only SAN is *.wildcard.test). Used by the
# wildcard-oidc-server config set / Test_OidcDiscoveryEndpoints_WildcardIssuer to exercise
# OIDC issuer derivation from a wildcard server-cert SAN.
ziti pki create server --pki-root $PkiRoot --ca-name ctrl1 --server-file ctrl1-wildcard --server-name ctrl1-wildcard --dns '*.wildcard.test'

# Controller 2: intermediate (signing) cert
ziti pki create intermediate --pki-root $PkiRoot --ca-name root --intermediate-file ctrl2 --intermediate-name 'Controller Two Signing Cert' --not-before $IntermediateNotBefore

# Controller 2: server cert
ziti pki create server --pki-root $PkiRoot --ca-name ctrl2 --dns localhost --ip 127.0.0.1 --server-name ctrl2 --spiffe-id 'controller/ctrl2'

# Controller 2: client cert
ziti pki create client --pki-root $PkiRoot --ca-name ctrl2 --client-name ctrl2 --spiffe-id 'controller/ctrl2'

# Controller 3: intermediate (signing) cert
ziti pki create intermediate --pki-root $PkiRoot --ca-name root --intermediate-file ctrl3 --intermediate-name 'Controller Three Signing Cert' --not-before $IntermediateNotBefore

# Controller 3: server cert
ziti pki create server --pki-root $PkiRoot --ca-name ctrl3 --dns localhost --ip 127.0.0.1 --server-name ctrl3 --spiffe-id 'controller/ctrl3'

# Controller 3: client cert
ziti pki create client --pki-root $PkiRoot --ca-name ctrl3 --client-name ctrl3 --spiffe-id 'controller/ctrl3'

# Routers are signed by controller 1's intermediate. Each router uses a single key
# shared by its server cert (link/edge listeners) and client cert (ctrl channel).

# Router 001: key
ziti pki create key --pki-root $PkiRoot --ca-name ctrl1 --key-file 001

# Router 001: server cert
ziti pki create server --pki-root $PkiRoot --ca-name ctrl1 --key-file 001 --server-file 001-server --server-name 001 --dns localhost --ip 127.0.0.1 --spiffe-id 'router/001'

# Router 001: client cert
ziti pki create client --pki-root $PkiRoot --ca-name ctrl1 --key-file 001 --client-file 001-client --client-name 001 --spiffe-id 'router/001'

# Router 002: key
ziti pki create key --pki-root $PkiRoot --ca-name ctrl1 --key-file 002

# Router 002: server cert
ziti pki create server --pki-root $PkiRoot --ca-name ctrl1 --key-file 002 --server-file 002-server --server-name 002 --dns localhost --ip 127.0.0.1 --spiffe-id 'router/002'

# Router 002: client cert
ziti pki create client --pki-root $PkiRoot --ca-name ctrl1 --key-file 002 --client-file 002-client --client-name 002 --spiffe-id 'router/002'
