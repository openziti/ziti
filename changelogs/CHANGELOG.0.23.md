# Release 0.23.1

## What's New

* Performance: Improve route selection cpu and memory use.
* Bug fix: Fix controller panic in routes.MapApiSessionToRestModel caused by missing return

# Release 0.23.0

## What's New

* Bug fix: Fix panic in router when router is shutdown before control channel is established
* Enhancement: Add source/target router ids on link metrics.
* Security: Fabric management channel wasn't properly validating certs against the server cert chain
* Security: Router link listeners weren't properly validating certs against the server cert chain
* Security: Link listeners now validate incoming links to ensure that the link was requested by the controller and the
  correct router dialed
* Security: Don't allow link forwarding entries to be overridden, as link ids should be unique
* Security: Validate ctrl channel clients against controller cert chain in addition to checking cert fingerprint

## Breaking Changes

The link validation required a controller side and router side component. The controller will continue to work with
earlier routers, but the routers with version >= 0.23.0 will need a controller with version >= 0.23.0.

## Link Metrics Router Ids

The link router ids will now be included as tags on the metrics.

```
{
  "metric": "link.latency",
  "metrics": {
    "link.latency.count": 322,
    "link.latency.max": 844083,
    "link.latency.mean": 236462.8671875,
    "link.latency.min": 100560,
    "link.latency.p50": 212710.5,
    "link.latency.p75": 260137.75,
    "link.latency.p95": 491181.89999999997,
    "link.latency.p99": 820171.6299999995,
    "link.latency.p999": 844083,
    "link.latency.p9999": 844083,
    "link.latency.std_dev": 118676.24663550049,
    "link.latency.variance": 14084051515.49014
  },
  "namespace": "metrics",
  "source_entity_id": "lDWL",
  "source_event_id": "52f9de3e-4293-4d4f-9dc8-5c4f40b04d12",
  "source_id": "4ecTdw8lG6",
  "tags": {
    "sourceRouterId": "CorTdA8l7",
    "targetRouterId": "4ecTdw8lG6"
  },
  "timestamp": "2021-11-10T18:04:32.087107445Z"
}
```

Note that this information is injected into the metric in the controller. If the controller doesn't know about the link,
because of a controller restart, the information can't be added.
