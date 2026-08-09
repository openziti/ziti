/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package events

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/openziti/metrics"
	"github.com/openziti/ziti/v2/common/servermetrics"
	"github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	"github.com/openziti/ziti/v2/controller/event"
	"github.com/stretchr/testify/require"
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

// Test_FilterMetrics_GroupGate guards the anyFieldAllowed fast path in
// convertMetricsMsgToEvents: a metric whose fields are all filtered out must be
// skipped (no event), while a metric matched only on a tail field (here a
// histogram's p9999) must still be emitted. The latter catches the hazard of the
// per-type key lists drifting out of sync with the filterMetric calls.
func Test_FilterMetrics_GroupGate(t *testing.T) {
	req := require.New(t)

	closeNotify := make(chan struct{})
	defer close(closeNotify)
	dispatcher := NewDispatcher(closeNotify)
	dispatcher.ctrlId = "ctrl1"

	eventC := make(chan *event.MetricsEvent, 8)
	filter, err := regexp.Compile("p9999$")
	req.NoError(err)
	adapter := dispatcher.NewFilteredMetricsAdapter(nil, filter, event.MetricsEventHandlerF(func(evt *event.MetricsEvent) {
		eventC <- evt
	}))
	dispatcher.AddMetricsMessageHandler(adapter)

	go func() {
		registry := metrics.NewRegistry("test", nil)
		registry.Histogram("some.histogram").Update(100) // only p9999 (a tail field) matches the filter
		registry.Meter("other.meter").Mark(1)            // no field matches -> must be skipped entirely
		dispatcher.AcceptMetricsMsg(servermetrics.Poll(registry))
	}()

	var evt *event.MetricsEvent
	select {
	case evt = <-eventC:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for histogram event")
	}

	req.Equal("some.histogram", evt.Metric)
	req.Equal(1, len(evt.Metrics))
	req.NotNil(evt.Metrics["p9999"])

	select {
	case extra := <-eventC:
		t.Fatalf("expected no further events, got one for %s", extra.Metric)
	case <-time.After(250 * time.Millisecond):
	}
}

func Test_FilterMetrics(t *testing.T) {
	req := require.New(t)

	closeNotify := make(chan struct{})
	defer close(closeNotify)
	dispatcher := NewDispatcher(closeNotify)
	dispatcher.ctrlId = "ctrl1"

	unfilteredEventC := make(chan *event.MetricsEvent, 1)
	adapter := dispatcher.NewFilteredMetricsAdapter(nil, nil, event.MetricsEventHandlerF(func(evt *event.MetricsEvent) {
		unfilteredEventC <- evt
	}))
	dispatcher.AddMetricsMessageHandler(adapter)

	filteredEventC := make(chan *event.MetricsEvent, 1)
	filter, err := regexp.Compile("foo.bar.(m1_rate|count)")
	req.NoError(err)
	adapter = dispatcher.NewFilteredMetricsAdapter(nil, filter, event.MetricsEventHandlerF(func(evt *event.MetricsEvent) {
		filteredEventC <- evt
	}))

	dispatcher.AddMetricsMessageHandler(adapter)

	go func() {
		registry := metrics.NewRegistry("test", nil)
		meter := registry.Meter("foo.bar")
		meter.Mark(1)
		dispatcher.AcceptMetricsMsg(servermetrics.Poll(registry))
	}()

	var evt *event.MetricsEvent
	select {
	case evt = <-unfilteredEventC:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for unfiltered event")
	}

	req.Equal("foo.bar", evt.Metric)
	fmt.Printf("%+v\n", evt.Metrics)
	req.Equal(5, len(evt.Metrics))
	req.Equal(int64(1), evt.Metrics["count"])
	req.NotNil(evt.Metrics["mean_rate"])
	req.NotNil(evt.Metrics["m1_rate"])
	req.NotNil(evt.Metrics["m5_rate"])
	req.NotNil(evt.Metrics["m15_rate"])

	select {
	case evt = <-filteredEventC:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for filtered event")
	}

	req.Equal("foo.bar", evt.Metric)
	fmt.Printf("%+v\n", evt.Metrics)
	req.Equal(2, len(evt.Metrics))
	req.Equal(int64(1), evt.Metrics["count"])
	req.NotNil(evt.Metrics["m1_rate"])
}

