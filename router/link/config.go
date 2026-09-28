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

package link

import (
	"github.com/openziti/ziti/v2/common/config/routerlink"
)

// The router.link config definition and validation live in common/config/routerlink
// so the controller applies the same rules without importing the link subsystem.

const (
	ConfigBaseType = routerlink.ConfigBaseType
	ConfigTypeV1   = routerlink.ConfigTypeV1
)

type (
	Config             = routerlink.Config
	ListenerConfig     = routerlink.ListenerConfig
	DialerConfig       = routerlink.DialerConfig
	ChannelOptions     = routerlink.ChannelOptions
	BackoffConfig      = routerlink.BackoffConfig
	HeartbeatsConfig   = routerlink.HeartbeatsConfig
	HeartbeatDurations = routerlink.HeartbeatDurations
	Groups             = routerlink.Groups
	GcMode             = routerlink.GcMode
)

const (
	GcModePreserve = routerlink.GcModePreserve
	GcModeOrphaned = routerlink.GcModeOrphaned
	GcModeChanged  = routerlink.GcModeChanged
)

// ParseConfig unmarshals raw JSON into a Config.
func ParseConfig(data string) (*Config, error) { return routerlink.ParseConfig(data) }

// ParseGcMode normalizes the string form of gcMode into the enum.
func ParseGcMode(s string) (GcMode, error) { return routerlink.ParseGcMode(s) }
