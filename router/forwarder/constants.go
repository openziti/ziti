package forwarder

import "time"

const (
	DefaultLatencyProbeInterval        = 10 * time.Second
	DefaultLatencyProbeTimeout         = 10 * time.Second
	DefaultXgressCloseCheckInterval    = 5 * time.Second
	DefaultXgressDialDwellTime         = 0
	DefaultFaultTxInterval             = 15 * time.Second
	DefaultIdleTxInterval              = 60 * time.Second
	DefaultIdleCircuitTimeout          = 60 * time.Second
	DefaultXgressDialWorkerQueueLength = 1000
	MinXgressDialWorkerQueueLength     = 1
	MaxXgressDialWorkerQueueLength     = 10000
	DefaultXgressDialWorkerCount       = 10
	MinXgressDialWorkerCount           = 1
	MaxXgressDialWorkerCount           = 10000
	DefaultLinkDialQueueLength         = 1000
	MinLinkDialWorkerQueueLength       = 1
	MaxLinkDialWorkerQueueLength       = 10000
	DefaultLinkDialWorkerCount         = 10
	MinLinkDialWorkerCount             = 1
	MaxLinkDialWorkerCount             = 10000
)
