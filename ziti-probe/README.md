Ziti Probe
----------

`ziti-probe` is utility application allowing to collect network metrics from the _edge_.

## Setup

create config type for metric collection service with the following schema (this is optional)
```json
{
  "$id": "http://ziti-edge.netfoundry.io/schemas/ziti-probe-config.v1.json",
  "type":"object",
  "additionalProperties": false,
  "required": [],
  "properties":{
    "dbType": {
      "type":"string"
    },
    "dbName": {
      "type": "string"
    },
    "interval": {
      "type": "integer",
      "minimum": 1,
      "maximum": 3600
    }
  }
}
```
create service configuration `probe-service-cfg` (exact name does not matter, 
it wil be used to configure the service in the next step), This is optional
```js
{
  "dbType": "influxdb", // only influxdb is currently supported 
  "dbName": "ziti", // name of database in Influxdb, ziti is the default
  "interval": 60 // how often metrics are uploaded (in seconds), 60 is the default
}
```
create service with a router termination
```shell script
$ ziti edge controller create service probe-service probe-service-cfg
$ ziti edge controller create terminator probe-service router1 tcp:127.0.0.1:8086
```
in this example `router1` is the name of the fabric router that has access to influxDB server, 
and `tcp:127.0.0.1:8086` is the address of influxDB server. Adjust as needed.

create ziti identity(ies) for your probe(s). `ziti-probe` is a Ziti-SDK-enabled application, 
so standard enrollment procedure should be followed.

## Running `ziti-probe`

```shell script
$ ziti-probe run <identity-file>
```
 #### What's happening?
 1. `ziti-probe` authenticates to Ziti controller with provided identity
 1. it checks that `probe-service` is available and get configuration. If configuration is not assigned default values are used.
 1. it opens connections to all edge routers configured for `probe-service` (refer to ziti policies)
 1. each connection performs a round-trip latency probe to each of edge routers every 30 seconds
 1. `ziti-probe` then submits collected metrics to `probe-service`. 
 metric names are in the following format: `latency.<edgeRouterAddress>`, e.g `latency.tls://54.145.23.14:3022`
