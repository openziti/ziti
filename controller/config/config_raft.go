package config

import (
	"github.com/hashicorp/go-hclog"
	"github.com/openziti/transport/v2"
	"time"
)

type RaftConfig struct {
	Recover               bool
	DataDir               string
	AdvertiseAddress      transport.Address
	InitialMembers        []string
	CommandHandlerOptions struct {
		MaxQueueSize uint16
	}

	SnapshotInterval  *time.Duration
	SnapshotThreshold *uint32
	TrailingLogs      *uint32
	MaxAppendEntries  *uint32

	ElectionTimeout    time.Duration
	CommitTimeout      *time.Duration
	HeartbeatTimeout   time.Duration
	LeaderLeaseTimeout time.Duration

	LogLevel *string
	Logger   hclog.Logger

	WarnWhenLeaderlessFor time.Duration
}
