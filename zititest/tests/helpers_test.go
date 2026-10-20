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

package tests

import (
	"github.com/openziti/ziti/zititest/models/smoke"
)

// sdkClientSupport reports whether the client routers can carry SDK clients' connections.
func sdkClientSupport() smoke.Support {
	return smoke.RoutersSupport(run.GetModel(), ".client", smoke.CapOidc)
}

// sdkHostingSupport reports whether the hosting routers can carry SDK-hosted services.
func sdkHostingSupport() smoke.Support {
	return smoke.RoutersSupport(run.GetModel(), ".host", smoke.CapOidc)
}

// clientSupport reports whether the client routers can carry connections from clients of clientType.
// Every client type but the router's own tunneler, "ert", connects through the SDK.
func clientSupport(clientType string) smoke.Support {
	if clientType == "ert" {
		return smoke.Support{Supported: true}
	}
	return sdkClientSupport()
}

// hostingSupport reports whether the hosting routers can carry services hosted by hostType. Every host
// type but the router's own tunneler, "ert", hosts through the SDK.
func hostingSupport(hostType string) smoke.Support {
	if hostType == "ert" {
		return smoke.Support{Supported: true}
	}
	return sdkHostingSupport()
}

// pathSupport reports whether clients of clientType can reach services hosted by hostType.
func pathSupport(clientType, hostType string) smoke.Support {
	return clientSupport(clientType).And(hostingSupport(hostType))
}
