package xgress_edge

import (
	"github.com/openziti/channel"
	"github.com/openziti/edge/edge_common"
	"github.com/openziti/fabric/router/xgress"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

/*
	Copyright NetFoundry, Inc.

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

func Test_load(t *testing.T) {

	t.Run("config with connect options", func(t *testing.T) {
		options := &Options{}
		testConfigWithConnectOptions := newTestConfigWithConnectOptions()
		err := options.load(testConfigWithConnectOptions)

		t.Run("does not error on load", func(t *testing.T) {
			require.New(t).NoError(err)
		})

		t.Run("has MaxQueuedConnects value set", func(t *testing.T) {
			require.New(t).Equal(connectMaxQueuedConnections, options.channelOptions.MaxQueuedConnects)
		})

		t.Run("has MaxOutstandingConnects value set", func(t *testing.T) {
			require.New(t).Equal(connectMaxOutstandingConnects, options.channelOptions.MaxOutstandingConnects)
		})

		t.Run("has ConnectTimeout value set", func(t *testing.T) {
			require.New(t).Equal(connectTimeout, options.channelOptions.ConnectTimeout)
		})
	})

	t.Run("config without connect options", func(t *testing.T) {
		options := &Options{}
		defaults := channel.DefaultConnectOptions()
		testConfigWithoutConnectOptions := newTestConfigWithoutConnectOptions()
		err := options.load(testConfigWithoutConnectOptions)

		t.Run("does not error on load", func(t *testing.T) {
			require.New(t).NoError(err)
		})

		t.Run("has MaxQueuedConnects set to the default value", func(t *testing.T) {
			require.New(t).Equal(defaults.MaxQueuedConnects, options.channelOptions.MaxQueuedConnects)
		})

		t.Run("has MaxOutstandingConnects set to the default value", func(t *testing.T) {
			require.New(t).Equal(defaults.MaxOutstandingConnects, options.channelOptions.MaxOutstandingConnects)
		})

		t.Run("has ConnectTimeout set to the default value", func(t *testing.T) {
			require.New(t).Equal(defaults.ConnectTimeout, options.channelOptions.ConnectTimeout)
		})
	})

	t.Run("config with invalid connect options", func(t *testing.T) {
		t.Run("errors if MaxQueuedConnects is invalid", func(t *testing.T) {
			options := &Options{}
			testConfig := newTestConfigWithoutConnectOptions()
			optionsConfig := testConfig["options"].(map[interface{}]interface{})
			optionsConfig["maxQueuedConnects"] = 0

			err := options.load(testConfig)
			require.New(t).Error(err)
		})

		t.Run("errors if MaxOutstandingConnects is invalid", func(t *testing.T) {
			options := &Options{}
			testConfig := newTestConfigWithoutConnectOptions()
			optionsConfig := testConfig["options"].(map[interface{}]interface{})
			optionsConfig["maxOutstandingConnects"] = 0

			err := options.load(testConfig)
			require.New(t).Error(err)
		})

		t.Run("errors if ConnectTimeout is invalid", func(t *testing.T) {
			options := &Options{}
			testConfig := newTestConfigWithoutConnectOptions()
			optionsConfig := testConfig["options"].(map[interface{}]interface{})
			optionsConfig["connectTimeout"] = "abcded"

			err := options.load(testConfig)
			require.New(t).Error(err)
		})
	})
}

const (
	connectMaxQueuedConnections   = 50
	connectMaxOutstandingConnects = 100
	connectTimeout                = 1000 * time.Millisecond
)

func newTestConfigWithConnectOptions() xgress.OptionsData {
	return xgress.OptionsData{
		"binding": edge_common.EdgeBinding,
		"address": "tls:0.0.0.0:3022",
		"options": map[interface{}]interface{}{
			"advertise":              "127.0.0.1:3022",
			"maxQueuedConnects":      connectMaxQueuedConnections,
			"maxOutstandingConnects": connectMaxOutstandingConnects,
			"connectTimeout":         connectTimeout.String(),
		},
	}
}

func newTestConfigWithoutConnectOptions() xgress.OptionsData {
	return xgress.OptionsData{
		"binding": edge_common.EdgeBinding,
		"address": "tls:0.0.0.0:3022",
		"options": map[interface{}]interface{}{
			"advertise": "127.0.0.1:3022",
		},
	}
}