func Test_PrometheusMetricsLabels(t *testing.T) {
	req := require.New(t)

	baseEvent := func() *PrometheusMetricsEvent {
		return &PrometheusMetricsEvent{
			Namespace:   event.MetricsEventNS,
			MetricType:  "meter",
			Metric:      "ctrl.rx.msgrate",
			SourceAppId: "ctrl1",
			Timestamp:   time.Now(),
			Metrics:     map[string]any{"m1_rate": 0.235},
			Version:     event.MetricsEventsVersion,
		}
	}

	// without a source entity id, only source_id is emitted
	evt := baseEvent()
	buf, err := evt.Marshal(false)
	req.NoError(err)
	out := string(buf)
	req.Contains(out, `source_id="ctrl1"`)
	req.NotContains(out, "source_entity_id")

	// per-router ctrl metrics share a source_id but must be distinguished by source_entity_id
	router1 := baseEvent()
	router1.SourceEntityId = "router1"
	buf, err = router1.Marshal(false)
	req.NoError(err)
	out1 := string(buf)
	req.Contains(out1, `source_id="ctrl1"`)
	req.Contains(out1, `source_entity_id="router1"`)

	router2 := baseEvent()
	router2.SourceEntityId = "router2"
	buf, err = router2.Marshal(false)
	req.NoError(err)
	out2 := string(buf)
	req.Contains(out2, `source_entity_id="router2"`)

	// the two routers' series must not be identical (which would cause Prometheus to drop samples)
	req.NotEqual(out1, out2)
}

func Test_MetricsFormat(t *testing.T) {
	req := require.New(t)

	closeNotify := make(chan struct{})
	defer close(closeNotify)
	dispatcher := NewDispatcher(closeNotify)

	unfilteredEventC := make(chan *event.MetricsEvent, 1)
	adapter := dispatcher.NewFilteredMetricsAdapter(nil, nil, event.MetricsEventHandlerF(func(evt *event.MetricsEvent) {
		unfilteredEventC <- evt
	}))
	dispatcher.AddMetricsMessageHandler(adapter)

	go func() {
		registry := metrics.NewRegistry("test", nil)
		meter := registry.Meter("foo.bar")
		time.Sleep(10 * time.Millisecond)
		meter.Mark(1)
		dispatcher.AcceptMetricsMsg(servermetrics.Poll(registry))
	}()

	var evt *event.MetricsEvent
	select {
	case evt = <-unfilteredEventC:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for unfiltered event")
	}

	req.Equal("foo.bar", evt.Metric)
	fmt.Printf("%+v\n", evt.Metrics)
	req.Equal(5, len(evt.Metrics))
	req.Equal(int64(1), evt.Metrics["count"])
	req.NotNil(evt.Metrics["mean_rate"])
	req.NotNil(evt.Metrics["m1_rate"])
	req.NotNil(evt.Metrics["m5_rate"])
	req.NotNil(evt.Metrics["m15_rate"])

	jsonEvent := (*JsonMetricsEvent)(evt)
	buf, err := jsonEvent.Format()
	req.NoError(err)

	jsonData := map[string]any{}
	req.NoError(json.Unmarshal(buf, &jsonData))

	req.Equal("metrics", jsonData["namespace"])
	req.Equal(float64(event.MetricsEventsVersion), jsonData["version"])
	req.Equal("meter", jsonData["metric_type"])
	req.Equal("foo.bar", jsonData["metric"])
	req.Equal("test", jsonData["source_id"])
	req.Equal(evt.SourceEventId, jsonData["source_event_id"])
	req.Equal(evt.Timestamp.Format(time.RFC3339Nano), jsonData["timestamp"])

	nested, ok := jsonData["metrics"]
	req.True(ok)
	nestedJson, ok := nested.(map[string]any)
	req.True(ok)

	req.Equal(float64(1), nestedJson["count"])
}

