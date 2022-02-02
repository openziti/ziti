package events

import (
	"fmt"
	"github.com/openziti/fabric/event"
	"github.com/openziti/fabric/metrics"
	metrics2 "github.com/openziti/foundation/metrics"
	"github.com/stretchr/testify/require"
	"regexp"
	"testing"
	"time"
)

func Test_ExtractId(t *testing.T) {
	name := "ctrl.3tOOkKfDn.tx.bytesrate"

	req := require.New(t)
	name, entityId := ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, "3tOOkKfDn")

	name = "ctrl.3tO.kKfDn.tx.bytesrate"
	name, entityId = ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, "3tO.kKfDn")

	name = "ctrl.3tO.kK.Dn.tx.bytesrate"
	name, entityId = ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, "3tO.kK.Dn")

	name = "ctrl..tO.kK.Dn.tx.bytesrate"
	name, entityId = ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, ".tO.kK.Dn")

	name = "ctrl..tO.kK.D..tx.bytesrate"
	name, entityId = ExtractId(name, "ctrl.", 2)
	req.Equal(name, "ctrl.tx.bytesrate")
	req.Equal(entityId, ".tO.kK.D.")
}

func Test_FilterMetrics(t *testing.T) {
	req := require.New(t)

	dispatcher := metrics.NewDispatchWrapper(func(event event.Event) {
		event.Handle()
	})

	unfilteredEventC := make(chan *MetricsEvent, 1)
	cleanupUnfiltered := AddFilteredMetricsEventHandler(nil, nil, MetricsHandlerF(func(event *MetricsEvent) {
		unfilteredEventC <- event
	}))

	defer cleanupUnfiltered()

	filteredEventC := make(chan *MetricsEvent, 1)
	filter, err := regexp.Compile("foo.bar.(m1_rate|count)")
	req.NoError(err)
	cleanupFiltered := AddFilteredMetricsEventHandler(nil, filter, MetricsHandlerF(func(event *MetricsEvent) {
		filteredEventC <- event
	}))

	defer cleanupFiltered()

	go func() {
		registry := metrics2.NewRegistry("test", nil)
		meter := registry.Meter("foo.bar")
		meter.Mark(1)
		dispatcher.AcceptMetrics(registry.Poll())
	}()

	var event *MetricsEvent
	select {
	case event = <-unfilteredEventC:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for unfiltered event")
	}

	req.Equal("foo.bar", event.Metric)
	fmt.Printf("%+v\n", event.Metrics)
	req.Equal(5, len(event.Metrics))
	req.Equal(int64(1), event.Metrics["count"])
	req.NotNil(event.Metrics["mean_rate"])
	req.NotNil(event.Metrics["m1_rate"])
	req.NotNil(event.Metrics["m5_rate"])
	req.NotNil(event.Metrics["m15_rate"])

	select {
	case event = <-filteredEventC:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for filtered event")
	}

	req.Equal("foo.bar", event.Metric)
	fmt.Printf("%+v\n", event.Metrics)
	req.Equal(2, len(event.Metrics))
	req.Equal(int64(1), event.Metrics["count"])
	req.NotNil(event.Metrics["m1_rate"])
}
