# Release 1.0.1

## What's New

* Deployments Alpha
    * Linux packages provide systemd services for controller and router. Both depend on existing package `openziti` which provides the `ziti` command line tool.
        * `openziti-controller` provides `ziti-controller.service`
        * `openziti-router` provides `ziti-router.service`
    * Container images for controller and router now share the bootstrapping logic with the packages, so they
      support the same configuration options.

# Release 1.0.0

## About 1.0

What does marking OpenZiti as 1.0 mean?

### Backwards Compatibility
We've guaranteed API stability for SDK clients for years and worked hard to ensure that routers 
and controllers would be backwards and forward compatible. However, we have had a variety of 
management API changes and CLI changes. For post 1.0 releases we expect to make additions to the 
APIs and CLI, but won't remove anything until it's been first marked as deprecated and then only
with a major version bump. 

### Stability and Scale
Recent releases have seen additional testing using chaos testing techniques. These tests involve
setting up relatively large scale environments, knocking out various components and then verifying
that the network is able to return to a stable state. These test are run for hours to try and 
eliminate race conditions and distributed state machine problems. 

OpenZiti is also being used as underlying infrastrcture for the zrok public service. Use of this 
network has grown quickly and proven that it's possible to build ziti native apps that can scale
up.

## Backward Incompatible Changes to pre-1.0 releases

Administrators no longer have access to dial/bind all services by default. See below for details.

## What's New

* Administrators no longer have access to dial/bind all services by default.
* TLS Handshakes can now be rate limited in the controller
* TLS Handshake timeouts can now be set on the controller when using ALPN
* Bugfixes

## DEFAULT Bind/Dial SERVICE PERMISSIONS FOR Admin IDENTITIES HAVE CHANGED

Admin identities were able to Dial and Bind all services regardless of the effective service policies
prior to this release. This could lead to a confusing situation where a tunneler that was assuming an Admin
identity would put itself into an infinite connect-loop when a service's host.v1 address overlapped with
any addresses in its intercept configuration.

Please create service policies to grant Bind or Dial permissions to Admin identities as needed.

## TLS Handshake

A TLS handhshake rate limiter can be enabled. This is useful in cases where there's a flood of TLS requests and the
controller can't handle them all. It can get into a state where it can't respond to TLS handshakes quickly enough,
so the clients time out. They then retry, adding to the the load. The controller ends up wasting time doing work 
that isn't use. 

This uses the same rate limiting as the auth rate limiter. 

Additionally the server side handshake timeout can now be configured.

Configuration:

```
tls: 
  handshakeTimeout: 15s

  rateLimiter:
    # if disabled, no tls handshake rate limiting with be enforced
    enabled: true
    # the smallest window size for tls handshakes
    minSize: 5
    # the largest allowed window size for tls handshakes
    maxSize: 5000
    # after how long to consider a handshake abandoned if neither success nor failure was reported
    timeout: 30s
```

New metrics:

* `tls_handshake_limiter.in_process` - number of TLS handshakes in progress
* `tls_handshake_limiter.window_size` - number of TLS handhshakes allowed concurrently
* `tls_handshake_limiter.work_timer` - timer tracking how long TLS handshakes are taking


## Component Updates and Bug Fixes

* github.com/openziti/channel/v2: [v2.0.122 -> v2.0.128](https://github.com/openziti/channel/compare/v2.0.122...v2.0.128)
* github.com/openziti/edge-api: [v0.26.14 -> v0.26.16](https://github.com/openziti/edge-api/compare/v0.26.14...v0.26.16)
* github.com/openziti/foundation/v2: [v2.0.40 -> v2.0.42](https://github.com/openziti/foundation/compare/v2.0.40...v2.0.42)
* github.com/openziti/identity: [v1.0.73 -> v1.0.75](https://github.com/openziti/identity/compare/v1.0.73...v1.0.75)
* github.com/openziti/metrics: [v1.2.48 -> v1.2.51](https://github.com/openziti/metrics/compare/v1.2.48...v1.2.51)
* github.com/openziti/runzmd: [v1.0.41 -> v1.0.43](https://github.com/openziti/runzmd/compare/v1.0.41...v1.0.43)
* github.com/openziti/sdk-golang: [v0.23.15 -> v0.23.19](https://github.com/openziti/sdk-golang/compare/v0.23.15...v0.23.19)
* github.com/openziti/secretstream: [v0.1.18 -> v0.1.19](https://github.com/openziti/secretstream/compare/v0.1.18...v0.1.19)
* github.com/openziti/storage: [v0.2.33 -> v0.2.36](https://github.com/openziti/storage/compare/v0.2.33...v0.2.36)
* github.com/openziti/transport/v2: [v2.0.125 -> v2.0.131](https://github.com/openziti/transport/compare/v2.0.125...v2.0.131)
    * [Issue #79](https://github.com/openziti/transport/issues/79) - Add adaptive rate limiting to shared tls listener

* github.com/openziti/ziti: [v0.34.2 -> v1.0.0](https://github.com/openziti/ziti/compare/v0.34.2...v1.0.0)
    * [Issue #1923](https://github.com/openziti/ziti/issues/1923) - Add release validation test suite
    * [Issue #1904](https://github.com/openziti/ziti/issues/1904) - Add TLS handshake rate limiter
    * [Issue #1921](https://github.com/openziti/ziti/issues/1921) - Tidy CLI
    * [Issue #1916](https://github.com/openziti/ziti/issues/1916) - SDK dials fails with 'token is malformed' error
    * [Issue #1911](https://github.com/openziti/ziti/issues/1911) - Fix panic on first HA controller startup
    * [Issue #1914](https://github.com/openziti/ziti/issues/1914) - Fix panic in PeerConnected
    * [Issue #1781](https://github.com/openziti/ziti/issues/1781) - Admin identities have bind and dial permissions to services
