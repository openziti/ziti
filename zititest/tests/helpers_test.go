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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openziti/ziti/v2/controller/rest_client/terminator"
	"github.com/openziti/ziti/v2/zitirest"
	"github.com/openziti/ziti/zititest/models/smoke"
)

// sdkClientSupport reports whether the client routers can carry SDK clients' connections.
func sdkClientSupport() smoke.Support {
	return smoke.RoutersSupport(run.GetModel(), ".client", smoke.CapOidc)
}

// sdkHostingSupport reports whether the hosting routers can carry SDK-hosted services.
func sdkHostingSupport() smoke.Support {
	return smoke.RoutersSupport(run.GetModel(), ".host", smoke.CapSdkHosting, smoke.CapOidc)
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

// hostedServices returns the encrypted and unencrypted kind services of each of hostTypes that the
// hosting routers can carry.
func hostedServices(kind string, hostTypes ...string) []string {
	var result []string
	for _, hostType := range hostTypes {
		if hostingSupport(hostType).Supported {
			result = append(result, hostType+"-"+kind, hostType+"-"+kind+"-unencrypted")
		}
	}
	return result
}

func waitForTerminators(t *testing.T, timeout time.Duration, services ...string) {
	t.Helper()

	ctrls := run.GetModel().SelectComponents(".ctrl")
	if len(ctrls) == 0 {
		t.Fatal("no controller components found in model")
	}

	c := ctrls[0]
	username := c.MustStringVariable("credentials.edge.username")
	password := c.MustStringVariable("credentials.edge.password")
	edgeApiBaseUrl := c.Host.PublicIp + ":1280"

	clients, err := zitirest.NewManagementClients(edgeApiBaseUrl)
	if err != nil {
		t.Fatalf("failed to create management clients: %v", err)
	}

	if err = clients.Authenticate(username, password); err != nil {
		t.Fatalf("failed to authenticate: %v", err)
	}

	deadline := time.Now().Add(timeout)

	for _, serviceName := range services {
		filter := fmt.Sprintf(`service.name = "%s" limit 1`, serviceName)

		for {
			ctx, cancelF := context.WithTimeout(context.Background(), 15*time.Second)
			result, err := clients.Fabric.Terminator.ListTerminators(&terminator.ListTerminatorsParams{
				Filter:  &filter,
				Context: ctx,
			}, nil)
			cancelF()

			if err == nil && result.Payload.Meta.Pagination.TotalCount != nil && *result.Payload.Meta.Pagination.TotalCount > 0 {
				t.Logf("terminators found for service %s", serviceName)
				break
			}

			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for terminators for service %s after %v", serviceName, timeout)
			}

			t.Logf("waiting for terminators for service %s...", serviceName)
			time.Sleep(time.Second)
		}
	}
}
