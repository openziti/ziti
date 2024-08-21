# Running Quickstart Tests

The quickstart test ensures a network is minimally functional by creating a service that intercepts the controller's 
edge HTTP/API plane, then obtains a valid http response by dialing that service.

## Automated

The automated test starts up an ephemeral OpenZiti network and runs `performQuickstartTest`.

To run the automated edge test, execute the following from the project root (environment variables are auto populated)

```bash
# From project root
go test -v -tags "quickstart automated" ./ziti/cmd/edge/... -run TestEdgeQuickstartAutomated

# From relative README path
go test -v -tags "quickstart automated" ../../ziti/cmd/edge/... -run TestEdgeQuickstartAutomated
```

## Manual

The manual test utilizes an existing network and runs `performQuickstartTest`. Use the manual method to test
any network, anywhere, by adjusting the environment variables as necessary.

The following environment variables are referenced for network information.

* `ZITI_USER` - defaults to `admin`
* `ZITI_PWD`  - defaults to `admin`
* `ZITI_CTRL_EDGE_ADVERTISED_ADDRESS` - defaults to `ziti-edge-controller`
* `ZITI_CTRL_EDGE_ADVERTISED_PORT`    - defaults to `1280`
* `ZITI_ROUTER_NAME` - defaults to `ziti-edge-router`

To run the manual test, edit the environment variables as necessary and execute the following from the project root

```bash
# Optionally adjust environment variable values as needed
ZITI_USER="admin"
ZITI_PWD="admin"
ZITI_CTRL_EDGE_ADVERTISED_ADDRESS="ziti-edge-controller"
ZITI_CTRL_EDGE_ADVERTISED_PORT="1280"
ZITI_ROUTER_NAME="ziti-edge-router"

# From project root
go test -v -tags "quickstart manual" ./ziti/cmd/edge/... -run TestEdgeQuickstartManual

# From relative README path
go test -v -tags "quickstart manual" ../../ziti/cmd/edge/... -run TestEdgeQuickstartManual
```
