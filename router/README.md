# Ziti Router Components

This document describes the key components of the Ziti router architecture.

## Core Components

### NetworkControllers
Manages connections to Ziti controllers. Handles controller discovery, connection establishment, failover, and maintains heartbeats. Routes control plane messages between the router and controllers.

### Link Registry
Maintains the registry of active links to other routers in the mesh. Tracks link states, manages link lifecycle, and provides link lookup capabilities for routing decisions.

### Forwarder
The packet forwarding engine that routes data between xgress instances. Delivers payloads to links,
xgress instances or edge forwarders.

### Faulter
Monitors link health and detects failures. Automatically reports link faults to controllers and triggers link recovery processes when connectivity issues are detected.

### Scanner
Continuously scans the forwarder's state to detect and clean up stale forwarding entries. Removes inactive circuits and performs periodic maintenance of the forwarding tables.

## Data Plane Components

### Xgress Components
Handle ingress and egress traffic for the router:

- **Xgress Listeners**: Accept incoming connections from clients and services
- **Xgress Dialers**: Establish outbound connections to services 
- **Xgress Factories**: Create protocol-specific xgress instances (edge, transport, proxy, tunnel)
- **Xgress Registry**: Global registry managing all xgress factory types

### Xgress Types
- `edge`: Handles Ziti SDK connections
- `transport`: Router-to-router communication
- `proxy`: TCP/UDP proxy connections  
- `tunnel`: Intercepted traffic from tunneling clients

## Metrics
Comprehensive metrics collection covering: 
- Link latency and throughput
- Xgress connection statistics
- Pool utilization (link dialer, egress dialer, rate limiter)

All metrics are reported to controllers and available via health check APIs.