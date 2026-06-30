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

package metrics_pb_test

import (
	"testing"

	libpb "github.com/openziti/metrics/metrics_pb"
	zitipb "github.com/openziti/ziti/v2/common/servermetrics/metrics_pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Test_WireCompatibleWithLibraryMetricsMessage verifies ziti's MetricsMessage is
// byte-compatible on the wire with the metrics library's MetricsMessage, in both
// directions. A router on either proto and a controller on the other must
// interoperate, since the move keeps the field numbers identical and the proto
// package name is not transmitted in the binary encoding.
//
// The fact that this test binary links both proto packages (distinct proto
// package names: ziti.common.servermetrics.pb vs ziti.metrics.pb) without
// panicking at init also guards against a global proto-registry
// duplicate-registration clash.
func Test_WireCompatibleWithLibraryMetricsMessage(t *testing.T) {
	req := require.New(t)

	z := &zitipb.MetricsMessage{
		EventId:     "event-1",
		SourceId:    "router-1",
		Tags:        map[string]string{"k": "v"},
		IntValues:   map[string]int64{"i": 7},
		FloatValues: map[string]float64{"f": 1.5},
		Meters: map[string]*zitipb.MetricsMessage_Meter{
			"m": {Count: 3, M1Rate: 1.1, MeanRate: 2.5},
		},
		Histograms: map[string]*zitipb.MetricsMessage_Histogram{
			"h": {Count: 4, Mean: 9.0, Max: 20, P99: 12},
		},
		Timers: map[string]*zitipb.MetricsMessage_Timer{
			"t": {Count: 5, Mean: 1.0, MeanRate: 0.5, P95: 3},
		},
		IntervalCounters: map[string]*zitipb.MetricsMessage_IntervalCounter{
			"ic": {IntervalLength: 60, Buckets: []*zitipb.MetricsMessage_IntervalBucket{
				{IntervalStartUTC: 1000, Values: map[string]uint64{"a": 1}},
			}},
		},
		UsageCounters: []*zitipb.MetricsMessage_UsageCounter{
			{IntervalStartUTC: 2000, IntervalLength: 60, Buckets: map[string]*zitipb.MetricsMessage_UsageBucket{
				"b": {Values: map[string]uint64{"u": 2}, Tags: map[string]string{"tk": "tv"}},
			}},
		},
		DoNotPropagate: true,
	}

	// ziti -> bytes -> library
	zBytes, err := proto.Marshal(z)
	req.NoError(err)
	l := &libpb.MetricsMessage{}
	req.NoError(proto.Unmarshal(zBytes, l))

	req.Equal(z.EventId, l.EventId)
	req.Equal(z.SourceId, l.SourceId)
	req.Equal(z.Tags, l.Tags)
	req.Equal(z.IntValues, l.IntValues)
	req.Equal(z.FloatValues, l.FloatValues)
	req.Equal(z.DoNotPropagate, l.DoNotPropagate)
	req.Equal(z.Meters["m"].Count, l.Meters["m"].Count)
	req.Equal(z.Meters["m"].MeanRate, l.Meters["m"].MeanRate)
	req.Equal(z.Histograms["h"].P99, l.Histograms["h"].P99)
	req.Equal(z.Timers["t"].MeanRate, l.Timers["t"].MeanRate)
	req.Equal(z.IntervalCounters["ic"].Buckets[0].Values, l.IntervalCounters["ic"].Buckets[0].Values)
	req.Equal(z.UsageCounters[0].Buckets["b"].Values, l.UsageCounters[0].Buckets["b"].Values)
	req.Equal(z.UsageCounters[0].Buckets["b"].Tags, l.UsageCounters[0].Buckets["b"].Tags)

	// library -> bytes -> ziti (reverse direction)
	lBytes, err := proto.Marshal(l)
	req.NoError(err)
	z2 := &zitipb.MetricsMessage{}
	req.NoError(proto.Unmarshal(lBytes, z2))
	req.Equal(z.EventId, z2.EventId)
	req.Equal(z.Histograms["h"].Mean, z2.Histograms["h"].Mean)
	req.Equal(z.UsageCounters[0].IntervalStartUTC, z2.UsageCounters[0].IntervalStartUTC)

	// the channel content-type must be identical so message routing is unchanged
	req.Equal(int32(libpb.ContentType_MetricsType), int32(zitipb.ContentType_MetricsType))
}
