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

### New Permissions Model (BETA)

As one feature goes out of beta, another arrives into beta. This release introduces a new permissions system
for more fine grained control to the management API. It's not expected to change, but may do so based on feedback
from users.

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
* Controller Edge APIs now return `WWW-Authenticate` response headers on `401 Unauthorized` responses, giving clients actionable information about which auth methods are accepted and what went wrong


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

1. Raft Apply Timeout (raft.applyTimeout)

Location: Controller configuration file, under raft section
Type: Duration
Default: 5s
Description: Timeout for applying commands to the Raft distributed log. Commands that exceed this timeout will trigger adaptive rate limiter backoff.

Example:
```
  raft:
    applyTimeout: 10s
```

2. Raft Rate Limiter Configuration (raft.rateLimiter)

A new adaptive rate limiter that controls the submission of commands to the Raft cluster. Unlike the existing command rate limiter, this specifically manages in-flight Raft operations with adaptive window sizing.

Configuration Structure:
```
  raft:
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

3. Restart Self on Snapshot (raft.restartSelfOnSnapshot)

Location: Controller configuration file, under raft section
Type: Boolean
Default: false
Description: When true, the controller will automatically restart itself when restoring a snapshot to an initialized system. When false, the controller will exit with code 0, requiring external process management to restart it.

Example:
```
  raft:
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

## Component Updates and Bug Fixes

* github.com/openziti/channel/v4: [v4.2.41 -> v4.2.50](https://github.com/openziti/channel/compare/v4.2.41...v4.2.50)
* github.com/openziti/edge-api: [v0.26.50 -> v0.26.52](https://github.com/openziti/edge-api/compare/v0.26.50...v0.26.52)
    * [Issue #164](https://github.com/openziti/edge-api/issues/164) - Add permissions list to identity

* github.com/openziti/foundation/v2: [v2.0.79 -> v2.0.84](https://github.com/openziti/foundation/compare/v2.0.79...v2.0.84)
    * [Issue #464](https://github.com/openziti/foundation/issues/464) - Add support for -pre in versions

* github.com/openziti/identity: [v1.0.118 -> v1.0.122](https://github.com/openziti/identity/compare/v1.0.118...v1.0.122)
* github.com/openziti/metrics: [v1.4.2 -> v1.4.3](https://github.com/openziti/metrics/compare/v1.4.2...v1.4.3)
    * [Issue #56](https://github.com/openziti/metrics/issues/56) - underlying resources of reference counted meters are not cleaned up when reference count hits zero

* github.com/openziti/runzmd: [v1.0.84 -> v1.0.86](https://github.com/openziti/runzmd/compare/v1.0.84...v1.0.86)
* github.com/openziti/sdk-golang: [v1.2.10 -> v1.3.1](https://github.com/openziti/sdk-golang/compare/v1.2.10...v1.3.1)
    * [Issue #824](https://github.com/openziti/sdk-golang/pull/824) - release notes and hard errors on no TOTP handler breaks partial auth events

* github.com/openziti/secretstream: [v0.1.41 -> v0.1.46](https://github.com/openziti/secretstream/compare/v0.1.41...v0.1.46)
* github.com/openziti/storage: [v0.4.31 -> v0.4.35](https://github.com/openziti/storage/compare/v0.4.31...v0.4.35)
    * [Issue #122](https://github.com/openziti/storage/issues/122) - StringFuncNode has incorrect nil check, allowing panic
    * [Issue #120](https://github.com/openziti/storage/issues/120) - Change post tx commit constraint handling order
    * [Issue #119](https://github.com/openziti/storage/issues/119) - Add ContextDecorator API

* github.com/openziti/transport/v2: [v2.0.198 -> v2.0.205](https://github.com/openziti/transport/compare/v2.0.198...v2.0.205)
* github.com/openziti/xweb/v3: [v2.3.4 -> v3.0.3](https://github.com/openziti/xweb/compare/v2.3.4...v3.0.3)
    * [Issue #32](https://github.com/openziti/xweb/issues/32) - watched identities sometimes don't reload when changed

* github.com/openziti/ziti: [v1.7.0 -> v1.8.0](https://github.com/openziti/ziti/compare/v1.7.0...v1.8.0)
    * [Issue #3509](https://github.com/openziti/ziti/issues/3509) - Enforce policy on the router for oidc sessions, by closing open circuits and terminators when service access is lost
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
    * [Issue #3381](https://github.com/openziti/ziti/issues/3381) - the fabric service REST apis are missing the maxIdleTime property
    * [Issue #3382](https://github.com/openziti/ziti/issues/3382) - Legacy service sessions generated pre-1.7.x are incompatible with v1.7.+ and need to be cleared
    * [Issue #3339](https://github.com/openziti/ziti/issues/3339) - get router ctrl.endpoint from ctrls claim in JWT
    * [Issue #3378](https://github.com/openziti/ziti/issues/3378) - login with file stopped working
    * [Issue #3346](https://github.com/openziti/ziti/issues/3346) - Fix confusing attempt logging
    * [Issue #3337](https://github.com/openziti/ziti/issues/3337) - Router reports "no xgress edge forwarder for circuit"
    * [Issue #3345](https://github.com/openziti/ziti/issues/3345) - Clean up connect events tests and remove global XG registry
    * [Issue #3264](https://github.com/openziti/ziti/issues/3264) - Allow routers to generate alert events in cases of service misconfiguration
