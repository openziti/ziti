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
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/config"
)

// Xrctrl provides an extension point for router-side controller communication,
// enabling components to register custom message handlers and extend the basic
// fabric functionality with additional protocols and message flows.
//
// This interface allows modular components to participate in the router-controller
// communication channel without requiring changes to the core networking stack.
// Components implementing this interface can handle specialized messages, maintain
// their own state synchronization, and react to controller connection events.
//
// There is a corresponding Xctrl interface for extending communication on
// the controller side, enabling bidirectional protocol extensions.
type Xrctrl interface {
	config.Subconfig
	channel.BindHandler

	// Enabled returns whether this extension component should be activated.
	// This allows for conditional component loading based on configuration
	// or runtime conditions.
	Enabled() bool

	// Run starts the extension component's main operation loop. The component
	// should respect the router environment's lifecycle signals and terminate
	// gracefully when requested.
	Run(env RouterEnv) error

	// NotifyOfReconnect handles controller reconnection events, allowing
	// components to reinitialize state or reestablish subscriptions after
	// network interruptions or controller failover scenarios.
	NotifyOfReconnect(ch channel.Channel)
}
