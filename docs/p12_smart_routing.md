# Phase 12: Smart Routing 1

Ziti Fabric 0.4 includes an initial, experimental implementation of "smart routing".

Smart routing continually measures the latency between established links in the overlay mesh. The latency data is used in these ways:

1. When establishing a "circuit" ("path" across the mesh) for a new session, the current latency data is used as a component of the overall "cost" of each candidate path. The cost is fed into the shortest path computation, which uses that data to select the least expensive path across the mesh. So, when new sessions are created, they're given the current least expensive path across the mesh.

2. The controller periodically scans the network, looking for the sessions experiencing the highest amount of latency. A portion of the sessions with the most latency have new paths computed, and if a session's newly-computed path is different then its current path, that session will be dynamically "optimized" onto the new path.

This is an experimental implementation. It _should_ result in relatively balanced network utilization, along with some amount of the active network sessions experiencing better then average performance. These algorithms need testing and feedback from real world workloads.

Keep in mind that the "cost" attribute on each link is user-settable (see the `ziti-fabric link set-cost` tool), and will be added to the link cost considered by the controller. By smart utilization of the link cost, you can directly influence the decisions made by the smart routing algorithms.

Setting the link cost will provoke the controller to immediately re-consider the routes for each of the sessions currently traversing the adjusted link. If better paths are available for those sessions, they will be re-routed.

## Network Scaling

This style of smart routing is intended to dovetail with a scaling strategy where additional network capacity (routers) is spun up and made available as the network workload increases. Smart routing will see the new, non-latent capacity and begin redirecting traffic to it, re-balancing the network.

There are quite a number of different strategies that are possible for selecting locations to site additional mesh capacity. The work around exploring and understanding those strategies needs to be coordinated with the design of the smart routing and network optimization algorithms present in Ziti. The "fablab" project currently exists to create tooling and infrastructure to explore these kinds of scenarios. We also encourage you to experiment.

As overall network volume decreases, the additional capacity can be removed from the overlay mesh, and smart routing will again optimize the sessions to best take advantage of performance available in the mesh topology.

## Configuration

There are a couple of configuration settings for tuning how smart routing behaves on your network:

	network:
	  cycleSeconds:         15
	  smart:
	    rerouteFraction:    0.02
	    rerouteCap:         4

The above stanza goes into your controller's YAML configuration file.

The `cycleSeconds` parameter controls how frequently the controller will scan the network, looking for active sessions to be optimized.

The `rerouteFraction` parameter controls the fraction of the total number of sessions to be considered for optimization. The sessions are scanned in descending order of their overall latency, and the worst `rerouteFraction` will be considered.

The `rerouteCap` parameter defines the upper limit on the total number of sessions to be considered for optimization, even if `rerouteFraction` would select more sessions. This allows an overall cap to set on the number of sessions to be optimized.

By tuning the controller scan cycle rate (`cycleSeconds`), you can control how frequently your network will be optimized. This is the primary configuration control, which determines how quickly the smart routing algorithms will respond to changing network weather. This performance will need to be balanced against controller CPU burn, and the additional control plane bandwidth required to implement the changing session routing.

The current implementation is missing a configuration setting, which determines how frequently latency probes are performed on the overlay mesh links. Latency probes are currently performed on 10 second intervals. There probably is not much point in reducing `network.cycleSeconds` below `10`, until the latency probe interval can be configured. We expect this, along with other additional configuation settings, will be available very soon.

## What's Next?

Research for the next version of these algorithms is currently underway. We expect the next iteration to include cost considerations and targets around bandwidth metrics.

We would love your feedback. Please let us know how our current smart routing implementation performs on your real world workloads. If you have ideas for additional capabilities, please let us know!

