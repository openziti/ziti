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

package ctrlmode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name        string
		clusterMode string
		ha          bool
		haFound     bool
		mode        Mode
		err         string
	}{
		{name: "cluster-3", clusterMode: "cluster-3", mode: Cluster3},
		{name: "cluster-1", clusterMode: "cluster-1", mode: Cluster1},
		{name: "non-ha", clusterMode: "non-ha", mode: NonHa},
		{name: "cluster_mode wins over ha", clusterMode: "cluster-1", ha: false, haFound: true, mode: Cluster1},
		{name: "ha true", ha: true, haFound: true, mode: Cluster3},
		{name: "ha false", ha: false, haFound: true, mode: NonHa},
		{name: "neither set", mode: Cluster1},
		{name: "unknown mode", clusterMode: "cluster-2", err: `invalid cluster_mode "cluster-2", must be cluster-3, cluster-1 or non-ha`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode, err := resolveMode(test.clusterMode, test.ha, test.haFound, Cluster1)
			if test.err != "" {
				require.EqualError(t, err, test.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.mode, mode)
		})
	}
}

func TestModeShape(t *testing.T) {
	require.True(t, Cluster3.Clustered())
	require.True(t, Cluster1.Clustered())
	require.False(t, NonHa.Clustered())

	require.Equal(t, 3, Cluster3.CtrlCount())
	require.Equal(t, 1, Cluster1.CtrlCount())
	require.Equal(t, 1, NonHa.CtrlCount())
}