// Test_FilterMetrics_MappedNames guards the interaction between the anyFieldAllowed fast path and the metrics
// mappers. The mappers rewrite a metric's name after that gate runs, and filterMetric then matches against the
// rewritten name, so a filter has to be written against the emitted one: an operator cannot write
// link.<id>.latency.p99, since the link id is not known in advance. Testing only the raw name at the gate
// rejects such a filter and takes the whole family with it.
func Test_FilterMetrics_MappedNames(t *testing.T) {
	tests := []struct {
		name        string
		filter      string
		rawMetric   string
		expectEvent bool
		expectName  string
	}{
		{
			name:        "a link filter written against the emitted name",
			filter:      `^link\.latency\.p99$`,
			rawMetric:   "link.abc123.latency",
			expectEvent: true,
			expectName:  "link.latency",
		},
		{
			name:        "a ctrl filter written against the emitted name",
			filter:      `^ctrl\.latency\.p99$`,
			rawMetric:   "ctrl.latency:router1",
			expectEvent: true,
			expectName:  "ctrl.latency",
		},
		{
			// A raw-name filter selects nothing, as it did before this gate existed: the per-field gate has
			// always matched against the emitted name, so such a filter never chose any field and the event was
			// discarded for having none. Failing here reaches the same outcome without the wasted work.
			name:        "a link filter written against the raw name",
			filter:      `^link\.abc123\.latency\.p99$`,
			rawMetric:   "link.abc123.latency",
			expectEvent: false,
		},
		{
			// The fast path still has to drop what matches neither, or it stops being a fast path.
			name:        "a filter matching neither name",
			filter:      `^something\.else\.p99$`,
			rawMetric:   "link.abc123.latency",
			expectEvent: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := require.New(t)

			closeNotify := make(chan struct{})
			defer close(closeNotify)
			dispatcher := NewDispatcher(closeNotify)
			dispatcher.ctrlId = "ctrl1"
			// The link mapper's tag enrichment needs a network, which this test has none of, so only its name
			// rewrite is stood in for. That is all the filter fast path depends on.
			dispatcher.addMetricsMapper(ctrlChannelMetricsMapper{}.mapMetrics)
			dispatcher.addMetricsMapper(func(_ *metrics_pb.MetricsMessage, evt *event.MetricsEvent) {
				if strings.HasPrefix(evt.Metric, "link.") {
					evt.Metric, evt.SourceEntityId = splitMetricName(evt.Metric)
				}
			})

			eventC := make(chan *event.MetricsEvent, 8)
			filter, err := regexp.Compile(test.filter)
			req.NoError(err)
			adapter := dispatcher.NewFilteredMetricsAdapter(nil, filter, event.MetricsEventHandlerF(func(evt *event.MetricsEvent) {
				eventC <- evt
			}))
			dispatcher.AddMetricsMessageHandler(adapter)

			go func() {
				registry := metrics.NewRegistry("test", nil)
				registry.Histogram(test.rawMetric).Update(100)
				dispatcher.AcceptMetricsMsg(servermetrics.Poll(registry))
			}()

			if !test.expectEvent {
				select {
				case evt := <-eventC:
					t.Fatalf("expected the family to be dropped, got an event for %s", evt.Metric)
				case <-time.After(250 * time.Millisecond):
				}
				return
			}

			select {
			case evt := <-eventC:
				req.Equal(test.expectName, evt.Metric)
				req.NotNil(evt.Metrics["p99"], "the matched field must be present")
			case <-time.After(time.Second):
				t.Fatalf("timed out waiting for an event")
			}
		})
	}
}

// Test_splitMetricName_matchesTheMappers guards against the shared name rewrite drifting from what the mappers
// actually emit, which would make the filter fast path test for a name no metric is ever emitted under.
//
// This is the whole safety net for that fast path, now that registering a mapper is internal to the package: a
// new mapper means teaching splitMetricName its rewrite and adding a name for it here.
func Test_splitMetricName_matchesTheMappers(t *testing.T) {
	raws := []string{
		"link.abc123.latency",
		"link.abc123.queue_time",
		"link.abc123.tx.bytesrate",
		"link.dropped_msgs:abc123",
		"ctrl.latency:router1",
		"ctrl.router1.tx.bytesrate",
		"pool.router.rx.queue_size",
	}

	for _, raw := range raws {
		t.Run(raw, func(t *testing.T) {
			req := require.New(t)

			viaMapper := &event.MetricsEvent{Metric: raw}
			ctrlChannelMetricsMapper{}.mapMetrics(nil, viaMapper)
			if strings.HasPrefix(raw, "link.") {
				name, linkId := splitMetricName(raw)
				viaMapper.Metric = name
				viaMapper.SourceEntityId = linkId
			}

			name, entityId := splitMetricName(raw)
			req.Equal(viaMapper.Metric, name, "emitted name")
			req.Equal(viaMapper.SourceEntityId, entityId, "entity id")
		})
	}
}
