package config

import (
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/controller/command"
)

type RaftConfig struct {
	Recover               bool
	DataDir               string
	RestartSelf           bool
	AdvertiseAddress      transport.Address
	CommandHandlerOptions struct {
		MaxQueueSize uint16
	}

	SnapshotInterval  time.Duration
	SnapshotThreshold uint32
	TrailingLogs      uint32
	MaxAppendEntries  *uint32

	ElectionTimeout    time.Duration
	CommitTimeout      *time.Duration
	HeartbeatTimeout   time.Duration
	LeaderLeaseTimeout time.Duration

	LogLevel *string
	Logger   hclog.Logger

	WarnWhenLeaderlessFor time.Duration

	ApplyTimeout time.Duration
	RateLimiter  command.AdaptiveRateLimiterConfig
}
