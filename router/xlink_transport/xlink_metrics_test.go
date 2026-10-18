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

package xlink_transport

import (
	"testing"

	"github.com/openziti/metrics"
	"github.com/stretchr/testify/require"
)

// Test_impl_disposeMetrics_releasesEveryMeter guards against the metrics leak
// where Init registered four dropped-message meters but Close disposed only one,
// leaking the other three on every link close. Init and disposeMetrics must stay
// in sync.
func Test_impl_disposeMetrics_releasesEveryMeter(t *testing.T) {
	req := require.New(t)
	registry := metrics.NewRegistry("test-router", nil)

	link := &impl{id: "testlink"}
	req.NoError(link.Init(registry))

	names := []string{
		"link.dropped_msgs:testlink",
		"link.dropped_xg_msgs:testlink",
		"link.dropped_rtx_msgs:testlink",
		"link.dropped_fwd_msgs:testlink",
	}
	for _, name := range names {
		req.NotNil(registry.GetMeter(name), "expected %s registered after Init", name)
	}

	link.disposeMetrics()

	for _, name := range names {
		req.Nil(registry.GetMeter(name), "expected %s disposed after close", name)
	}
}
