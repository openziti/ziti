package network

import "time"

const (
	DefaultNetworkOptionsCycleSeconds            = 60
	DefaultNetworkOptionsRouteTimeout            = 10 * time.Second
	DefaultNetworkOptionsCreateCircuitRetries    = 3
	DefaultNetworkOptionsCtrlChanLatencyInterval = 10 * time.Second
	DefaultNetworkOptionsPendingLinkTimeout      = 10 * time.Second
	DefaultNetworkOptionsSmartRerouteFraction    = 0.02
	DefaultNetworkOptionsSmartRerouteCap         = 4
)
