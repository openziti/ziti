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

	"github.com/openziti/channel/v5"
	"github.com/openziti/metrics"
	"github.com/stretchr/testify/require"
)

// closeTestChannel is a channel.Channel whose Close succeeds. The rest panics to surface accidental use.
type closeTestChannel struct {
	channel.Channel
}

func (closeTestChannel) Close() error { return nil }

type closeTestLinkChannel struct {
	LinkChannel
}

func (closeTestLinkChannel) GetChannel() channel.Channel { return closeTestChannel{} }

// TestLinkClose_DisposesEveryMetric: a link registers its dropped-message meters in Init, and nothing but its
// own close releases them, so any it leaves behind outlives it for the router's lifetime.
func TestLinkClose_DisposesEveryMetric(t *testing.T) {
	registry := metrics.NewRegistry("test", nil)
	link := &impl{id: "l1", ch: closeTestLinkChannel{}}
	require.NoError(t, link.Init(registry))

	var registered []string
	registry.EachMetric(func(name string, _ metrics.Metric) { registered = append(registered, name) })
	require.NotEmpty(t, registered)

	require.NoError(t, link.Close())

	for _, name := range registered {
		require.False(t, registry.IsValidMetric(name), "%s must be disposed when the link closes", name)
	}
}

// TestLinkClose_BeforeInit: a link can be closed before Init has registered anything.
func TestLinkClose_BeforeInit(t *testing.T) {
	link := &impl{id: "l1", ch: closeTestLinkChannel{}}
	require.NoError(t, link.Close())
}
