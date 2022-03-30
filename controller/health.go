package controller

import (
	"context"
	gosundheit "github.com/AppsFlyer/go-sundheit"
	"github.com/AppsFlyer/go-sundheit/checks"
	"github.com/openziti/foundation/metrics"
	"github.com/openziti/storage/boltz"
	"github.com/openziti/foundation/util/concurrenz"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
	"time"
)

func (c *Controller) initializeHealthChecks() (gosundheit.Health, error) {
	healthChecker := gosundheit.New()
	check, err := checks.NewPingCheck("bolt.read", &boltPinger{
		dbProvider:  c.network.GetDb,
		openReadTxs: c.GetNetwork().GetMetricsRegistry().Gauge("bolt.open_read_txs"),
	})

	if err != nil {
		return nil, err
	}

	err = healthChecker.RegisterCheck(check,
		gosundheit.InitialDelay(c.config.HealthChecks.BoltCheck.InitialDelay),
		gosundheit.ExecutionPeriod(c.config.HealthChecks.BoltCheck.Interval),
		gosundheit.ExecutionTimeout(c.config.HealthChecks.BoltCheck.Timeout),
		gosundheit.InitiallyPassing(true))

	if err != nil {
		return nil, err
	}

	return healthChecker, nil
}

type boltPinger struct {
	dbProvider  func() boltz.Db
	openReadTxs metrics.Gauge
	running     concurrenz.AtomicBoolean
}

func (self *boltPinger) PingContext(ctx context.Context) error {
	if !self.running.CompareAndSwap(false, true) {
		return errors.Errorf("previous bolt ping is still running")
	}

	deadline, hasDeadline := ctx.Deadline()

	checkFunc := func(tx *bbolt.Tx) error {
		self.openReadTxs.Update(int64(tx.DB().Stats().OpenTxN))
		return nil
	}

	if !hasDeadline {
		defer self.running.Set(false)
		return self.dbProvider().View(checkFunc)
	}

	errC := make(chan error, 1)
	go func() {
		defer self.running.Set(false)
		errC <- self.dbProvider().View(checkFunc)
	}()

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	select {
	case err := <-errC:
		return err
	case <-timer.C:
		return errors.Errorf("bolt ping timed out")
	}
}
