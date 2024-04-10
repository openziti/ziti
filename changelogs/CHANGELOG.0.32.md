# Release 0.32.2

## What's New

* Terminator performance improvements
* API Rate Limiter enabled by default

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.117 -> v2.0.119](https://github.com/openziti/channel/compare/v2.0.117...v2.0.119)
    * [Issue #127](https://github.com/openziti/channel/issues/127) - Support some additional types in message headers

* github.com/openziti/foundation/v2: [v2.0.36 -> v2.0.37](https://github.com/openziti/foundation/compare/v2.0.36...v2.0.37)
* github.com/openziti/identity: [v1.0.69 -> v1.0.70](https://github.com/openziti/identity/compare/v1.0.69...v1.0.70)
* github.com/openziti/metrics: [v1.2.43 -> v1.2.45](https://github.com/openziti/metrics/compare/v1.2.43...v1.2.45)
* github.com/openziti/runzmd: [v1.0.37 -> v1.0.38](https://github.com/openziti/runzmd/compare/v1.0.37...v1.0.38)
* github.com/openziti/sdk-golang: [v0.22.21 -> v0.22.28](https://github.com/openziti/sdk-golang/compare/v0.22.21...v0.22.28)
    * [Issue #495](https://github.com/openziti/sdk-golang/issues/495) - Refresh edge session if listen attempt fails, to ensure that session is still valid

* github.com/openziti/storage: [v0.2.28 -> v0.2.30](https://github.com/openziti/storage/compare/v0.2.28...v0.2.30)
* github.com/openziti/transport/v2: [v2.0.121 -> v2.0.122](https://github.com/openziti/transport/compare/v2.0.121...v2.0.122)
* github.com/openziti/ziti: [v0.32.1 -> v0.32.2](https://github.com/openziti/ziti/compare/v0.32.1...v0.32.2)
    * [Issue #1741](https://github.com/openziti/ziti/issues/1741) - Prevent stuck links
    * [Issue #1736](https://github.com/openziti/ziti/issues/1736) - controller crashes generating create circuit responses
    * [Issue #1733](https://github.com/openziti/ziti/issues/1733) - Improve terminator creation performance
    * [Issue #1734](https://github.com/openziti/ziti/issues/1734) - Make API rate limiter enabled by default
    * [Issue #1726](https://github.com/openziti/ziti/issues/1726) - Fix some sdk hosting logging
    * [Issue #1725](https://github.com/openziti/ziti/issues/1725) - Fix panic in entity event processing
    * [Issue #652](https://github.com/openziti/ziti/issues/652) - CI support for MacOS arm64

# Release 0.32.1

## What's New

* Bugfixes
* New router setting to control startup timeout

## Router startup timeout

The router now has a configuration setting to control how long it wait on startup to be able to 
connect to a controller, before it gives up and exits.

```
ctrl:
  endpoints: 
    - tls:localhost:1280
  startupTimeout: 5m 
```

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.116 -> v2.0.117](https://github.com/openziti/channel/compare/v2.0.116...v2.0.117)
    * [Issue #125](https://github.com/openziti/channel/issues/125) - Ensure reconnecting channel is marked as connected before calling reconnect callback

* github.com/openziti/edge-api: [v0.26.8 -> v0.26.10](https://github.com/openziti/edge-api/compare/v0.26.8...v0.26.10)
* github.com/openziti/sdk-golang: [v0.22.17 -> v0.22.21](https://github.com/openziti/sdk-golang/compare/v0.22.17...v0.22.21)
* github.com/openziti/ziti: [v0.32.0 -> v0.32.1](https://github.com/openziti/ziti/compare/v0.32.0...v0.32.1)
    * [Issue #1709](https://github.com/openziti/ziti/issues/1709) - Fix link management race conditions found by chaos testing
    * [Issue #1715](https://github.com/openziti/ziti/issues/1715) - Ensure controller raft peers don't end up with duplicate connections 
    * [Issue #1702](https://github.com/openziti/ziti/issues/1702) - Add link management chaos test
    * [Issue #1691](https://github.com/openziti/ziti/issues/1691) multiple er re-enrolls creates multiple enrollments

# Release 0.32.0

## What's New

* Auth Rate Limiter
* Link Management Fixes
* ziti edge quickstart command deprecates redundant --already-initialized flag. The identical behavior is implied by --home.

## Backwards compatibility

This release includes new response types from the REST authentication APIS. They are now able to return 
`429` (server too busy) responses to auth requests. As this is an API change, the version number is 
being bumped to 0.32.

If controller and router are both v0.32 or later, only the router which dialed a link will report it to the controller. 
If the controller is older, newer routers will report links from both the dialing and listening side of the link.

## Auth Rate Limiter

In order to prevent clients from overwhelming the server with auth requests, an auth rate limiter has been introduced.
The rate limiter is adaptive, in that it will react to auth attempts timing out by shrinking the number of allowed
queued auth attempts. The number will slowly recover over time.

Example configuration:

```
edge:
  # This section allows configurating the rate limiter for auth attempts
  authRateLimiter:
    # if disabled, no auth rate limiting with be enforced
    enabled: true
    # the smallest window size for auth attempts
    minSize: 5
    # the largest allowed window size for auth attempts
    maxSize: 250
```

New metrics:

* `auth.limiter.queued_count` - current number of queued auth attempts
* `auth.limiter.window_size`  - current size at which new auth attempts will be rejected
* `auth.limiter.work_timer`   - tracks the rate at which api sessions are being created and how long it's taking to create them

## Link Management Fixes

With long lived link ids, there was potential for link control message to be ambiguous, as the link id wasn't enough to identify
a specific iteration of that link. An iteration field has been added to links so that messaging is unambiguous. 
Links will also only be reported from the dialing router now to reduce ambiguouity and race condition in link control channel
messaging.

## Router SSL Handshake Timeout Config

There is a new router config setting which allows setting the SSL handshake timeout for TLS connections, when using ALPN for listeners.

```
tls:
  handshakeTimeout: 15s
```

## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.111 -> v2.0.116](https://github.com/openziti/channel/compare/v2.0.111...v2.0.116)
    * [Issue #123](https://github.com/openziti/channel/issues/123) - Ensure hello messages respect connect timeout
    * [Issue #120](https://github.com/openziti/channel/issues/120) - Allow handling new underlay instances with function instead of channel 

* github.com/openziti/edge-api: [v0.26.6 -> v0.26.8](https://github.com/openziti/edge-api/compare/v0.26.6...v0.26.8)
* github.com/openziti/foundation/v2: [v2.0.35 -> v2.0.36](https://github.com/openziti/foundation/compare/v2.0.35...v2.0.36)
    * [Issue #391](https://github.com/openziti/foundation/issues/391) - goroutine pool can stall if configured for 0 min workers and with single producer

* github.com/openziti/identity: [v1.0.68 -> v1.0.69](https://github.com/openziti/identity/compare/v1.0.68...v1.0.69)
* github.com/openziti/metrics: [v1.2.41 -> v1.2.43](https://github.com/openziti/metrics/compare/v1.2.41...v1.2.43)
* github.com/openziti/runzmd: [v1.0.36 -> v1.0.37](https://github.com/openziti/runzmd/compare/v1.0.36...v1.0.37)
* github.com/openziti/sdk-golang: [v0.22.0 -> v0.22.17](https://github.com/openziti/sdk-golang/compare/v0.22.0...v0.22.17)
    * [Issue #482](https://github.com/openziti/sdk-golang/issues/482) - Deprecate ListenOptions.MaxConnections in favor of MaxTerminators

* github.com/openziti/secretstream: [v0.1.14 -> v0.1.16](https://github.com/openziti/secretstream/compare/v0.1.14...v0.1.16)
* github.com/openziti/storage: [v0.2.27 -> v0.2.28](https://github.com/openziti/storage/compare/v0.2.27...v0.2.28)
* github.com/openziti/transport/v2: [v2.0.119 -> v2.0.121](https://github.com/openziti/transport/compare/v2.0.119...v2.0.121)
    * [Issue #73](https://github.com/openziti/transport/issues/73) - Allow overriding shared TLS/ALPN listener SSL handshake timeout

* github.com/openziti/ziti: [v0.31.4 -> v0.32.0](https://github.com/openziti/ziti/compare/v0.31.4...v0.32.0)
    * [Issue #1692](https://github.com/openziti/ziti/issues/1692) - Improve link stability with long lived link ids
    * [Issue #1693](https://github.com/openziti/ziti/issues/1693) - Make links owned by the dialing router
    * [Issue #1685](https://github.com/openziti/ziti/issues/1685) - Race condition where we try to create terminator after client connection is closed
    * [Issue #1678](https://github.com/openziti/ziti/issues/1678) - Add link validation utility
    * [Issue #1673](https://github.com/openziti/ziti/issues/1673) - xgress dialers not getting passed xgress config
    * [Issue #1669](https://github.com/openziti/ziti/issues/1669) - Make sure link accepts are not single threaded
    * [Issue #1657](https://github.com/openziti/ziti/issues/1657) - Add api session rate limiter
