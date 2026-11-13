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

// Package ctrlmode selects how a fablab model runs its controllers: as a three-node raft cluster,
// as a single-node raft cluster, or as one controller without raft.
package ctrlmode

import (
	"fmt"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/fablab/kernel/model"
)

// Mode is how a model runs its controllers.
type Mode string

const (
	// Cluster3 runs the model's three controllers as one raft cluster.
	Cluster3 Mode = "cluster-3"
	// Cluster1 runs the primary controller alone, as a single-node raft cluster.
	Cluster1 Mode = "cluster-1"
	// NonHa runs the primary controller alone, without raft.
	NonHa Mode = "non-ha"
)

const (
	// ModeVariable selects the mode. When it is unset or empty, the ha variable picks Cluster3 (true)
	// or NonHa (false).
	ModeVariable = "cluster_mode"

	// ClusteredVariable is set by Factory to whether the controllers run raft, for templates to read.
	ClusteredVariable = "ctrl_is_clustered"

	// OidcVariable is set by Factory to whether the controllers serve the OIDC API, for templates to
	// read.
	OidcVariable = "ctrl_oidc"

	// PrimaryCtrlId is the controller a single-controller mode keeps.
	PrimaryCtrlId = "ctrl1"
)

// Clustered reports whether mode runs raft.
func (mode Mode) Clustered() bool {
	return mode != NonHa
}

// CtrlCount returns how many controllers mode runs.
func (mode Mode) CtrlCount() int {
	if mode == Cluster3 {
		return 3
	}
	return 1
}

// Factory returns a structure factory that resolves m's mode, using defaultMode when neither
// cluster_mode nor ha is set. For a single-controller mode it removes every controller but
// PrimaryCtrlId, and any host that leaves empty. It fails if the controllers left don't number
// Mode.CtrlCount, and sets ClusteredVariable and OidcVariable. OIDC follows the oidc variable, which
// defaults to true; a cluster requires OIDC, so oidc false fails in a clustered mode. Run it before any
// factory that reads those variables or selects controllers.
func Factory(defaultMode Mode) model.Factory {
	return model.FactoryFunc(func(m *model.Model) error {
		clusterMode, _ := m.GetStringVariable(ModeVariable)
		ha, haFound := m.GetBoolVariable("ha")
		mode, err := resolveMode(clusterMode, ha, haFound, defaultMode)
		if err != nil {
			return err
		}

		if mode.CtrlCount() == 1 {
			for _, c := range m.SelectComponents(".ctrl") {
				if c.Id == PrimaryCtrlId {
					continue
				}
				delete(c.Host.Components, c.Id)
				if len(c.Host.Components) == 0 {
					delete(c.Host.Region.Hosts, c.Host.Id)
				}
			}
		}

		if count := len(m.SelectComponents(".ctrl")); count != mode.CtrlCount() {
			return fmt.Errorf("%s %s runs %d controllers, but the model has %d", ModeVariable, mode, mode.CtrlCount(), count)
		}

		oidc, oidcFound := m.GetBoolVariable("oidc")
		oidcEnabled, err := resolveOidc(mode, oidc, oidcFound)
		if err != nil {
			return err
		}

		m.Scope.Defaults[ClusteredVariable] = mode.Clustered()
		m.Scope.Defaults[OidcVariable] = oidcEnabled

		pfxlog.Logger().Infof("cluster mode [%s], oidc enabled [%v]", mode, oidcEnabled)
		return nil
	})
}

// IsClustered reports whether m's controllers run raft. It is valid only after Factory has run.
func IsClustered(m *model.Model) bool {
	return m.GetFlag(ClusteredVariable)
}

// OidcEnabled reports whether m's controllers serve the OIDC API. It is valid only after Factory has
// run.
func OidcEnabled(m *model.Model) bool {
	return m.GetFlag(OidcVariable)
}

func resolveMode(clusterMode string, ha, haFound bool, defaultMode Mode) (Mode, error) {
	switch mode := Mode(clusterMode); mode {
	case Cluster3, Cluster1, NonHa:
		return mode, nil
	case "":
		if !haFound {
			return defaultMode, nil
		}
		if ha {
			return Cluster3, nil
		}
		return NonHa, nil
	default:
		return "", fmt.Errorf("invalid %s %q, must be %s, %s or %s", ModeVariable, clusterMode, Cluster3, Cluster1, NonHa)
	}
}

func resolveOidc(mode Mode, oidc, oidcFound bool) (bool, error) {
	if !oidcFound {
		return true, nil
	}
	if !oidc && mode.Clustered() {
		return false, fmt.Errorf("oidc is false, but %s %s runs raft, which requires OIDC; use %s", ModeVariable, mode, NonHa)
	}
	return oidc, nil
}
