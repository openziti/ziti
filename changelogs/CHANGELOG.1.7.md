# Release 1.7.1

## What's New

This release updates the build to use Go 1.25.+. This is the only change in the release.

# Release 1.7.0

## What's New

* proxy.v1 config type
* Alert Events (Beta)
* Azure Service Bus Event Sink (contributed by @ffaraone)

## New proxy.v1 Config Type

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

## Alert Events

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
and proven useful, they will marked as stable.

## Azure Service Bus Event Sink

GitHub user @ffaraone contributed this feature, which adds support for streaming controller events to Azure Service Bus. 
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

## Component Updates and Bug Fixes

* github.com/openziti/agent: [v1.0.31 -> v1.0.33](https://github.com/openziti/agent/compare/v1.0.31...v1.0.33)
* github.com/openziti/channel/v4: [v4.2.28 -> v4.2.41](https://github.com/openziti/channel/compare/v4.2.28...v4.2.41)
* github.com/openziti/edge-api: [v0.26.47 -> v0.26.50](https://github.com/openziti/edge-api/compare/v0.26.47...v0.26.50)
* github.com/openziti/foundation/v2: [v2.0.72 -> v2.0.79](https://github.com/openziti/foundation/compare/v2.0.72...v2.0.79)
    * [Issue #455](https://github.com/openziti/foundation/issues/455) - Correctly close goroutine pool when external close is signaled
    * [Issue #452](https://github.com/openziti/foundation/issues/452) - Goroutine pool with a min worker count of 1 can drop to 0 workers due to race condition

* github.com/openziti/identity: [v1.0.111 -> v1.0.118](https://github.com/openziti/identity/compare/v1.0.111...v1.0.118)
    * [Issue #68](https://github.com/openziti/identity/issues/68) - Shutdown file watcher when stopping identity watcher

* github.com/openziti/runzmd: [v1.0.80 -> v1.0.84](https://github.com/openziti/runzmd/compare/v1.0.80...v1.0.84)
* github.com/openziti/sdk-golang: [v1.2.3 -> v1.2.10](https://github.com/openziti/sdk-golang/compare/v1.2.3...v1.2.10)
    * [Issue #818](https://github.com/openziti/sdk-golang/issues/818) - Full re-auth should not clear services list, as that breaks the on-change logic
    * [Issue #817](https://github.com/openziti/sdk-golang/issues/817) - goroutines can get stuck when iterating over randomized HA controller list
    * [Issue #736](https://github.com/openziti/sdk-golang/issues/736) - Migrate from github.com/mailru/easyjson
    * [Issue #813](https://github.com/openziti/sdk-golang/issues/813) - SDK doesn't stop close listener when it detects that a service being hosted gets deleted
    * [Issue #811](https://github.com/openziti/sdk-golang/issues/811) - Credentials are lost when explicitly set
    * [Issue #807](https://github.com/openziti/sdk-golang/issues/807) - Don't send close from rxer to avoid blocking
    * [Issue #800](https://github.com/openziti/sdk-golang/issues/800) - Tidy create service session logging

* github.com/openziti/secretstream: [v0.1.39 -> v0.1.41](https://github.com/openziti/secretstream/compare/v0.1.39...v0.1.41)
* github.com/openziti/storage: [v0.4.26 -> v0.4.31](https://github.com/openziti/storage/compare/v0.4.26...v0.4.31)
* github.com/openziti/transport/v2: [v2.0.188 -> v2.0.198](https://github.com/openziti/transport/compare/v2.0.188...v2.0.198)
* github.com/openziti/go-term-markdown: v1.0.1 (new)
* github.com/openziti/ziti: [v1.6.8 -> v1.7.0](https://github.com/openziti/ziti/compare/v1.6.8...v1.7.0)
    * [Issue #3264](https://github.com/openziti/ziti/issues/3264) - Add support for streaming events to Azure Service Bus
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
    * [Issue #3207](https://github.com/openziti/ziti/issues/3207) - Allow router embedders to customize config before start
