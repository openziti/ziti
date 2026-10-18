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

package env

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test_loadLinkHeartbeatOptions_OmittedKeysKeepLinkDefaults is the central case. channel.LoadHeartbeatOptions
// fills omitted keys from the channel defaults, which are not the link defaults, so naming one key used to
// move the others: a stanza setting only sendInterval halved the link's unresponsive timeout.
func Test_loadLinkHeartbeatOptions_OmittedKeysKeepLinkDefaults(t *testing.T) {
	req := require.New(t)

	options, err := loadLinkHeartbeatOptions(map[interface{}]interface{}{
		"sendInterval": "3s",
	})
	req.NoError(err)
	req.Equal(3*time.Second, options.SendInterval, "the named key applies")
	req.Equal(DefaultLinkUnresponsiveTimeout, options.CloseUnresponsiveTimeout,
		"naming sendInterval must not move the unresponsive timeout")

	options, err = loadLinkHeartbeatOptions(map[interface{}]interface{}{
		"closeUnresponsiveTimeout": "90s",
	})
	req.NoError(err)
	req.Equal(90*time.Second, options.CloseUnresponsiveTimeout)
	req.Equal(DefaultLinkHeartbeatSendInterval, options.SendInterval,
		"naming the unresponsive timeout must not move the send interval")
}

// Test_loadLinkHeartbeatOptions_EmptyStanzaKeepsLinkDefaults: an empty stanza is the same as no stanza.
func Test_loadLinkHeartbeatOptions_EmptyStanzaKeepsLinkDefaults(t *testing.T) {
	req := require.New(t)

	options, err := loadLinkHeartbeatOptions(map[interface{}]interface{}{})
	req.NoError(err)
	req.Equal(DefaultLinkHeartbeatSendInterval, options.SendInterval)
	req.Equal(DefaultLinkUnresponsiveTimeout, options.CloseUnresponsiveTimeout)
}

// Test_loadLinkHeartbeatOptions_EveryKeyApplies covers all three at once with values no other key uses, so
// a block assigning to the wrong field is visible.
func Test_loadLinkHeartbeatOptions_EveryKeyApplies(t *testing.T) {
	req := require.New(t)

	options, err := loadLinkHeartbeatOptions(map[interface{}]interface{}{
		"sendInterval":             "11s",
		"checkInterval":            "12s",
		"closeUnresponsiveTimeout": "13s",
	})
	req.NoError(err)
	req.Equal(11*time.Second, options.SendInterval)
	req.Equal(12*time.Second, options.CheckInterval)
	req.Equal(13*time.Second, options.CloseUnresponsiveTimeout)
}

// Test_loadLinkHeartbeatOptions_RejectsNonPositiveIntervals: the heartbeater's ticker panics on a
// non-positive interval, so these have to fail at load rather than be defaulted or reach the ticker. A zero
// close timeout is not in this set; it means closing is disabled.
func Test_loadLinkHeartbeatOptions_RejectsNonPositiveIntervals(t *testing.T) {
	req := require.New(t)

	for name, stanza := range map[string]map[interface{}]interface{}{
		"zero send interval":      {"sendInterval": "0s"},
		"negative send interval":  {"sendInterval": "-1s"},
		"zero check interval":     {"checkInterval": "0s"},
		"negative check interval": {"checkInterval": "-1s"},
	} {
		_, err := loadLinkHeartbeatOptions(stanza)
		req.Error(err, name)
	}

	options, err := loadLinkHeartbeatOptions(map[interface{}]interface{}{
		"closeUnresponsiveTimeout": "0s",
	})
	req.NoError(err, "a zero close timeout disables closing rather than being invalid")
	req.Zero(options.CloseUnresponsiveTimeout)
}

// Test_loadLinkHeartbeatOptions_ErrorsNameTheKey: a config error that does not say which key is wrong
// leaves an operator diffing against defaults to find it.
func Test_loadLinkHeartbeatOptions_ErrorsNameTheKey(t *testing.T) {
	req := require.New(t)

	_, err := loadLinkHeartbeatOptions(map[interface{}]interface{}{"sendInterval": "0s"})
	req.ErrorContains(err, "sendInterval")

	_, err = loadLinkHeartbeatOptions(map[interface{}]interface{}{"checkInterval": "0s"})
	req.ErrorContains(err, "checkInterval")
}
