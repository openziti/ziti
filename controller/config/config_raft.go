package config

import (
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/controller/command"
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

	ApplyTimeout    time.Duration
	PreferredLeader bool
	RateLimiter     command.AdaptiveRateLimitTrackerConfig
	PeerDialer      PeerDialerConfig

	// NonMemberGrace is how long the leader will allow a TLS-valid but
	// non-member controller to stay connected to the mesh before dropping it.
	NonMemberGrace time.Duration
}

// PeerDialerConfig controls retry behavior for the cluster peer dialer.
type PeerDialerConfig struct {
	MinRetryInterval   time.Duration
	MaxRetryInterval   time.Duration
	RetryBackoffFactor float64
	FastFailureWindow  time.Duration
	DialTimeout        time.Duration
	// ScanInterval is the period of the dialer's full scan, which reconciles
	// dial states against current cluster membership.
	ScanInterval time.Duration
	// QueueCheckInterval is how often the dialer pops expired entries from its
	// retry heap. Effectively the resolution of MinRetryInterval.
	QueueCheckInterval time.Duration
}
