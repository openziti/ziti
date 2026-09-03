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

package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func parseNetworkConfig(t *testing.T, doc string) map[interface{}]interface{} {
	t.Helper()
	cfgmap := map[interface{}]interface{}{}
	require.NoError(t, yaml.Unmarshal([]byte(doc), &cfgmap))
	return cfgmap
}

// TestLoadNetworkConfig_EachKeyReachesItsOwnField loads every key at once, each with a value no other key
// uses, and checks every field.
//
// One key at a time would not do. This loader is a run of near-identical blocks, so the mistake it invites
// is a block that reads its own key and assigns to the field of the block it was copied from. A test that
// sets only `ioPool` and checks only `IoPool` passes whether or not that block also wrote to `PeerEventsPool`,
// and the three pool stanzas are six assignments across three blocks that differ by one word. Distinct
// values everywhere is what makes a cross-assignment visible: the wrong field holds a number that belongs
// to another key.
func TestLoadNetworkConfig_EachKeyReachesItsOwnField(t *testing.T) {
	req := require.New(t)

	options, err := LoadNetworkConfig(parseNetworkConfig(t, `
cycleSeconds: 11
routeTimeoutSeconds: 12
createCircuitRetries: 13
pendingLinkTimeoutSeconds: 14
minRouterCost: 15
routerConnectChurnLimit: 16s
initialLinkLatency: 17s
metricsReportInterval: 18s
intervalAgeThreshold: 19s
routerConnectConcurrency: 21
routerConnectSetupTimeout: 22s
routerMessaging:
  queueSize: 31
  maxWorkers: 32
gossipApplyPool:
  queueSize: 41
  maxWorkers: 42
peerEventsPool:
  queueSize: 51
  maxWorkers: 52
ioPool:
  queueSize: 61
  maxWorkers: 62
smart:
  rerouteFraction: 0.75
  rerouteCap: 71
  minCostDelta: 72
hostMetrics:
  enabled: true
`))
	req.NoError(err)

	req.Equal(uint32(11), options.CycleSeconds)
	req.Equal(12*time.Second, options.RouteTimeout)
	req.Equal(uint32(13), options.CreateCircuitRetries)
	req.Equal(14*time.Second, options.PendingLinkTimeout)
	req.Equal(uint16(15), options.MinRouterCost)
	req.Equal(16*time.Second, options.RouterConnectChurnLimit)
	req.Equal(17*time.Second, options.InitialLinkLatency)
	req.Equal(18*time.Second, options.MetricsReportInterval)
	req.Equal(19*time.Second, options.IntervalAgeThreshold)

	req.Equal(uint32(21), options.RouterConnectConcurrency)
	req.Equal(22*time.Second, options.RouterConnectSetupTimeout)

	req.Equal(uint32(31), options.RouterComm.QueueSize)
	req.Equal(uint32(32), options.RouterComm.MaxWorkers)

	// The three pool stanzas are the ones most likely to cross-assign: same two keys, different field.
	req.Equal(uint32(41), options.GossipApplyPool.QueueSize)
	req.Equal(uint32(42), options.GossipApplyPool.MaxWorkers)
	req.Equal(uint32(51), options.PeerEventsPool.QueueSize)
	req.Equal(uint32(52), options.PeerEventsPool.MaxWorkers)
	req.Equal(uint32(61), options.IoPool.QueueSize)
	req.Equal(uint32(62), options.IoPool.MaxWorkers)

	req.Equal(float32(0.75), options.Smart.RerouteFraction)
	req.Equal(uint32(71), options.Smart.RerouteCap)
	req.Equal(uint32(72), options.Smart.MinCostDelta)

	req.True(options.HostMetrics.Enabled)
}

// TestLoadNetworkConfig_AbsentKeysKeepDefaults: a config need only name what it changes, so an empty
// stanza must not zero the fields it does not mention.
func TestLoadNetworkConfig_AbsentKeysKeepDefaults(t *testing.T) {
	req := require.New(t)

	options, err := LoadNetworkConfig(parseNetworkConfig(t, "cycleSeconds: 11\n"))
	req.NoError(err)

	req.Equal(uint32(11), options.CycleSeconds)
	req.Equal(DefaultNetworkConfig().RouterConnectConcurrency, options.RouterConnectConcurrency)
	req.Equal(DefaultOptionsRouterConnectSetupTimeout, options.RouterConnectSetupTimeout)
	req.Equal(DefaultNetworkConfig().GossipApplyPool.QueueSize, options.GossipApplyPool.QueueSize)
	req.Equal(DefaultNetworkConfig().IoPool.MaxWorkers, options.IoPool.MaxWorkers)
}

// TestLoadNetworkConfig_RejectsUnusableValues covers the values that would start a controller that cannot
// do what the config asked. Failing at load names the key; failing later does not.
func TestLoadNetworkConfig_RejectsUnusableValues(t *testing.T) {
	req := require.New(t)

	for name, doc := range map[string]string{
		"connect concurrency of zero":      "routerConnectConcurrency: 0\n",
		"connect concurrency past the cap": "routerConnectConcurrency: 100000\n",
		"connect concurrency not a number": "routerConnectConcurrency: lots\n",
		// A setup timeout is a duration string, so a bare number is a unit the reader has to guess at.
		"setup timeout as a bare number": "routerConnectSetupTimeout: 120\n",
		"setup timeout not a duration":   "routerConnectSetupTimeout: 2 minutes\n",
		"setup timeout of zero":          "routerConnectSetupTimeout: 0s\n",
		"setup timeout negative":         "routerConnectSetupTimeout: -1m\n",
	} {
		_, err := LoadNetworkConfig(parseNetworkConfig(t, doc))
		req.Error(err, name)
	}
}

// TestLoadNetworkConfig_ErrorsNameTheKeyTheyReject: a config error that does not say which key is wrong
// leaves an operator diffing a config against defaults to find it.
func TestLoadNetworkConfig_ErrorsNameTheKeyTheyReject(t *testing.T) {
	req := require.New(t)

	_, err := LoadNetworkConfig(parseNetworkConfig(t, "routerConnectSetupTimeout: 0s\n"))
	req.ErrorContains(err, "routerConnectSetupTimeout")

	_, err = LoadNetworkConfig(parseNetworkConfig(t, "routerConnectConcurrency: 0\n"))
	req.ErrorContains(err, "routerConnectConcurrency")
}
