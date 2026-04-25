# Release 2.0.0

## What's New

This is the next major version release of OpenZiti, following the 1.0 release in April 2024.
Of particular note is that HA controllers are now considered ready for general use.
This release also introduces a new permissions model, OIDC/JWT token-based enrollment, 
clustering performance improvements, and a number of other features and fixes. Because 
some of these changes are not backwards compatible with older routers, we're marking this 
as a major version bump.

### HA Controllers are now considered ready for general use

This is a pretty big milestone and marks the completion of work that's been ongoing for a couple of years.
The HA work has brought with it some notable changes. Authentication now uses JWTs by default. Using JWTs
means the controllers don't need to store session and propagate them to the routers. This removes a bottleneck
from the network and allows the load to be more easily distributed among controllers and routers.

To support distributed authentication, the routers now get a bespoke version of the data model. In addition
to enabling distributed authentication this will allow us to remove the need for service polling and further 
reduce the load on controllers in the future.

### Router Compatibility

Related to the JWT work, routers with version 2.+ will only work with controllers that are version 2.+. This means
to upgrade your network, controllers should be upgraded first. Routers can then be upgraded individually.

2.x routers should still work fine with older router versions.

We try very hard to avoid breaking changes like this, but sometimes the engineering trade-offs lead there. This change
was first made in the 1.7 release. That release has not been marked stable, and we have no plans to do so, because
of the backwards incompatibility. 

If you need to support in the 1.6.12+ range, see the section "OIDC enabled by default".

### New Permissions Model (BETA)

As one feature goes out of beta, another arrives into beta. This release introduces a new permissions system
for more fine grained control to the management API. It's not expected to change, but may do so based on feedback
from users.

### CLI Reorganization

The `ziti` CLI has been reorganized to consolidate commands that were previously 
spread across `ziti edge` and `ziti fabric` into unified top-level commands. Entity 
management is now available directly via `ziti create`, `ziti delete`, `ziti list`, 
and `ziti update`. Session management has been simplified with top-level `ziti login`. 
The existing `ziti edge` and `ziti fabric` command trees remain available.

**Breaking change:** `ziti create ca` now creates a Ziti edge Certificate Authority 
(previously it created a PKI CA). PKI CA creation is still available via `ziti pki create ca`.

