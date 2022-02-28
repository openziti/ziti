package controller

import "time"

const (
	DefaultEdgeApiActivityUpdateBatchSize = 250
	DefaultEdgeAPIActivityUpdateInterval  = 90 * time.Second
	MaxEdgeAPIActivityUpdateBatchSize     = 10000
	MinEdgeAPIActivityUpdateBatchSize     = 1
	MaxEdgeAPIActivityUpdateInterval      = 10 * time.Minute
	MinEdgeAPIActivityUpdateInterval      = time.Millisecond

	DefaultEdgeSessionTimeout = 10 * time.Minute
	MinEdgeSessionTimeout     = 1 * time.Minute

	MinEdgeEnrollmentDuration     = 5 * time.Minute
	DefaultEdgeEnrollmentDuration = 5 * time.Minute

	DefaultHttpIdleTimeout       = 5000 * time.Millisecond
	DefaultHttpReadTimeout       = 5000 * time.Millisecond
	DefaultHttpReadHeaderTimeout = 5000 * time.Millisecond
	DefaultHttpWriteTimeout      = 100000 * time.Millisecond
)
