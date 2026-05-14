package network

import (
	"sync"
	"testing"

	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/stretchr/testify/require"
)

func TestStaleLinkReportCollector_RecordConcurrentReports(t *testing.T) {
	req := require.New(t)
	collector := newStaleLinkReportCollector(1)

	const reportsPerSide = 100
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < reportsPerSide; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			collector.Record(&ctrl_pb.LinkStaleReport{
				LinkId: "link1",
				Side:   ctrl_pb.StaleLinkSide_StaleLinkSideDialer,
				Stale:  true,
				Reason: "dialer",
			})
		}()
		go func() {
			defer wg.Done()
			<-start
			collector.Record(&ctrl_pb.LinkStaleReport{
				LinkId: "link1",
				Side:   ctrl_pb.StaleLinkSide_StaleLinkSideListener,
				Stale:  true,
				Reason: "listener",
			})
		}()
	}

	close(start)
	wg.Wait()

	v := collector.Get("link1")
	req.Equal(mgmt_pb.StaleVerdict_StaleVerdictStale, v.dialer)
	req.Equal(mgmt_pb.StaleVerdict_StaleVerdictStale, v.listener)
	req.Len(v.reasons, reportsPerSide*2)

	stale, partial := aggregateVerdicts(v)
	req.True(stale)
	req.False(partial)
}

func TestStaleLinkVerdicts_GcRequiresBothEndpointsStale(t *testing.T) {
	staleVerdict := mgmt_pb.StaleVerdict_StaleVerdictStale
	okVerdict := mgmt_pb.StaleVerdict_StaleVerdictNotStale
	unknownVerdict := mgmt_pb.StaleVerdict_StaleVerdictUnknown

	tests := []struct {
		name          string
		verdicts      linkVerdicts
		expectStale   bool
		expectPartial bool
		expectGc      bool
	}{
		{
			name:          "both endpoints stale",
			verdicts:      linkVerdicts{dialer: staleVerdict, listener: staleVerdict},
			expectStale:   true,
			expectPartial: false,
			expectGc:      true,
		},
		{
			name:          "dialer stale listener ok",
			verdicts:      linkVerdicts{dialer: staleVerdict, listener: okVerdict},
			expectStale:   true,
			expectPartial: false,
			expectGc:      false,
		},
		{
			name:          "dialer ok listener stale",
			verdicts:      linkVerdicts{dialer: okVerdict, listener: staleVerdict},
			expectStale:   true,
			expectPartial: false,
			expectGc:      false,
		},
		{
			name:          "dialer stale listener unknown",
			verdicts:      linkVerdicts{dialer: staleVerdict, listener: unknownVerdict},
			expectStale:   true,
			expectPartial: true,
			expectGc:      false,
		},
		{
			name:          "dialer unknown listener stale",
			verdicts:      linkVerdicts{dialer: unknownVerdict, listener: staleVerdict},
			expectStale:   true,
			expectPartial: true,
			expectGc:      false,
		},
		{
			name:          "both endpoints ok",
			verdicts:      linkVerdicts{dialer: okVerdict, listener: okVerdict},
			expectStale:   false,
			expectPartial: false,
			expectGc:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := require.New(t)

			stale, partial := aggregateVerdicts(&tt.verdicts)
			req.Equal(tt.expectStale, stale)
			req.Equal(tt.expectPartial, partial)
			req.Equal(tt.expectGc, fullyConfirmedStale(&tt.verdicts))
		})
	}
}
