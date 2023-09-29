/*
	(c) Copyright NetFoundry Inc.

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
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/router/xlink"
)

// BindHandlerFactory can be implemented and provided to the factory to perform channel binding and other channel setup
// tasks at accept time.
type BindHandlerFactory interface {
	NewBindHandler(xlink xlink.Xlink, latency bool, listenerSide bool) channel.BindHandler
}
