# Running Quickstart Tests
The quickstart test ensures a network is minimally functional by creating a service that intercepts the controller's 
edge HTTP/API plane, then obtains a valid http response by dialing that service.

The following environment variables are referenced for network information.

* `ZITI_USER` - defaults to `admin`
* `ZITI_PWD`  - defaults to `admin`
* `ZITI_CTRL_EDGE_ADVERTISED_ADDRESS` - defaults to `ziti-edge-controller`
* `ZITI_CTRL_EDGE_ADVERTISED_PORT`    - defaults to `1280`
* `ZITI_ROUTER_NAME` - defaults to `ziti-edge-router`

## Automated
The automated test starts up an ephemeral OpenZiti network and runs `performQuickstartTest`.

To run the automated edge test, execute the following from the project root (environment variables are auto populated)
```
go test -v -tags "quickstart automated" ./ziti/cmd/edge/... -run TestEdgeQuickstsart
```

## Manual
The manual test utilizes an existing network and runs `performQuickstartTest`.

To run the manual test, edit the environment variables as necessary and execute the following from the project root
```
go test -v -tags "quickstart manual" ./ziti/cmd/edge/... -run TestSimpleWebService
```