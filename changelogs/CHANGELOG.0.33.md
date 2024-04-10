# Release 0.33.1

## What's New

* Backward compatibility router <-> controller fix to address metrics parsing panic

## Component Updates and Bug Fixes
* github.com/openziti/ziti: [v0.33.0 -> v0.33.1](https://github.com/openziti/ziti/compare/v0.33.0...v0.33.1)
    * [Issue #1826](https://github.com/openziti/ziti/issues/1826) - 0.33.+ routers can cause panic in pre-0.33 controller with new metric

# Release 0.33.0

## What's New

* SDK Terminator stability improvements
* Minor feature updates and bug fixes

## SDK Terminator stability improvements

This release was focused on creating a chaos test for SDK terminators, running it and fixing any issues found.
The test repeatedly and randomly restarts the controller, routers and tunnelers then verifies that terminators
end up in the correct state. 

The following tools were also used/added to aid in diagnosing and fixing issues:

* `ziti fabric validate router-sdk-terminators` 
    * Compares the controller state with the router state
* `ziti fabric validate terminators`
    * Checks each selected terminator to ensure it's still valid on the router and/or sdk
* `ziti fabric inspect sdk-terminators`
    * Allows inspecting each routers terminator state
* `ziti fabric inspect router-messaging`
    * Allows inspecting what the controller has queued for router state sync and terminator validations
* `ziti edge validate service-hosting`
    * Shows how many terminators each identity which can host a service has

Several changes were made to the terminator code to ensure that terminators are properly created and cleaned up.
The routers now use an adaptive rate limiter to control how fast they send terminator related requests to the
controller. For this to work properly, the rate limiting on the controller must be enabled, so it can report
back to the routers when it's got too much work.

## Component Updates and Bug Fixes

* github.com/openziti/edge-api: [v0.26.10 -> v0.26.12](https://github.com/openziti/edge-api/compare/v0.26.10...v0.26.12)
* github.com/openziti/ziti: [v0.32.2 -> v0.33.0](https://github.com/openziti/ziti/compare/v0.32.2...v0.33.0)
    * [Issue #1815](https://github.com/openziti/ziti/issues/1815) - Panic if api session sync failed handler is called twice in the router
    * [Issue #1794](https://github.com/openziti/ziti/issues/1794) - Add SDK terminator chaos test and fix any bugs found as part of chaos testing
    * [Issue #1781](https://github.com/openziti/ziti/issues/1781) - Improve performance when adding intercepted services
    * [Issue #1369](https://github.com/openziti/ziti/issues/1369) - Allow filtering by policy type when listing identities for service or services for identity
    * [Issue #1791](https://github.com/openziti/ziti/issues/1791) - route dial isn't checking for network timeouts correctly
    * [Issue #1204](https://github.com/openziti/ziti/issues/1204) - ziti cli identity tags related flags misbehaving
    * [Issue #987](https://github.com/openziti/ziti/issues/987) - "ziti create config router edge" doesn't know about --tunnelerMode proxy
    * [Issue #652](https://github.com/openziti/ziti/issues/652) - Update CLI script M1 Support when github actions allows
