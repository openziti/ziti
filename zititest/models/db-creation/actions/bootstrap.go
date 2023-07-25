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

package actions

import (
	"encoding/json"
	"fmt"
	"github.com/openziti/fablab/kernel/lib/actions"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/actions/host"
	"github.com/openziti/fablab/kernel/lib/actions/semaphore"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/ziti/zititest/zitilab"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/actions/edge"
	"github.com/openziti/ziti/zititest/zitilab/models"
	"time"
)

type bootstrapAction struct{}

// Define a struct to represent the nested "ResourceRecordSet" object
type ResourceRecordSet struct {
	Name            string `json:"Name"`
	Type            string `json:"Type"`
	TTL             int    `json:"TTL"`
	ResourceRecords []struct {
		Value string `json:"Value"`
	} `json:"ResourceRecords"`
}

// Define a struct to represent the nested "Changes" object
type Change struct {
	Action            string            `json:"Action"`
	ResourceRecordSet ResourceRecordSet `json:"ResourceRecordSet"`
}

// Define the main Payload struct to represent the entire JSON payload
type Payload struct {
	Changes []Change `json:"Changes"`
}

func NewBootstrapAction() model.ActionBinder {
	action := &bootstrapAction{}
	return action.bind
}

func (a *bootstrapAction) bind(m *model.Model) model.Action {
	dnsAddJsonData := `{
	"Changes": [{
		"Action": "CREATE",
		"ResourceRecordSet": {
			"Name": "controller.testing.openziti.org",
			"Type": "A",
			"TTL": 300,
			"ResourceRecords": [{
				"Value": "52.90.174.214"
			}]
		}
	}]
}`

	// Create an instance of the Payload struct to store the JSON data
	var data Payload

	// Unmarshal the JSON data into the struct
	err := json.Unmarshal([]byte(dnsAddJsonData), &data)
	if err != nil {
		fmt.Println("Error while unmarshalling JSON:", err)
		return nil
	}

	//fmt.Println("dnsAddJsonData: \n", data)

	addDNSRecord := "aws route53 change-resource-record-sets --hosted-zone-id Z09612893W445K5ME8MYS --change-batch '" + dnsAddJsonData + "'"
	workflow := actions.Workflow()
	workflow.AddAction(host.GroupExec("*", 25, addDNSRecord))
	workflow.AddAction(host.GroupExec("*", 25, "rm -f logs/*"))
	workflow.AddAction(component.Stop("#ctrl"))
	workflow.AddAction(component.Exec("#ctrl", zitilab.ControllerActionInitStandalone))
	workflow.AddAction(component.Start("#ctrl"))
	workflow.AddAction(edge.ControllerAvailable("#ctrl", 30*time.Second))

	// Login to Ziti Controller
	workflow.AddAction(edge.Login("#ctrl"))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Setup Ziti Routers
	workflow.AddAction(component.StopInParallel(models.EdgeRouterTag, 25))
	workflow.AddAction(edge.InitEdgeRouters(models.EdgeRouterTag, 2))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Init Identities
	workflow.AddAction(edge.InitIdentities(models.SdkAppTag, 2))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Create Configs
	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-server", "host.v1", `
					{
							"address" : "localhost",
							"port" : 7001,
							"protocol" : "tcp"
					}`))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))
	workflow.AddAction(zitilib_actions.Edge("create", "config", "iperf-intercept", "intercept.v1", `
		{
			"addresses": ["iperf.service"],
			"portRanges" : [
				{ "low": 7001, "high": 7001 }
			 ],
			"protocols": ["tcp"]
		}`))
	workflow.AddAction(semaphore.Sleep(2 * time.Second))

	// Start Beats Services
	workflow.AddAction(host.GroupExec("*", 25, "sudo service filebeat stop; sleep 5; sudo service filebeat start"))
	workflow.AddAction(host.GroupExec("*", 25, "sudo service metricbeat stop; sleep 5; sudo service metricbeat start"))

	return workflow
}
