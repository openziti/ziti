package controller

import "time"

const (
	DefaultProfileMemoryInterval             = 15 * time.Second
	DefaultHealthChecksBoltCheckInterval     = 30 * time.Second
	DefaultHealthChecksBoltCheckTimeout      = 20 * time.Second
	DefaultHealthChecksBoltCheckInitialDelay = 30 * time.Second
)