See [CLI Reorganization Details](#cli-reorganization-details) for the full command mapping.

### Updated Release Process

We have moved to creating `-preN` releases for major and minor versions. This way we can put out release candidates,
or feature previews and put them through internal testing and let interested folks from the community try them out.
Then, when we're ready, we can run the full validation suite against the last pre-release and retag it. 

Patch releases won't have `-preN` and should contain only high priority bug fixes.

### Deprecation Cleanup

Since we already have a breaking change, we're removing some other backwards compatibility code.

* Controller managed links 
    * Router managed links were introduced in v0.30.0. 
    * If you're upgrading from an older versions, you'll want to upgrade to the latest 1.x release before jumping to 2.x
    * Github tracking issue: https://github.com/openziti/ziti/issues/3512
* `ziti edge create identity <type>`
    * Identity types other than router were removed in v0.30.2
    * The `type` can be dropped from the CLI command
    * Github tracking issue: https://github.com/openziti/ziti/issues/3532
* Terminator create/update/delete events
    * These have been superseded by entity change events, which also have create/update/delete events for terminators
    * Entity change events were introduced in v0.28.0
    * Github tracking issue: https://github.com/openziti/ziti/issues/3531
* `xgress_edge_tunnel` v1
    * This is the first implementation of the tunneler in edge-router code (ER/T) which used legacy api sessions and services
    * The v2 version uses the router data model and was introduced in v0.30.x
    * Github tracking issue: https://github.com/openziti/ziti/issues/3516
* Service policy filter `type = 1` / `type = 2`
    * Service policy list queries now expect the string form (`type = "Dial"`, `type = "Bind"`) matching the REST API
    * The integer form was an undocumented side effect of the internal storage format and never worked with the documented filter names
    * Github tracking issue: https://github.com/openziti/ziti/issues/3818

### Legacy Session Deprecation

OIDC sessions are now preferred. They are the default, or will become the default for SDKs and tunnelers. They are also required
when running HA. Legacy API and service session are now deprecated and will be removed in the OpenZiti v3.0.0 release. 

### Additional Features

* Controllers can now optionally bind APIs using an OpenZiti identity
* `ziti edge login` now supports the `--network-identity` flag to authenticate and establish connections through the Ziti overlay network
* `ziti edge login` now supports using a bearer token with `--token` for authentication. The token is expected to be
  provided as just the JWT, not with the "Bearer " prefix
* Identity configuration can now be loaded from files or environment variables for flexible deployment scenarios
* Identities can now be provisioned just-in-time through OIDC/JWT token-based enrollment
* Multiple model updates can now be in-flight at the same time, improving clustering performance
* Authentication-related model updates can now be non-blocking and even dropped if the system is too busy
* Routers now provide more error context to SDKs for terminator errors, enabling better retry behavior
* New `proxy.v1` config type for dynamic service proxies (originally released in 1.7.0)
* New alert event type for surfacing operational issues to network operators - Beta (originally released in 1.7.0)
* New Azure Service Bus event sink for streaming controller events, contributed by @ffaraone (originally released in 1.7.0)
* Bundled ZAC upgraded to 4.0
* Build updated to Go 1.25
* CLI cleaned up to remove calls to `os.Exit`, making it more friendly for embedding
* OIDC is now enabled by default
* Controller Edge APIs now return `WWW-Authenticate` response headers on `401 Unauthorized` responses, giving clients actionable information about which auth methods are accepted and what went wrong
* HA Controllers can be marked as 'preferredLeader' via config
* Dynamic cost range for smart routing expanded beyond the previous 64K limit
* Dial failures now return the circuit ID and error information for easier debugging
* Router-to-controller control channels now support multiple underlays with priority-based message routing
* The dialing identity's ID and name are now forwarded to the hosting SDK
* Controllers can now dial routers to establish control channels, enabling connectivity when routers are behind firewalls (Beta)
* Refresh-token revocations are now batched and best-effort, removing the database/raft bottleneck on token refreshes
* [OIDC discovery endpoint extensions](#oidc-discovery-endpoint-extensions) - OpenZiti-specific endpoint URLs in the OIDC discovery document
* `ziti edge quickstart` now always runs in HA mode. The `ha` subcommand has been removed. Use
  `ziti edge quickstart join` to add additional members to the cluster. Note: existing quickstart instances
  are not compatible with the new HA-only mode and will need to be recreated.
* The `--clustered` flag on `ziti create config controller` has been removed; the generated config is always
  cluster-ready. If you have scripts passing `--clustered`, remove it.
* [Connect events pool](#connect-events-pool) - fixes a goroutine leak when routers reconnect and ensures per-router event ordering
* [Multiple DNS upstreams](#multiple-dns-upstreams) - the tunneler can now fan out recursive queries to several upstream resolvers in parallel

## OIDC Discovery Endpoint Extensions

The OIDC discovery document (`/.well-known/openid-configuration`) now includes a vendor-specific
`openziti_endpoints` field. This lets SDKs discover OpenZiti's custom login and MFA endpoints at
runtime instead of hardcoding paths.

The field contains absolute URLs for each endpoint, derived from the issuer the client connected to:

```json
{
  "issuer": "https://controller.example.com:1280/oidc",
  "authorization_endpoint": "https://controller.example.com:1280/oidc/authorize",
  "token_endpoint": "https://controller.example.com:1280/oidc/oauth/token",
  "...other standard OIDC fields...",
  "openziti_endpoints": {
    "password":           "https://controller.example.com:1280/oidc/login/password",
    "cert":               "https://controller.example.com:1280/oidc/login/cert",
    "ext_jwt":            "https://controller.example.com:1280/oidc/login/ext-jwt",
    "totp":               "https://controller.example.com:1280/oidc/login/totp",
    "totp_enroll":        "https://controller.example.com:1280/oidc/login/totp/enroll",
    "totp_enroll_verify": "https://controller.example.com:1280/oidc/login/totp/enroll/verify",
    "auth_queries":       "https://controller.example.com:1280/oidc/login/auth-queries"
  }
}
```

| Key                | Method(s)   | Description                                       |
|--------------------|-------------|---------------------------------------------------|
| `password`         | POST        | Username/password authentication                  |
| `cert`             | POST        | Client certificate authentication                 |
| `ext_jwt`          | POST        | External JWT authentication                       |
| `totp`             | POST        | TOTP code verification for MFA                    |
| `totp_enroll`      | POST/DELETE | Start (POST) or delete (DELETE) TOTP enrollment   |
| `totp_enroll_verify`| POST       | Verify a TOTP enrollment code                     |
| `auth_queries`     | GET         | Retrieve pending authentication queries           |

When the controller serves edge-oidc on multiple web servers, each discovery response reflects
the issuer (and port) the client connected to.

## Connect Events Pool

The controller now uses per-router, single-worker goroutine pools to process identity
connect/disconnect events. Previously each router connection spawned a dedicated
goroutine that was never cleaned up on disconnect, leaking a goroutine per reconnect
cycle. Under churn (e.g., chaos testing with hundreds of routers) this could accumulate
tens of thousands of leaked goroutines and destabilize the controller.

Using a single-worker pool per router also ensures that events from the same router are
always processed in FIFO order. Previously, a shared multi-worker pool could process a
full-state sync after a newer incremental event from the same router, causing identities
to be incorrectly marked as disconnected.

The pool is configurable in the controller config file:

```yaml
connectEvents:
  queueSize: 5    # per-router work queue depth (default: 5)
  idleTime:  30s  # worker idle timeout before exit (default: 30s)
```

The defaults are suitable for most deployments. Each router's worker starts on demand
and exits after the idle timeout, so no goroutines are held when there is no work.

Note: the `minWorkers` and `maxWorkers` settings have been removed. Each router's pool
is fixed at one worker for correctness.

## Community Contributors

Thank you to the following community members for their contributions:

* @ffaraone - Azure Service Bus event sink
* @dmuensterer - OIDC token refresh fixes
* @nenkoru - Controller isleader health check endpoint
* Jan Starkl - UPDB auth attempts fix
* Mamy Ratsimbazafy - uint16 port range fix

## Basic Permission System (BETA)

Added a basic permission system that allows more control over identity access to controller management API operations. 
This replaces the previous binary admin/non-admin model with a more flexible permission system.

**NOTE:** This feature is in BETA, primarily so we can get feedback on which permissions make sense. The implementation is unlikely to change
but the set of exposed permissions may grow, shrink or change based on user feedback. 

### Permission Model

The permission system supports three levels of authorization:

  1. **Global Permissions**: System-wide access levels
     - `admin` - Full access to all operations. This is still controlled by the `isAdmin` flag on identity
     - `admin_readonly` - Read-only access to all resources except debugging facilities inspect and validate

  2. **Entity-Level Permissions**: Full CRUD access to specific entity types
     - Granting an entity-level permission (e.g., `service`) provides complete create, read, update, and delete access for that entity type

  3. **Action-Level Permissions**: Specific operation access on entity types
     - Fine-grained control using the pattern `<entity>.<action>` (e.g., `service.read`, `identity.update`)
     - Supports `create`, `read`, `update`, and `delete` actions per entity type

### Supported Entity Permissions

The following entity-level permissions are available:

- `auth-policy` - Authentication policy management
- `ca` - Certificate Authority management
- `config` - Configuration management
- `config-type` - Configuration type management
- `edge-router-policy` - Edge router policy management
- `enrollment` - Enrollment management
- `external-jwt-signer` - External JWT signer management
- `identity` - Identity management
- `posture-check` - Posture check management
- `router` - Edge and transit router management
- `service` - Service management
- `service-policy` - Service policy management
- `service-edge-router-policy` - Service edge router policy management
- `terminator` - Terminator management
- `ops` - Operational resources (API sessions, sessions, circuits, links, inspect and validate)

### Permission Assignment

Permissions are assigned to identities via the `permissions` field in the identity resource. Multiple permissions can be granted to a single identity, and permissions are additive.

### Cross-Entity Operations

Listing related entities through an entity's endpoints requires appropriate permissions for the related entity type. For example:
- Listing services for a service-policy requires `service.read` permission
- Listing identities for an edge-router-policy requires `identity.read` permission
- Listing configs for a service requires `config.read` permission

**NOTE:** 
More permissions than expected may be required when performing actions through the CLI or ZAC. Take for example, when an identity 
has `config.create` and is attempting to create a new config. The CLI may fail if the identity doesn't have `config-type.read`
as well because it will need to look up the config type id that corresponds to the given config type name.

Similar cross entity read permissions may be required when creating services.

### Admin Protection

Non-admin identities cannot:
- Create identities with the `isAdmin` flag
- Create identities with any permissions granted
- Modify admin-related fields on existing identities
- Update or delete admin identities
- Grant permissions to identities

These protections ensure that privilege escalation is prevented and admin access remains controlled.


## Binding Controller APIs With Identity

Controller APIs can now be bound to an OpenZiti overlay network identity, allowing secure communication through
the Ziti network. This is useful for scenarios where you want to expose controller APIs only through the overlay
network rather than on a standard network interface.

### Configuration Structure

A standard `bindPoint` configuration looks like this:
```text
    bindPoints:
      - interface: 127.0.0.1:18441
        address: 127.0.0.1:18441
```

To bind controller APIs to an OpenZiti identity, add an additional `identity` block to your `bindPoints`. The
identity configuration specifies where to load the Ziti identity file and which service to bind it to:

```text
    bindPoints:
      - interface: 127.0.0.1:18441
        address: 127.0.0.1:18441
      - identity:
          file: "c:/temp/ctrl.testing/ctrl.identity.json"
          service: "mgmt"
```

### Supported Configuration Options

- `file`: Path to a Ziti identity JSON file containing the controller's identity and enrollment certificate
- `env`: Name of an environment variable containing a base64-encoded Ziti identity (alternative to `file`)
- `service`: The name of the Ziti service to bind the controller API to

### Using Environment Variables

For deployments where storing identity files on disk is not preferred, you can reference a base64-encoded
identity file from an environment variable. The environment variable should contain the base64-encoded contents
of the identity JSON file.

For example, if an environment variable named `ZITI_CTRL_IDENTITY` contains a base64-encoded identity file:

```text
    bindPoints:
      - interface: 127.0.0.1:18441
        address: 127.0.0.1:18441
      - identity:
          env: ZITI_CTRL_IDENTITY
          service: "mgmt"
```

### IPv6 Support

Both IPv4 and IPv6 addresses are supported for standard bind points. IPv6 addresses should be specified in bracket
notation with a port number:

```text
    bindPoints:
      - interface: "[::1]:18441"
        address: "[::1]:18441"
      - identity:
          file: "/path/to/identity.json"
          service: "mgmt"
```

## CLI Enhancements for Identity-Based Connections

The `ziti edge login` command and REST client utilities have been enhanced to support identity-based connections
through the Ziti overlay network.

### New `--network-identity` Flag for `ziti edge login`

The `ziti edge login` command now includes a `--network-identity` flag that allows you to authenticate to a Ziti
controller through the overlay network using a Ziti identity:

```bash
ziti edge login https://ziti.mgmt.apis.local:1280 \
  --username myuser \
  --password mypass \
  --network-identity /path/to/identity.json
```

This is useful when the controller is only accessible through the Ziti overlay network or when you want to ensure
all communication to the controller flows through the overlay for security purposes.

### Identity Resolution Order

When establishing connections, identities are resolved in the following order:

1. **Command-line flag**: The `--network-identity` flag takes precedence
2. **Environment variable**: If `ZITI_CLI_NETWORK_ID` is set and contains a base64-encoded identity, it is used
3. **Cached identity file**: If a network identity was saved from a previous login in the Ziti config directory, it may be used

This layered approach allows for flexibility in deployment scenarios:
- Development: Use command-line flags for quick testing
- Automation: Use environment variables in CI/CD pipelines
- Production: Cache identities securely for repeated access

#### Dialing Modes When Authenticating

The CLI supports two dialing modes:

**Intercept-based Dialing (Default)**
By default, URLs are expected to leverage intercepts. Create a service with an appropriate intercept config and use
the intercept address when dialing. This is the standard mode for most use cases. For example, given a service with
the intercept `ziti.mgmt.apis.local`
```bash
ziti edge login https://ziti.mgmt.apis.local:1280 \
  --username myuser \
  --password mypass \
  --network-identity /path/to/identity.json
```

**Identity-aware Dialing (Addressable Terminators)**
To support addressable terminators-based dialing, specify a user in the URL. This activates dial-by-identity
functionality. The URL format should be `identity-to-dial@service-name-to-dial`. For example:
```bash
ziti edge login https://my-identity@my-service:1280 \
  --username myuser \
  --password mypass \
  --network-identity /path/to/identity.json
```

In this mode, the transport extracts the identity from the URL and uses it to establish a direct connection to
the specified service via the addressable terminator.


## OIDC/JWT Token-based Enrollment

OpenZiti now supports provisioning identities just-in-time through OIDC/JWT token enrollment. External identity 
providers can be configured to allow identities to enroll using JWT tokens, with support for the resulting 
identities to use certificate or token authentication.

### External JWT Signer Configuration

External JWT signers are configured via the Edge Management API to define enrollment behavior with the following new 
enrollment-specific properties:

- **enrollToCertEnabled** - When enabled, identities can exchange a JWT token and a certificate signing request (CSR) 
        for a client certificate during enrollment. The certificate can then be used for standard certificate-based
        authentication.

- **enrollToTokenEnabled** - When enabled, identities can use a JWT token to enroll. The current token or future tokens
        may be used for authentication.

- **enrollNameClaimsSelector** - Specifies which JWT claim contains the identity name. Accepts a JSON pointer 
        (e.g., `/preferred_username`) or a simple property name (e.g., `preferred_username`, automatically converted to
        `/preferred_username`). Defaults to `/sub` if not specified. The extracted value becomes the identity name in Ziti.

- **enrollAttributeClaimsSelector** - Specifies which JWT claims to extract as identity attributes during enrollment.
        Accepts a JSON pointer (e.g., `/roles`) or a simple property name (e.g., `roles`). Extracted attributes are 
        applied to the newly enrolled identity for use in authorization policies.

- **enrollAuthPolicyId** - Specifies the authentication policy to apply to newly enrolled identities. This determines
        what authentication methods are available for the identity post-enrollment.

Additionally the existing property named **claimsProperty** that specifies external id to match identities to:

- now supports a JSON pointer (e.g., `/id`) or a simple property name (e.g., `id`)
- is used to populate the `externalId` field of the identity

### Enrollment Paths

#### Certificate Enrollment (enrollToCertEnabled)

When certificate enrollment is enabled, unauthenticated users can:

1. Obtain a list of available IdPs from the public Edge Client API `GET /external-jwt-signers` endpoint, where 
   `enrollToCertEnabled` is set to `true`
2. Obtain a JWT from the configured OIDC provider
3. Generate a certificate signing request (CSR)
4. Submit an enrollment request with the JWT and CSR
5. Have their identity created in Ziti with attributes extracted from JWT claims
6. Receive a signed client certificate for certificate-based authentication

#### Token Enrollment (enrollToTokenEnabled)

When token enrollment is enabled, unauthenticated users can:

1. Obtain a list of available IdPs from the public Edge Client API `GET /external-jwt-signers` endpoint, where 
   `enrollToTokenEnabled` is set to `true`
1. Obtain a JWT from the configured OIDC provider
2. Submit an enrollment request with the JWT
3. Have their identity created in Ziti with attributes extracted from JWT claims
4. Receive a Ziti API token for token-based authentication

### Edge Management API

The Edge Management API provides full CRUD operations for configuring external JWT signers:

- `POST /external-jwt-signers` - Create a new external JWT signer with all configuration options
- `GET /external-jwt-signers` - List all configured external JWT signers
- `GET /external-jwt-signers/{id}` - Retrieve a specific signer configuration
- `PUT /external-jwt-signers/{id}` - Update all fields of a signer
- `PATCH /external-jwt-signers/{id}` - Partially update a signer
- `DELETE /external-jwt-signers/{id}` - Delete a signer

### Edge Client API

The Edge Client API exposes a reduced set of external JWT signer information for unauthenticated enrollment requests:

- `GET /external-jwt-signers` - List available JWT signers with enrollment capabilities

The client API response includes the following fields for each signer:

- `name` - Signer name
- `externalAuthUrl` - URL where users obtain JWT tokens
- `clientId` - OIDC client ID
- `scopes` - Requested OIDC scopes
- `openIdConfigurationUrl` - OIDC discovery endpoint
- `audience` - Expected token audience
- `targetToken` - Token type to use (ACCESS or ID)
- **`enrollToCertEnabled`** - Flag indicating certificate enrollment is available
- **`enrollToTokenEnabled`** - Flag indicating token enrollment is available

### CLI Commands

**Create an external JWT signer with enrollment options:**
```
ziti edge controller create ext-jwt-signer <name> <issuer> \
  --jwks-endpoint <url> \
  --audience <audience> \
  --enroll-to-cert \
  --enroll-to-token=false \
  --enroll-name-claims-selector preferred_username \
  --enroll-attr-claims-selector roles \
  --enroll-auth-policy <policy-id-or-name>
```

**Update enrollment options on an existing signer:**
```
ziti edge controller update ext-jwt-signer <name|id> \
  --enroll-to-cert \
  --enroll-auth-policy <policy-id-or-name>
```

**List external JWT signers:**
```
ziti edge controller list ext-jwt-signers
```

## Clustering Performance Improvements

In previous releases, model updates were submitted to raft one at at time. This prevented 
raft from being efficient by allowing command batching. This release allows multiple 
model updates to be in-flight at the same time. 

New Configuration Options

1. Raft Apply Timeout (cluster.applyTimeout)

Location: Controller configuration file, under the cluster section
Type: Duration
Default: 5s
Description: Timeout for applying commands to the Raft distributed log. Commands that exceed this timeout will trigger adaptive rate limiter backoff.

Example:
```
cluster:
  applyTimeout: 10s
```

2. Cluster Rate Limiter Configuration (cluster.rateLimiter)

A new adaptive rate limiter that controls the submission of commands to the Raft cluster. Unlike the existing command rate limiter, this specifically manages in-flight Raft operations with adaptive window sizing.

Configuration Structure:
```
cluster:
  rateLimiter:
    enabled: true
    minSize: 5
    maxSize: 250
    timeout: 30s
```

Sub-options:

  - enabled (boolean)
    - Default: true
    - Description: Enable/disable adaptive rate limiting for Raft command submission
  - minSize (integer)
    - Default: 5
    - Minimum: 1
    - Description: Minimum window size for concurrent in-flight Raft operations
  - maxSize (integer)
    - Default: 250
    - Description: Maximum window size for concurrent in-flight Raft operations. Must be >= minSize
  - timeout (duration)
    - Default: 30s
    - Description: Time after which outstanding work is assumed to have failed if not marked completed

3. Restart Self on Snapshot (cluster.restartSelfOnSnapshot)

Location: Controller configuration file, under cluster section
Type: Boolean
Default: false
Description: When true, the controller will automatically restart itself when restoring a snapshot to an initialized system. When false, the controller will exit with code 0, requiring external process management to restart it.

Example:
```
cluster:
    restartSelfOnSnapshot: true
```

### New Metrics

The adaptive rate limiter exposes three new metrics:

  1. raft.rate_limiter.queue_size (gauge)
    - Current number of operations queued/in-flight
  2. raft.rate_limiter.work_timer (timer)
    - Duration of rate-limited operations
  3. raft.rate_limiter.window_size (gauge)
    - Current adaptive window size

## Rate Limiter Algorithm Improvements

The adaptive rate limiter tracker now uses a success rate metric to decide when to grow or shrink its
concurrency window, instead of using raw queue position. An exponentially decaying histogram tracks the
ratio of successes to backoffs. When the success rate exceeds a configurable threshold, the window is
grown by a multiplicative increase factor. When it falls below the threshold, the window is shrunk by a
multiplicative decrease factor. The check intervals for increase and decrease are independently configurable.

This approach produces smoother, more stable window adjustments under varying load, avoiding the
over-aggressive shrinking that could occur when queue position alone was used as the signal.

This specific rate limiter implementation is used in three places:
* **Controller TLS handshake rate limiting** - controls the rate of incoming TLS handshakes on the controller
* **Raft command submission** - controls the rate of commands submitted to the Raft distributed log
* **Router control channel rate limiting** - controls the rate of requests from the router to the controller

Note: this work was done in advance of enabling the TLS handshake rate limiter by default. The TLS
handshake rate limiter is not yet enabled by default, but can be enabled via the `tls.rateLimiter`
configuration section.

New configuration options (available under each `rateLimiter` section):

  - successThreshold (float)
    - Default: 0.9
    - Description: Success rate threshold above which the window size will be increased and below which it will be decreased
  - increaseFactor (float)
    - Default: 1.02
    - Description: Multiplier applied to the current window size when growing. Must be greater than 1
  - decreaseFactor (float)
    - Default: 0.9
    - Description: Multiplier applied to the current window size when shrinking. Must be between 0 and 1
  - increaseCheckInterval (integer)
    - Default: 10
    - Description: Number of successes between window size increase checks
  - decreaseCheckInterval (integer)
    - Default: 10
    - Description: Number of backoffs between window size decrease checks

## Background Processing for Identity Updates

Identity environment and authenticator updates that occur during authentication are now processed asynchronously in the background. 
This prevents authentication requests from blocking when the system is under load, significantly improving resilience during thundering herd scenarios.

When the background queue fills up, updates can be dropped as they will be refreshed on the next authentication attempt. 
This allows the system to gracefully handle load spikes without impacting authentication performance.

For now, dropping entries when the queue fills will be disabled by default, but can be enabled, see below.

### Configuration

A new `command.background` configuration section controls the background processing behavior:

```yaml
  command:
    background:
      enabled: true           # Enable background processing (default: true)
      queueSize: 1000        # Maximum queue size (default: 1000)
      dropWhenFull: true     # Drop updates when queue is full (default: false)
      delayThreshold: 50ms   # The threshold for how long updates are taking before starting to background updates
```

Note that the `commandRateLimiter` configuration section may instead be specified under `command` as `rateLimiter`.

Example:

```yaml
  command:
    background:
      enabled: true
      queueSize: 250
      dropWhenFull: false
      delayThreshold: 50ms
    rateLimiter:
      enabled:   true
      maxQueued: 25
```

Note that if command rate limiter configuration is specified in both locations, the settings under `command` will take 
precedence. The standalone `commandRateLimiter` section may be deprecated in the future.

### Metrics

When background processing is enabled, the following metrics are exposed:

- command.background.queue_size - Current number of queued background tasks
- command.background.worker_count - Current number of worker goroutines
- command.background.busy_workers - Number of workers currently processing tasks
- command.background.work_timer - Timer tracking background task execution (includes histogram, meter, and count)
- command.background.dropped_entries - Count of dropped updates when queue is full (only when dropWhenFull is enabled)

## New proxy.v1 Config Type

*Originally released in 1.7.0*

Added support for dynamic service proxies with configurable binding and protocol options.
This allows Edge Routers and Tunnelers to create proxy endpoints that can forward traffic for Ziti services.

This differs from intercept.v1 in that intercept.v1 will intercept traffic on specified
IP addresses or DNS entries to forward to a service using tproxy or tun interface,
depending on implementation.

A proxy on the other hand will just start a regular TCP/UDP listener on the configured port,
so traffic will have to be configured for that destination.

Example proxy.v1 Configuration:

```
  {
    "port": 8080,
    "protocols": ["tcp"],
    "binding": "0.0.0.0"
  }
```

Configuration Properties:
  - port (required): Port number to listen on (1-65535)
  - protocols (required): Array of supported protocols (tcp, udp)
  - binding (optional): Interface to bind to. For the ER/T defaults to the configured lanIF config property.

This config type is currently supported by the ER/T when running in either proxy or tproxy mode.

## Alert Events (BETA)

*Originally released in 1.7.0*

A new alert event type has been added to allow Ziti components to emit alerts for issues that network operators can address.
Alert events are generated when components encounter problems such as service configuration errors or resource
availability issues.

Alert events include:
  - Alert source type and ID (currently supports routers, with controller and SDK support planned for future releases)
  - Severity level (currently supports error, with info and warning planned for future releases)
  - Alert message and supporting details
  - Related entities (router, identity, service, etc.) associated with the alert

Example alert event when a router cannot bind a configured network interface:

```
  {
    "namespace": "alert",
    "event_src_id": "ctrl1",
    "timestamp": "2021-11-08T14:45:45.785561479-05:00",
    "alert_source_type": "router",
    "alert_source_id": "DJFljCCoLs",
    "severity": "error",
    "message": "error starting proxy listener for service 'test'",
    "details": [
      "unable to bind eth0, no address"
    ],
    "related_entities": {
      "router": "DJFljCCoLs",
      "identity": "DJFljCCoLs",
      "service": "3DPjxybDvXlo878CB0X2Zs"
    }
  }
```

Alert events can be consumed through the standard event system and logged to configured event handlers for monitoring and alerting purposes.

These events are currently in Beta, as the format is still subject to change. Once they've been in use in production for a while
and proven useful, they will be marked as stable.

## Azure Service Bus Event Sink

*Originally released in 1.7.0. Contributed by @ffaraone.*

Adds support for streaming controller events to Azure Service Bus.
The new logger enables real-time event streaming from the OpenZiti controller to Azure Service Bus
queues or topics, providing integration with Azure-based monitoring and analytics systems.

To enable the Azure Service Bus event logger, add configuration to the controller config file under the events section:

```
  events:
    serviceBusLogger:
      subscriptions:
        - type: circuit
        - type: session
        - type: metrics
          sourceFilter: .*
          metricFilter: .*
        # Add other event types as needed
      handler:
        type: servicebus
        format: json
        connectionString: "Endpoint=sb://your-namespace.servicebus.windows.net/;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=your-key"
        topic: "ziti-events"          # Use 'topic' for Service Bus topic
        # queue: "ziti-events-queue"  # Or use 'queue' for Service Bus queue
        bufferSize: 100                # Optional, defaults to 50
```

- Required configuration:
    - format: Event format, currently supports only json
    - connectionString: Azure Service Bus connection string
    - Either topic or queue: Destination name (mutually exclusive)

- Optional configuration:
    - bufferSize: Internal message buffer size (default: 50)

## OIDC is now enabled by default

The controller now automatically adds the `edge-oidc` API binding to any web listener that hosts
the `edge-client` API, even when `edge-oidc` is not explicitly listed in the `web` section of the
configuration. This means OIDC-based authentication is available out of the box without requiring
changes to existing controller configurations.

### Where OIDC binds

The `edge-oidc` binding is added to the same web listener(s) as `edge-client`. If `edge-client` is
hosted on `127.0.0.1:1280`, the OIDC endpoints will be available on that same address under the
`/oidc` path (e.g. `https://127.0.0.1:1280/oidc/.well-known/openid-configuration`).

The controller capabilities returned by `GET /version` will include `OIDC_AUTH` and the
`edge-oidc` entry will be present in `apiVersions` when OIDC is active.

### Opting out

If OIDC should not be enabled automatically, set `disableOidcAutoBinding: true` under `edge.api`:

```yaml
edge:
  api:
    address: 127.0.0.1:1280
    sessionTimeout: 30m
    disableOidcAutoBinding: true
```

When `disableOidcAutoBinding` is `true`, OIDC will only be active if `edge-oidc` is explicitly
listed as a binding in the `web` section. 

When OIDC is not enabled, all (SDK) clients will revert to using legacy authentication.
Legacy authentication does not support multiple controllers or HA in general. Clients will treat
their controller as if it is a single controller instance and will function as if no other controllers
exist.


## WWW-Authenticate Headers

The controller's Edge APIs now include `WWW-Authenticate` headers on `401 Unauthorized` responses,
per issuer: https://github.com/openziti/ziti/issues/3356. These headers provide insight into why
a token was rejected. The main benefit of these headers is to convey information for JWT backed
API Sessions and API Session authentication where a token may not have been provided (missing),
is used beyond its expiration date (expired), or the token has become invalid for any other
reason (invalid).

`www-authenticate` headers are provided as a single header instance with multiple challenge values
separate by commas.

### No Credentials Provided

When a request hits a protected endpoint without any credentials, a single `WWW-Authenticate` header
is returned listing both accepted auth schemes as comma-separated challenges:

```
WWW-Authenticate: zt-session realm="zt-session" error="missing" error_description="no matching token was provided",Bearer realm="openziti-oidc" error="missing" error_description="no matching token was provided"
```

### Token Errors

When a token is present but cannot be accepted, the header identifies the scheme and what went wrong:

```
WWW-Authenticate: Bearer realm="openziti-oidc" error="expired" error_description="token expired"
WWW-Authenticate: Bearer realm="openziti-oidc" error="invalid" error_description="token is invalid"
WWW-Authenticate: zt-session realm="zt-session" error="invalid" error_description="token is invalid"
```

### OIDC External JWT — Primary Authentication

When an auth policy requires an external JWT signer for primary authentication (e.g., a PKCE flow
backed by an ext-jwt signer), the header identifies which signers are accepted and what went wrong.
Multiple accepted signers are pipe-delimited in the `id` and `issuer` parameters:

```
WWW-Authenticate: Bearer realm="openziti-primary-ext-jwt" error="missing" error_description="no matching token was provided" id="signer-id-1|signer-id-2" issuer="https://issuer1.example.com|https://issuer2.example.com"
```

The `error` value follows the same `missing`/`expired`/`invalid` pattern as standard bearer token errors.

### OIDC External JWT — Secondary / MFA Authentication

When an auth policy requires an external JWT signer as a secondary factor (step-up after primary
auth succeeds), the header identifies the single required signer. The `error` value follows the
same `missing`/`expired`/`invalid` pattern:

```
WWW-Authenticate: Bearer realm="openziti-secondary-ext-jwt" error="missing" error_description="no matching token was provided" id="<signer-id>" issuer="<issuer>"
```

### Anonymous Endpoints

Unauthenticated endpoints such as version information do not return `WWW-Authenticate` headers.

## HA Preferred Leaders

Controllers can be marked as a preferred leader. 

**Example Config**
```yaml
cluster:
  dataDir: /home/{{ .Model.MustVariable "credentials.ssh.username" }}/fablab/ctrldata
  preferredLeader: true
```

If a controller that is not marked preferredLeader becomes a preferredLeader, it will check 
if there's a node available that is marked as preferred. If there is one, or one later
joins the cluster, the non-preferred node will attempt to transfer leadership to the 
node that is marked as preferred.

## Expanded Dynamic Cost Range

The dynamic cost range for smart routing has been expanded beyond the previous 64K limit. Under high
load, terminators could saturate the cost space, making dynamic cost values meaningless for routing
decisions and leading to uneven load distribution. The expanded range allows for more granular cost
differentiation even under heavy load.

## Circuit ID and Error in Dial Failures

Dial failures now return the circuit ID and, when available, the error that caused the circuit to fail.
Previously, the circuit ID was only returned on successful dials. Note that SDKs will need to be
updated to surface the circuit id when a dial failure happens.

## Multi-Underlay Control Channels

Router-to-controller control channels now support multiple underlays with priority-based message routing.
This allows time-sensitive control messages (heartbeats, routing, circuit requests) to be separated from
operational data (metrics, inspections) across dedicated TCP connections, preventing bulk operations from
delaying user-affecting control plane traffic. This feature does not yet allow specifying multiple 
network interfaces to use, to load balance data across.

## Dialing Identity Forwarded to Hosting SDK

The identity ID and name of the dialing client are now forwarded to the hosting SDK when a circuit is
established. This allows hosting applications to identify which identity initiated the connection,
enabling identity-aware request handling on the server side. This will require SDK updates to add this
to the API for hosting applications.

## Controller-Initiated Control Channel Dials (BETA)

Controllers can now dial routers to establish control channels. Previously, routers were solely
responsible for dialing controllers. This feature is designed for deployments where one or more
controllers are in a private network that routers cannot reach, but the controllers can dial out.
A common scenario is an HA cluster where some controllers are publicly reachable and some are in
a private network. External routers connect to the public controllers normally, and the private
controllers dial out to the routers.

### Router Configuration

Routers can configure one or more control channel listeners. Each listener specifies a bind address,
an advertise address (reported to the controller), and optional groups for matching.

```yaml
ctrl:
  listeners:
    - bind: tls://0.0.0.0:6262
      advertise: tls://router.example.com:6262
      groups:
        - default
```

The router is the authoritative source of ctrl channel listener information, similar to link
listeners. When a router connects to any controller, it reports its configured `ctrlChanListeners`
and the controller data model is updated automatically. This means that in most deployments —
where the router can reach at least one controller — no manual configuration of listener addresses
is needed on the controller side.

The `ctrlChanListeners` field can also be set via the CLI for cases where a router cannot reach
any controller and thus cannot initialize the data model itself:

```bash
ziti edge update edge-router myRouter --bootstrap-ctrl-chan-listener 'tls://router.example.com:6262=group1,group2'
```

Groups default to `["default"]` if not specified.

### Controller Configuration

The controller dialer is disabled by default and must be explicitly enabled. When enabled, the
controller will dial routers that have control channel listeners configured and are not already
connected.

```yaml
ctrl:
  dialer:
    enabled: true
    groups:
      - default
    dialDelay: 30s
    minRetryInterval: 1s
    maxRetryInterval: 5m
    retryBackoffFactor: 1.5
    fastFailureWindow: 5s
    queueSize: 32
    maxWorkers: 10
```

- `enabled` - Enables the controller dialer (default: `false`)
- `groups` - List of groups to match against router listener groups (default: `["default"]`)
- `dialDelay` - Delay before the controller starts dialing after boot (default: `30s`)
- `minRetryInterval` - Minimum backoff delay between dial retries (default: `1s`)
- `maxRetryInterval` - Maximum backoff delay between dial retries (default: `5m`)
- `retryBackoffFactor` - Multiplier applied to the retry delay on each failure, jittered +/- 0.5 (default: `1.5`)
- `fastFailureWindow` - If a connection drops within this window after connecting, backoff continues rather than resetting (default: `5s`)
- `queueSize` - Maximum number of pending dial jobs in the worker pool queue (default: `32`)
- `maxWorkers` - Maximum number of concurrent dial workers (default: `10`)

The controller will only dial routers whose listener groups overlap with the controller's configured
groups. When a router advertises multiple ctrl channel listener addresses, the dialer rotates through
them on each failure so that an unreachable address does not block attempts to the others.

### Metrics

The controller dialer worker pool exposes the following metrics under the `ctrl_channel.dialer` prefix:

- `ctrl_channel.dialer.queue_size` - Current number of pending dial jobs in the queue
- `ctrl_channel.dialer.worker_count` - Current number of dial worker goroutines
- `ctrl_channel.dialer.busy_workers` - Number of workers currently executing a dial
- `ctrl_channel.dialer.work_timer` - Timer tracking the duration of each dial attempt

## SDK Inspection Support

New CLI commands have been added for inspecting SDK state, useful for diagnosing terminator and
connectivity issues.

**Note:** SDK inspection requires SDK support. Currently only the Go SDK supports inspection,
as of version 1.5.0. Other SDKs will need to add support before these commands can be used
with them.

### `ziti fabric inspect sdk`

Retrieves SDK context inspection data from identities connected to routers.

```
ziti fabric inspect sdk <target-selector> <identity-id>
```

The `<target-selector>` is a regex matching router IDs (use `.*` for all routers). The
`<identity-id>` is the identity whose SDK context you want to inspect. The command returns
detailed state from the connected SDK instance, including active services, terminators, and
connection status.

### `ziti agent tunnel dump-sdk`

Dumps SDK context information from a running `ziti tunnel` process via the IPC agent.

```
ziti agent tunnel dump-sdk
```

Returns JSON-formatted inspection data for all SDK contexts registered in the tunnel process,
including service listeners, connections, and terminator state.

## Revocation System Improvements

When a session is refreshed, the old refresh token's revocation is no longer created
synchronously through raft. Instead, revocations are queued in memory and flushed in
batches on a configurable interval. This removes the database and raft as a bottleneck
on token refreshes. If the old token is close to expiring, the revocation is skipped
entirely.

New configuration tunables under `edge.oidc`:

| Key | Default | Description |
|-----|---------|-------------|
| `revocationMinTokenLifetime` | unset | Skip revocation if the old token expires within this duration (must be < 50% of `refreshTokenDuration`) |
| `revocationBucketInterval` | `1m` | Bucket window for batching revocations before flushing through raft |
| `revocationBucketMaxSize` | `200` | Max revocations per raft entry |
| `revocationMaxQueued` | `25000` | Max revocations queued in memory before dropping |
| `revocationEnforcerFrequency` | `1m` | How often expired revocations are purged (leader only) |

## CLI Reorganization Details

The CLI has been reorganized so that edge and fabric entity management commands are 
available directly at the top level. The `ziti edge` and `ziti fabric` command trees 
remain fully functional — the new top-level commands are additional entry points, not 
replacements.

### Top-Level CRUD Commands

Entity create, delete, list, and update operations that previously required the 
`ziti edge` or `ziti fabric` prefix are now available directly:

| New command | Previous command |
|---|---|
| `ziti create identity ...` | `ziti edge create identity ...` |
| `ziti delete service ...` | `ziti edge delete service ...` |
| `ziti list edge-routers` | `ziti edge list edge-routers` |
| `ziti update identity ...` | `ziti edge update identity ...` |
| `ziti list circuits` | `ziti fabric list circuits` |
| `ziti delete terminator ...` | `ziti fabric delete terminator ...` |

All edge and fabric entities are available under the consolidated commands. When an 
entity name exists in both edge and fabric (e.g., `service`, `router`), the edge 
version is the default. Fabric equivalents are accessible with a `fabric-` prefix 
(e.g., `ziti list fabric-services`).

### Top-Level Login

Session management is now available at the top level:

| New command | Previous command |
|---|---|
| `ziti login` | `ziti edge login` |
| `ziti login forget` | `ziti edge login forget` |
| `ziti login use` | `ziti edge use` |

### Breaking Change: `ziti create ca`

`ziti create ca` now creates a Ziti edge Certificate Authority, matching the 
behavior of `ziti edge create ca`. Previously, `ziti create ca` was a PKI command 
for generating CA certificates on the local filesystem.

The PKI command is still available at its original location:

```
ziti pki create ca ...
```

Scripts that use `ziti create ca` for PKI operations should be updated to use 
`ziti pki create ca` instead.

## Multiple DNS Upstreams

The tunneler's DNS resolver now accepts multiple upstream DNS servers and fans out recursive queries
to all of them in parallel, rather than being limited to a single upstream. This is useful for split-horizon
setups where different resolvers are authoritative for different zones, and for environments where a primary
resolver may be slow or unreachable.

### CLI

The `ziti tunnel --dnsUpstream` flag is now a repeatable string slice. Upstreams can be listed by repeating
the flag or by passing a comma-separated value:

```bash
ziti tunnel run \
  --dnsUpstream udp://10.96.0.10:53 \
  --dnsUpstream tcp://8.8.8.8:53
```

### Router Config

In `xgress_edge_tunnel` router configs, `options.dnsUpstream` now accepts either a single string (as before)
or a list of strings. Existing configs continue to work unchanged:

```yaml
# single upstream (unchanged)
options:
  dnsUpstream: udp://10.96.0.10:53

# multiple upstreams
options:
  dnsUpstream:
    - udp://10.96.0.10:53
    - tcp://8.8.8.8:53
```

### How Resolution Works

When a query comes in, the resolver dispatches it to every configured upstream concurrently. The first
response with `RCODE=NOERROR` wins and is returned to the client immediately, so split-horizon lookups
work even if one upstream is slow. If no upstream returns NOERROR, the resolver picks the best-ranked
non-NOERROR reply (NXDOMAIN > SERVFAIL > REFUSED) so authoritative negative answers aren't masked by
transport failures. If every upstream fails to respond, the query is treated as unanswerable and handled
per the configured `dnsUnanswerable` disposition.

## Agent Inspect

The `ziti agent` CLI now has a generic `inspect` subcommand that works against any ziti process
(controller, router, or tunneler) over the local IPC agent channel. It sends the inspect request
directly to the target process, so unlike `ziti fabric inspect` it doesn't fan out through the
controller and doesn't require network connectivity to the target.

```
ziti agent inspect <value> [values...]
```

Values are matched against whatever the target process exposes. Common inspect keys:

* Routers: `stackdump`, `links`, `config`, `metrics`, `sdk-terminators`, `ert-terminators`,
  `router-circuits`, `router-data-model`, `router-controllers`
* Controllers: `stackdump`, `config`, `metrics`, `connected-routers`, `connected-peers`,
  `cluster-config`, `router-messaging`, `terminator-costs`, `data-model-index`
* Tunnelers: `stackdump`, `sdk`

## Current Beta Features

Beta features are still under development and are subject to change. They should
be usable in their released form. Though unlikely, there is a small chance they will 
be removed. 

* Basic Permission System
* Alert Events
* Controller-Initiated Control Channel Dials

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.31 -> v1.0.33](https://github.com/openziti/agent/compare/v1.0.31...v1.0.33)
* github.com/openziti/channel/v4: [v4.2.28 -> v4.3.11](https://github.com/openziti/channel/compare/v4.2.28...v4.3.11)
    * [Issue #242](https://github.com/openziti/channel/issues/242) - Reconnecting channel shouldn't allow changing ids
    * [Issue #235](https://github.com/openziti/channel/issues/235) - Bump allowed hello message headers size to 16k from 4k
    * [Issue #228](https://github.com/openziti/channel/issues/228) - Ensure that Underlay never return nil on MultiChannel
    * [Issue #226](https://github.com/openziti/channel/issues/226) - Allow specifying a minimum number of underlays for a channel, regardless of underlay type
    * [Issue #225](https://github.com/openziti/channel/issues/225) - Add ChannelCreated to the UnderlayHandler API to allow handlers to be initialized with the channel before binding
    * [Issue #224](https://github.com/openziti/channel/issues/224) - Update the underlay dispatcher to allow unknown underlay types to fall through to the default
    * [Issue #222](https://github.com/openziti/channel/issues/222) - Allow injecting the underlay type into messages

* github.com/openziti/edge-api: [v0.26.47 -> v0.28.1](https://github.com/openziti/edge-api/compare/v0.26.47...v0.28.1)
    * [Issue #175](https://github.com/openziti/edge-api/issues/175) - ctrlChanListeners should have x-omit-empty: false attribute
    * [Issue #170](https://github.com/openziti/edge-api/issues/170) - Add preferredLeader flag to controllers
    * [Issue #167](https://github.com/openziti/edge-api/issues/167) - Add ctrlChanListeners to router types
    * [Issue #164](https://github.com/openziti/edge-api/issues/164) - Add permissions list to identity

* github.com/openziti/foundation/v2: [v2.0.72 -> v2.0.90](https://github.com/openziti/foundation/compare/v2.0.72...v2.0.90)
    * [Issue #472](https://github.com/openziti/foundation/issues/472) - Add support for multi-bit set/get to AtomicBitSet
    * [Issue #464](https://github.com/openziti/foundation/issues/464) - Add support for -pre in versions
    * [Issue #455](https://github.com/openziti/foundation/issues/455) - Correctly close goroutine pool when external close is signaled
    * [Issue #452](https://github.com/openziti/foundation/issues/452) - Goroutine pool with a min worker count of 1 can drop to 0 workers due to race condition

* github.com/openziti/identity: [v1.0.111 -> v1.0.128](https://github.com/openziti/identity/compare/v1.0.111...v1.0.128)
    * [Issue #68](https://github.com/openziti/identity/issues/68) - Shutdown file watcher when stopping identity watcher

* github.com/openziti/metrics: [v1.4.2 -> v1.4.5](https://github.com/openziti/metrics/compare/v1.4.2...v1.4.5)
    * [Issue #58](https://github.com/openziti/metrics/issues/58) - Add GaugeFloat64 support
    * [Issue #56](https://github.com/openziti/metrics/issues/56) - underlying resources of reference counted meters are not cleaned up when reference count hits zero

* github.com/openziti/runzmd: [v1.0.80 -> v1.0.90](https://github.com/openziti/runzmd/compare/v1.0.80...v1.0.90)
* github.com/openziti/sdk-golang: [v1.2.3 -> v1.7.0](https://github.com/openziti/sdk-golang/compare/v1.2.3...v1.7.0)
    * [Issue #901](https://github.com/openziti/sdk-golang/issues/901) - Move xgress back to having retransmitter goroutine per-xgress
    * [Issue #906](https://github.com/openziti/sdk-golang/issues/906) - Fix potential nil references on session service structs
    * [Issue #897](https://github.com/openziti/sdk-golang/issues/897) - Allow xgress to use pull model for reads when appropriate
    * [Issue #895](https://github.com/openziti/sdk-golang/issues/895) - Limit effect sudden rtt spikes can have on rtt moving average
    * [Issue #902](https://github.com/openziti/sdk-golang/issues/902) - Inspect response message content types are mixed up
    * [Issue #887](https://github.com/openziti/sdk-golang/issues/887) - Fix listener manager cleanup
    * [Issue #886](https://github.com/openziti/sdk-golang/issues/886) - When controller is busy during service refresh, backoff and retry instead of falling back to full refresh
    * [Issue #885](https://github.com/openziti/sdk-golang/issues/885) - Only compare relevant service fields when looking for changes
    * [Issue #884](https://github.com/openziti/sdk-golang/issues/884) - Add deadline for bind establishment
    * [Issue #883](https://github.com/openziti/sdk-golang/issues/883) - Router level listener can be left open if multi-listener closes during listener establishment
    * [Issue #877](https://github.com/openziti/sdk-golang/issues/877) - Handle differences in xgress eof/end-of-circuit handling by adding a capabilities exchange
    * [Issue #832](https://github.com/openziti/sdk-golang/issues/832) - Fuzz session refresh timers
    * [Issue #879](https://github.com/openziti/sdk-golang/issues/879) - Return the connId in inspect response
    * [Issue #878](https://github.com/openziti/sdk-golang/issues/878) - Fix responses from rx goroutines
    * [Issue #874](https://github.com/openziti/sdk-golang/issues/874) - Add inspect support at the context level
    * [Issue #871](https://github.com/openziti/sdk-golang/issues/871) - Make SDK better at sticking to MaxTerminator terminators
    * [Issue #708](https://github.com/openziti/sdk-golang/issues/708) - Support for Go's built-in context in Dial methods
    * [Issue #860](https://github.com/openziti/sdk-golang/issues/860) - Make the dialing identity's id and name available on dialed connections
    * [Issue #857](https://github.com/openziti/sdk-golang/issues/857) - Use new error code and retry hints to correctly react to terminator errors
    * [Issue #847](https://github.com/openziti/sdk-golang/issues/847) - Ensure the initial version check succeeds, to ensure we don't legacy sessions on ha or oidc-enabled controllers
    * [Issue #824](https://github.com/openziti/sdk-golang/pull/824) - release notes and hard errors on no TOTP handler breaks partial auth events
    * [Issue #818](https://github.com/openziti/sdk-golang/issues/818) - Full re-auth should not clear services list, as that breaks the on-change logic
    * [Issue #817](https://github.com/openziti/sdk-golang/issues/817) - goroutines can get stuck when iterating over randomized HA controller list
    * [Issue #736](https://github.com/openziti/sdk-golang/issues/736) - Migrate from github.com/mailru/easyjson
    * [Issue #813](https://github.com/openziti/sdk-golang/issues/813) - SDK doesn't stop close listener when it detects that a service being hosted gets deleted
    * [Issue #811](https://github.com/openziti/sdk-golang/issues/811) - Credentials are lost when explicitly set
    * [Issue #807](https://github.com/openziti/sdk-golang/issues/807) - Don't send close from rxer to avoid blocking
    * [Issue #800](https://github.com/openziti/sdk-golang/issues/800) - Tidy create service session logging

* github.com/openziti/secretstream: [v0.1.39 -> v0.1.49](https://github.com/openziti/secretstream/compare/v0.1.39...v0.1.49)
* github.com/openziti/transport/v2: [v2.0.188 -> v2.0.215](https://github.com/openziti/transport/compare/v2.0.188...v2.0.215)
    * [Issue #31](https://github.com/openziti/transport/issues/31) - ipv6 Transport Address Parsing
    * [Issue #149](https://github.com/openziti/transport/issues/149) - Archive transwarp code

* github.com/openziti/xweb/v3: [v2.3.4 -> v3.0.4](https://github.com/openziti/xweb/compare/v2.3.4...v3.0.4)
    * [Issue #32](https://github.com/openziti/xweb/issues/32) - watched identities sometimes don't reload when changed

* github.com/openziti/go-term-markdown: v1.0.1 (new)
* github.com/openziti/ziti/v2: [v1.6.8 -> v2.0.0](https://github.com/openziti/ziti/compare/v1.6.8...v2.0.0)
    * [Issue #3830](https://github.com/openziti/ziti/issues/3830) - statemanager is holding on to edge connections and they're never getting cleared
    * [Issue #3824](https://github.com/openziti/ziti/issues/3824) - Allow calling inspect using the IPC agent on the controller, router and go tunnel
    * [Issue #2049](https://github.com/openziti/ziti/issues/2049) - The ziti agent command should have a controller connection status
    * [Issue #3784](https://github.com/openziti/ziti/issues/3784) - Fix link registry race condition on reporting links on reconnect
    * [Issue #3734](https://github.com/openziti/ziti/issues/3734) - Enforce client certificate proof-of-possession on controller REST API for OIDC sessions
    * [Issue #3806](https://github.com/openziti/ziti/issues/3806) - Expose OpenZiti-specific login and MFA endpoints in the OIDC discovery document
    * [Issue #3818](https://github.com/openziti/ziti/issues/3818) - Filtering policies by keywords `Dial` and `Bind` doesn't work
    * [Issue #3816](https://github.com/openziti/ziti/issues/3816) - Support multiple upstream DNS providers in ziti tunnel and ER/T
    * [Issue #3699](https://github.com/openziti/ziti/issues/3699) - Consolidate CLI edge and fabric commands in top level create/update/delete/list/login commands
    * [Issue #3788](https://github.com/openziti/ziti/issues/3788) - OIDC Endpoints return 400 Bad Request instead of underlying error
    * [Issue #3680](https://github.com/openziti/ziti/issues/3680) - adds revocation control to CLI and Management API
    * [Issue #3717](https://github.com/openziti/ziti/issues/3717) - Generic error message for specific error
    * [Issue #3543](https://github.com/openziti/ziti/issues/3543) - New Circuit Failure code for sockets not available
    * [Issue #3364](https://github.com/openziti/ziti/issues/3364) - Make no such host error specific
    * [Issue #2888](https://github.com/openziti/ziti/issues/2888) - New specific Error code for port not allowed
    * [Issue #2859](https://github.com/openziti/ziti/issues/2859) - Create specific error code for DNS failed resolution
    * [Issue #1580](https://github.com/openziti/ziti/issues/1580) - Invalid link destination should have a specific error code
    * [Issue #3706](https://github.com/openziti/ziti/issues/3706) - Increase link payload/ack queue sizes and make them configurable
    * [Issue #3778](https://github.com/openziti/ziti/issues/3778) - SetRouterDataModel can deadlock in the router
    * [Issue #3777](https://github.com/openziti/ziti/issues/3777) - With the new circuit reserve, we can have circuits with no path in the controller circuit set, which can cause panics
    * [Issue #3770](https://github.com/openziti/ziti/issues/3770) - Update Token Requests Should Close Channel Connections If Invalid
    * [Issue #3762](https://github.com/openziti/ziti/issues/3762) - Revocations not included in full router data model state
    * [Issue #3757](https://github.com/openziti/ziti/issues/3757) - Mesh peer signing cert from header is overwritten by TLS underlay cert
    * [Issue #3756](https://github.com/openziti/ziti/issues/3756) - TLS handshake rate limiter timeout check reads from wrong config scope
    * [Issue #3755](https://github.com/openziti/ziti/issues/3755) - commandHandler config read from wrong scope
    * [Issue #3754](https://github.com/openziti/ziti/issues/3754) - dialFailed drops applyFailed parameter, preventing duplicate link retry jitter
    * [Issue #3753](https://github.com/openziti/ziti/issues/3753) - SPIFFE trust domain prefix check has swapped HasPrefix arguments
    * [Issue #3746](https://github.com/openziti/ziti/issues/3746) - The controller connect events control channel handler leaks a goroutine
    * [Issue #3747](https://github.com/openziti/ziti/issues/3747) - Update controller peer error marshalling for app code changes
    * [Issue #3721](https://github.com/openziti/ziti/issues/3721) - Add CreateCircuitV3 to controller
    * [Issue #3719](https://github.com/openziti/ziti/issues/3719) - Allow binding specific inspects to pass through to xgress listener implementations
    * [Issue #3696](https://github.com/openziti/ziti/issues/3696) - oidc provider is non-deterministic for wildcard certs
    * [Issue #3681](https://github.com/openziti/ziti/issues/3681) - coalesce OIDC JWT revocations to reduce controller write pressure
    * [Issue #3683](https://github.com/openziti/ziti/issues/3683) - add fablab test for testing flow control changes over a longer term
    * [Issue #3673](https://github.com/openziti/ziti/issues/3673) - revocation build-up in db and rdm
    * [Issue #3674](https://github.com/openziti/ziti/issues/3674) - Update to Go 1.26
    * [Issue #3496](https://github.com/openziti/ziti/issues/3496) - MFA TOTP Enrollment During OIDC Authentication Does Not Work
    * [Issue #3609](https://github.com/openziti/ziti/issues/3609) - Stabilize terminator creation test for 2.0
    * [Issue #3648](https://github.com/openziti/ziti/issues/3648) - tunnel: myCopy logs router ID as circuitId, causing misleading debug output
    * [Issue #3658](https://github.com/openziti/ziti/issues/3658) - Raft cluster join fails with "hello message too big" when using long hostnames
    * [Issue #3626](https://github.com/openziti/ziti/issues/3626) - controller: overlay bind point produces malformed URL in /versions apiBaseUrls
    * [Issue #3635](https://github.com/openziti/ziti/issues/3635) - Allow controllers to dial routers to support more topologies
    * [Issue #3607](https://github.com/openziti/ziti/issues/3607) - linux installer not upgradable from v1
    * [Issue #3650](https://github.com/openziti/ziti/issues/3650) - Reroute doesn't proactively clean up orphaned route entries
    * [Issue #3571](https://github.com/openziti/ziti/issues/3571) - Ensure 2.0 backwards compatibility with 1.6 and 1.5 using the smoketest
    * [Issue #3636](https://github.com/openziti/ziti/issues/3636) - Adaptive rate limiter should use success rate rather than queue position
    * [Issue #3600](https://github.com/openziti/ziti/issues/3600) - Add a preferredLeader flag, allow selected nodes to be preferred for raft leader, if they're available
    * [Issue #3642](https://github.com/openziti/ziti/issues/3642) - Use xgress_common.Connection type for xgress_transport and xgress_proxy
    * [Issue #3597](https://github.com/openziti/ziti/issues/3597) - Enable OIDC API by default
    * [Issue #3624](https://github.com/openziti/ziti/issues/3624) - Multi-underlay control channel doesn't correctly handle lack of group secret on non-grouped underlays
    * [Issue #3613](https://github.com/openziti/ziti/issues/3613) - Initializing cluster from existing db using `db:` config settings results in panic
    * [Issue #3620](https://github.com/openziti/ziti/issues/3620) - Controller control channel heartbeats config parsing was erroneously nested under options parsing
    * [Issue #3619](https://github.com/openziti/ziti/issues/3619) - Router connect events full sync interval was using min/max values meant for the batch interval
    * [Issue #3618](https://github.com/openziti/ziti/issues/3618) - Router interfaceDiscovery.minReportInterval value was being set to checkInterval
    * [Issue #3617](https://github.com/openziti/ziti/issues/3617) - Transit router disabled flag not passed through raft command structure
    * [Issue #3333](https://github.com/openziti/ziti/issues/3333) - Updb user-lockout triggered by successful login attempts
    * [Issue #3599](https://github.com/openziti/ziti/issues/3599) - Add gap detection and handling to router data model
    * [Issue #3356](https://github.com/openziti/ziti/issues/3356) - Add WWW-Authenticate Headers
    * [Issue #3074](https://github.com/openziti/ziti/issues/3074) - Dynamic cost range is too limited
    * [Issue #3558](https://github.com/openziti/ziti/issues/3558) - terminator cost increased on egress dial success, not on circuit completion
    * [Issue #3556](https://github.com/openziti/ziti/issues/3556) - global circuit costs not cleared when terminator is deleted
    * [Issue #3557](https://github.com/openziti/ziti/issues/3557) - costing calculation for the weighted terminator selection strategy  is incorrect
    * [Issue #2512](https://github.com/openziti/ziti/issues/2512) - Return circuit ID and error in dial failures
    * [Issue #3569](https://github.com/openziti/ziti/issues/3569) - Version 2.0+ routers should not connect to controllers which do not support JWT formatted legacy sessions
    * [Issue #3565](https://github.com/openziti/ziti/issues/3565) - Link dialer save 'is first conn' true, so all dials claim to be first, causing potential race condition
    * [Issue #3575](https://github.com/openziti/ziti/issues/3575) - OIDC token endpoint code bugs possibly resulting in panics/eof errors
    * [Issue #3550](https://github.com/openziti/ziti/issues/3550) - Support multi-underlay control channels
    * [Issue #3535](https://github.com/openziti/ziti/issues/3535) - Remove the legacy xgress_edge_tunnel implementation
    * [Issue #3547](https://github.com/openziti/ziti/issues/3547) - Add support for sending the dialing identity id and name to the hosting sdk
    * [Issue #3541](https://github.com/openziti/ziti/issues/3541) - Remove option to disable the router data model in the controller
    * [Issue #3540](https://github.com/openziti/ziti/issues/3540) - Handle UDP difference between proxy and tproxy implementations
    * [Issue #3527](https://github.com/openziti/ziti/issues/3527) - ER/T UDP tunnels keep closed connections for 30s, preventing potential new good connections in that time
    * [Issue #3526](https://github.com/openziti/ziti/issues/3526) - ER/T half-close logic is incorrect
    * [Issue #3524](https://github.com/openziti/ziti/issues/3524) - Provide more error context to SDKs for terminator errors
    * [Issue #3509](https://github.com/openziti/ziti/issues/3509) - Enforce policy on the router for oidc sessions, by closing open circuits and terminators when service access is lost
    * [Issue #3531](https://github.com/openziti/ziti/issues/3531) - Remove created/updated/deleted terminator events. Obsoleted by entity change events.
    * [Issue #3532](https://github.com/openziti/ziti/issues/3532) - Removed deprecated create identity <type> subcommands
    * [Issue #3521](https://github.com/openziti/ziti/issues/3521) - Cleanup CLI to remove calls to os.Exit to be embed friendlier
    * [Issue #3516](https://github.com/openziti/ziti/issues/3516) - Remove support for create terminator v1
    * [Issue #3512](https://github.com/openziti/ziti/issues/3512) - Remove legacy link management code from the controller
    * [Issue #3511](https://github.com/openziti/ziti/issues/3511) - router proxy mode fails to resolve interface if binding is 0.0.0.0
    * [Issue #3503](https://github.com/openziti/ziti/issues/3503) - Allow routers to request current cluster membership information
    * [Issue #3501](https://github.com/openziti/ziti/issues/3501) - Get cluster membership information from raft directly, rather than trying to cache it in the DB
    * [Issue #3500](https://github.com/openziti/ziti/issues/3500) - Set a router data model timeline when initializing a new HA setup, rather than letting it stay blank
    * [Issue #3504](https://github.com/openziti/ziti/issues/3504) - Reduce router data model full state updates
    * [Issue #3492](https://github.com/openziti/ziti/pull/3492) - Bump openziti/ziti-console-assets from 3.12.9 to 4.0.0 in /dist/docker-images/ziti-controller in the all group
    * [Issue #3484](https://github.com/openziti/ziti/issues/3484) - router ctrl channel handler for handling cluster changes has an initialization race condition
    * [Issue #3477](https://github.com/openziti/ziti/issues/3477) - Optionally enable model changes triggered by login to be non-blocking and to be droppable if the system is under load
    * [Issue #3473](https://github.com/openziti/ziti/issues/3473) - Enable tls handshake rate limiter by default and tweak default values.
    * [Issue #3471](https://github.com/openziti/ziti/issues/3471) - Go tunneler is ignoring host config MaxConnections
    * [Issue #3469](https://github.com/openziti/ziti/issues/3469) - Only send model updates on resubscribe if the RDM index has advanced
    * [Issue #2573](https://github.com/openziti/ziti/issues/2573) - An edge router in a tight restart loop causes a resource leak on routers to which it connects.
    * [Issue #3430](https://github.com/openziti/ziti/issues/3430) - Add permissions list to identity
    * [Issue #2109](https://github.com/openziti/ziti/issues/2109) - Add Edge Management Read Only Capability
    * [Issue #3435](https://github.com/openziti/ziti/issues/3435) - Add edge management API permissions by entity type and action
    * [Issue #3441](https://github.com/openziti/ziti/issues/3441) - Update router connection tracker to interrogate active connections
    * [Issue #3451](https://github.com/openziti/ziti/issues/3451) - ci - compare only stable releases when promoting
    * [Issue #3437](https://github.com/openziti/ziti/issues/3437) - SDK OIDC token updates to routers should return an error if invalid
    * [Issue #3348](https://github.com/openziti/ziti/issues/3348) - Unable to clear/reset the "tags" property on an entity to an empty object
    * [Issue #3452](https://github.com/openziti/ziti/issues/3452) - `ziti agent cluster add` has bad behavior if the add address doesn't match the advertise address
    * [Issue #3410](https://github.com/openziti/ziti/issues/3410) - Consolidate fabric REST API code with edge management and edge client code
    * [Issue #3425](https://github.com/openziti/ziti/issues/3425) - RDM not properly responding to tunneler enabled flag changes
    * [Issue #3420](https://github.com/openziti/ziti/issues/3420) - The terminator id cache uses the same id for all terminators in a host.v2 config, resulting in a single terminator
    * [Issue #3419](https://github.com/openziti/ziti/issues/3419) - When using the router data model, precedence specified on the per-service identity mapping are incorrectly interpreted
    * [Issue #3318](https://github.com/openziti/ziti/issues/3318) - Terminator creation seems to slow exponentially as the number of terminators rises from 10k to 20k to 40k
    * [Issue #3407](https://github.com/openziti/ziti/issues/3407) - The CLI doesn't properly pass JWT authentication information to websocket endpoints
    * [Issue #3359](https://github.com/openziti/ziti/issues/3359) - Ensure router data model subscriptions have reasonable performance and will scale
    * [Issue #3354](https://github.com/openziti/ziti/issues/3354) - SDK/ENV Info from SDKs is not distributed in HA
    * [Issue #3381](https://github.com/openziti/ziti/issues/3381) - the fabric service REST apis are missing the maxIdleTime property
    * [Issue #3382](https://github.com/openziti/ziti/issues/3382) - Legacy service sessions generated pre-1.7.x are incompatible with v1.7.+ and need to be cleared
    * [Issue #3339](https://github.com/openziti/ziti/issues/3339) - get router ctrl.endpoint from ctrls claim in JWT
    * [Issue #3378](https://github.com/openziti/ziti/issues/3378) - login with file stopped working
    * [Issue #3349](https://github.com/openziti/ziti/issues/3349) - UPDB OIDC login returns wrong content type
    * [Issue #2324](https://github.com/openziti/ziti/issues/2324) - Add Ext-JWT/OIDC enrollment
    * [Issue #3346](https://github.com/openziti/ziti/issues/3346) - Fix confusing attempt logging
    * [Issue #3337](https://github.com/openziti/ziti/issues/3337) - Router reports "no xgress edge forwarder for circuit"
    * [Issue #3345](https://github.com/openziti/ziti/issues/3345) - Clean up connect events tests and remove global XG registry
    * [Issue #3264](https://github.com/openziti/ziti/issues/3264) - Allow routers to generate alert events in cases of service misconfiguration
    * [Issue #3321](https://github.com/openziti/ziti/issues/3321) - Health Check API missing base path on discovery endpoint
    * [Issue #3323](https://github.com/openziti/ziti/issues/3323) - router/tunnel static services fail to bind unless new param protocol is defined
    * [Issue #3309](https://github.com/openziti/ziti/issues/3309) - Detect link connections meant for another router
    * [Issue #3286](https://github.com/openziti/ziti/issues/3286) - edge-api binding doesn't have the correct path on discovery endpoints
    * [Issue #3297](https://github.com/openziti/ziti/issues/3297) - stop promoting hotfixes downstream
    * [Issue #3295](https://github.com/openziti/ziti/issues/3295) - make ziti tunnel service:port pairs optional
    * [Issue #3291](https://github.com/openziti/ziti/issues/3291) - replace decommissioned bitnami/kubectl
    * [Issue #3277](https://github.com/openziti/ziti/issues/3277) - Router can deadlock on closing a connection if the incoming data channel is full
    * [Issue #3269](https://github.com/openziti/ziti/issues/3269) - Add host-interfaces config type
    * [Issue #3258](https://github.com/openziti/ziti/issues/3258) - Add config type proxy.v1 so proxies can be defined dynamically for the ER/T
    * [Issue #3259](https://github.com/openziti/ziti/issues/3259) - Interfaces config type not added due to wrong name
    * [Issue #3265](https://github.com/openziti/ziti/issues/3265) - Forwarding errors should log at debug, since they are usual part of circuit teardown
    * [Issue #3261](https://github.com/openziti/ziti/issues/3261) - ER/T dialed xgress connections may only half-close when peer is fully closed


