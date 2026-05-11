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

package state

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/v2/common"
	"github.com/openziti/ziti/v2/router/env"
	"github.com/openziti/ziti/v2/router/managedconfig"
)

// configAllowList is the slice of ManagedConfigOptions the subscriber relies
// on. Narrowing it to an interface keeps the subscriber testable without
// pulling in the full RouterEnv.
type configAllowList interface {
	IsAllowed(configType string) bool
}

// RouterConfigSubscriber is the bridge from RDM Config events to the
// managedconfig.Registry. It filters incoming events through the operator's
// allow-list before dispatching to the registry, so a router never applies a
// config type the operator hasn't opted into.
//
// Errors from ApplyController/RemoveController are logged; the subscriber
// has no way to push back on the RDM (events are already committed). The
// registry's own alert callback covers handler-level failures.
type RouterConfigSubscriber struct {
	allow    configAllowList
	registry *managedconfig.Registry
}

// NewRouterConfigSubscriber constructs a subscriber bound to the given
// RouterEnv. Pulls the allow-list and registry off env at construction time
// so OnRouterConfig* don't re-resolve them per call.
func NewRouterConfigSubscriber(routerEnv env.RouterEnv) *RouterConfigSubscriber {
	return &RouterConfigSubscriber{
		allow:    &routerEnv.GetConfig().ManagedConfig,
		registry: routerEnv.GetRouterConfigRegistry(),
	}
}

// newRouterConfigSubscriberFromParts is the testable constructor; production
// code should use NewRouterConfigSubscriber.
func newRouterConfigSubscriberFromParts(allow configAllowList, registry *managedconfig.Registry) *RouterConfigSubscriber {
	return &RouterConfigSubscriber{allow: allow, registry: registry}
}

// OnRouterConfigApplied implements common.RouterConfigEventSubscriber. Drops
// configs whose type isn't allowed by the local managed-config policy; logs
// rejections at info so operators can see why a controller config didn't
// land.
func (self *RouterConfigSubscriber) OnRouterConfigApplied(configType string, data string) {
	if self.allow == nil || !self.allow.IsAllowed(configType) {
		pfxlog.Logger().WithField("configType", configType).Info("router config not in local allow-list; ignoring controller apply")
		return
	}
	if err := self.registry.ApplyController(configType, data); err != nil {
		pfxlog.Logger().WithField("configType", configType).WithError(err).Warn("registry rejected ApplyController")
	}
}

// OnRouterConfigRemoved implements common.RouterConfigEventSubscriber. Remove
// also goes through the allow-list: if the operator has since revoked a type
// it accepted earlier, the registry already lost ownership of it; we don't
// need to forward the removal.
func (self *RouterConfigSubscriber) OnRouterConfigRemoved(configType string) {
	if self.allow == nil || !self.allow.IsAllowed(configType) {
		return
	}
	if err := self.registry.RemoveController(configType); err != nil {
		pfxlog.Logger().WithField("configType", configType).WithError(err).Warn("registry rejected RemoveController")
	}
}

// ensure interface compliance
var _ common.RouterConfigEventSubscriber = (*RouterConfigSubscriber)(nil)
