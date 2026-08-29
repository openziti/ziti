package network

import (
	"fmt"
	"sync"
	"testing"

	"github.com/openziti/foundation/v2/versions"
	"github.com/openziti/ziti/v2/common/capabilities"
	"github.com/openziti/ziti/v2/common/pb/ctrl_pb"
	"github.com/openziti/ziti/v2/common/pb/mgmt_pb"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/models"
	"github.com/stretchr/testify/require"
)

func TestScopeLinksToRouters(t *testing.T) {
	req := require.New(t)
	mkLink := func(id, srcId, dstId string) *model.Link {
		return &model.Link{
			Id:    id,
			Src:   &model.Router{BaseEntity: models.BaseEntity{Id: srcId}},
			DstId: dstId,
		}
	}
	linkMap := map[string]*model.Link{
		"l1": mkLink("l1", "rA", "rB"), // src in filter
		"l2": mkLink("l2", "rB", "rA"), // dst in filter
		"l3": mkLink("l3", "rB", "rC"), // neither in filter
		"l4": mkLink("l4", "rA", "rA"), // both in filter
	}

	scoped := scopeLinksToRouters(linkMap, map[string]struct{}{"rA": {}})
	req.Len(scoped, 3)
	req.Contains(scoped, "l1")
	req.Contains(scoped, "l2")
	req.Contains(scoped, "l4")
	req.NotContains(scoped, "l3", "a link with neither endpoint in the filter is excluded")
}

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

func TestStaleLinkQueryBlocker(t *testing.T) {
	mkRouter := func(id string, caps *capabilities.RouterCapabilityMask, version string) *model.Router {
		r := &model.Router{
			BaseEntity:   models.BaseEntity{Id: id},
			Capabilities: caps,
		}
		if version != "" {
			r.VersionInfo = &versions.VersionInfo{Version: version}
		}
		return r
	}

	req := require.New(t)

	capable := mkRouter("rA", capabilities.NewMask(capabilities.RouterStaleLinkCheck), "v2.1.0")
	req.Empty(staleLinkQueryBlocker(capable), "a capable router is queried")

	// Having other capabilities is not enough.
	other := mkRouter("rB", capabilities.NewMask(capabilities.RouterDataModel), "v2.0.0")
	blocker := staleLinkQueryBlocker(other)
	req.Contains(blocker, "rB")
	req.Contains(blocker, "v2.0.0", "the reason names the version so it's actionable")

	// A pre-2.0 router sends no capabilities header at all.
	req.NotEmpty(staleLinkQueryBlocker(mkRouter("rC", nil, "v0.30.0")))

	// Version is optional; the reason must still render.
	noVersion := staleLinkQueryBlocker(mkRouter("rD", nil, ""))
	req.Contains(noVersion, "rD")
	req.Contains(noVersion, "unknown version")
}

func TestAppendUnqueryableReasons(t *testing.T) {
	req := require.New(t)
	link := &model.Link{
		Id:    "l1",
		Src:   &model.Router{BaseEntity: models.BaseEntity{Id: "rA"}},
		DstId: "rB",
	}

	req.Empty(appendUnqueryableReasons(nil, map[string]string{}, link))
	req.Empty(appendUnqueryableReasons(nil, map[string]string{"rZ": "unrelated"}, link),
		"a router that isn't an endpoint of this link contributes nothing")

	req.Equal([]string{"src down"},
		appendUnqueryableReasons(nil, map[string]string{"rA": "src down"}, link))
	req.Equal([]string{"dst down"},
		appendUnqueryableReasons(nil, map[string]string{"rB": "dst down"}, link))
	req.Equal([]string{"src down", "dst down"},
		appendUnqueryableReasons(nil, map[string]string{"rA": "src down", "rB": "dst down"}, link))

	// Reasons already gathered from reports are preserved.
	req.Equal([]string{"reported stale", "src down"},
		appendUnqueryableReasons([]string{"reported stale"}, map[string]string{"rA": "src down"}, link))
}

func TestValidateMatchMode(t *testing.T) {
	req := require.New(t)

	req.NoError(validateMatchMode(mgmt_pb.StaleLinkMatchMode_StaleLinkMatchChanged))
	req.NoError(validateMatchMode(mgmt_pb.StaleLinkMatchMode_StaleLinkMatchOrphaned))

	// A protobuf enum field can carry a value this build doesn't declare.
	// Falling back would silently pick Changed, the broadest removal criterion.
	for _, unknown := range []int32{2, 7, -1} {
		err := validateMatchMode(mgmt_pb.StaleLinkMatchMode(unknown))
		req.Error(err, "mode %d must be rejected", unknown)
		req.Contains(err.Error(), fmt.Sprintf("%d", unknown))
	}
}

func TestLinkVerdicts_JudgedIteration(t *testing.T) {
	req := require.New(t)

	iteration, agreed := (&linkVerdicts{dialerIteration: 4, listenerIteration: 4}).judgedIteration()
	req.True(agreed)
	req.Equal(uint32(4), iteration)

	// Endpoints disagreeing means the link was re-dialed mid-sweep, so neither
	// verdict describes the link as it now stands.
	_, agreed = (&linkVerdicts{dialerIteration: 4, listenerIteration: 5}).judgedIteration()
	req.False(agreed)
}

func TestApplyStaleReport_CarriesIteration(t *testing.T) {
	req := require.New(t)
	v := &linkVerdicts{}

	applyStaleReport(v, &ctrl_pb.LinkStaleReport{
		LinkId:    "l1",
		Side:      ctrl_pb.StaleLinkSide_StaleLinkSideDialer,
		Stale:     true,
		Iteration: 3,
	})
	applyStaleReport(v, &ctrl_pb.LinkStaleReport{
		LinkId:    "l1",
		Side:      ctrl_pb.StaleLinkSide_StaleLinkSideListener,
		Stale:     true,
		Iteration: 3,
	})

	req.True(fullyConfirmedStale(v))
	iteration, agreed := v.judgedIteration()
	req.True(agreed)
	req.Equal(uint32(3), iteration, "the iteration the verdict was computed against survives aggregation")
}

func TestStaleLinkReportCollector_GetPreservesIterations(t *testing.T) {
	req := require.New(t)
	collector := newStaleLinkReportCollector(1)

	collector.Record(&ctrl_pb.LinkStaleReport{
		LinkId: "l1", Side: ctrl_pb.StaleLinkSide_StaleLinkSideDialer, Stale: true, Iteration: 9,
	})
	collector.Record(&ctrl_pb.LinkStaleReport{
		LinkId: "l1", Side: ctrl_pb.StaleLinkSide_StaleLinkSideListener, Stale: true, Iteration: 9,
	})

	v := collector.Get("l1")
	iteration, agreed := v.judgedIteration()
	req.True(agreed)
	req.Equal(uint32(9), iteration, "Get copies iterations, not just verdicts")
}
