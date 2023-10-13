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
	"os"
	"time"
)

const DomainName = "controller.testing.openziti.org"
const Create = "CREATE"
const Delete = "DELETE"

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

func Route53StringCreator(m *model.Model, action string) string {
	var payload = Payload{
		Changes: []Change{
			{
				Action: action,
				ResourceRecordSet: ResourceRecordSet{
					Name: DomainName, // The DNS record name
					Type: "A",        // Type A represents an IPv4 address
					TTL:  300,        // TTL value in seconds
					ResourceRecords: []struct {
						Value string `json:"Value"`
					}{
						{Value: m.MustSelectHost("#ctrl").PublicIp},
					},
				},
			},
		},
	}
	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling struct to JSON:", err)
	}
	dnsAddJsonData := string(jsonData)
	s := "aws route53 change-resource-record-sets --hosted-zone-id Z09612893W445K5ME8MYS --change-batch '" + dnsAddJsonData + "'"
	return s
}

func (a *bootstrapAction) bind(m *model.Model) model.Action {
	workflow := actions.Workflow()
	// Set AWS config remotely
	accessKey := os.Getenv("S3_KEY")
	if accessKey != "" {
		fmt.Println("S3_KEY", accessKey)
	} else {
		fmt.Println("S3_KEY missing")
	}
	accessSecret := os.Getenv("S3_SECRET")
	if accessSecret != "" {
		fmt.Println("S3_SECRET", accessSecret)
	} else {
		fmt.Println("S3_SECRET missing")
	}
	accessKeyIDString := "export AWS_ACCESS_KEY_ID=" + accessKey
	accessSecretString := "export AWS_SECRET_ACCESS_KEY=" + accessSecret
	setAccessKeyIDString := "aws configure set aws_access_key_id " + accessKey
	setAccessSecretString := "aws configure set aws_secret_access_key " + accessSecret
	workflow.AddAction(host.GroupExec("#ctrl", 1, accessKeyIDString))
	workflow.AddAction(host.GroupExec("#ctrl", 1, accessSecretString))
	workflow.AddAction(host.GroupExec("#ctrl", 1, setAccessKeyIDString))
	workflow.AddAction(host.GroupExec("#ctrl", 1, setAccessSecretString))
	workflow.AddAction(host.GroupExec("#ctrl", 1, "aws configure set default.region us-east-1"))
	workflow.AddAction(host.GroupExec("#ctrl", 1, "aws configure set default.output json"))

	// Run aws_setup script - passing in AWS Key and Secret
	awsScriptExecutionText := "sudo /home/ubuntu/fablab/bin/aws_setup.sh " + accessKey + " " + accessSecret
	workflow.AddAction(host.GroupExec("#ctrl", 1, "sudo chmod 0755 /home/ubuntu/fablab/bin/aws_setup.sh"))
	workflow.AddAction(host.GroupExec("#ctrl", 1, awsScriptExecutionText))

	//Add Route53 DNS A Record
	workflow.AddAction(model.ActionFunc(func(run model.Run) error {
		m := run.GetModel()
		s := Route53StringCreator(m, Create)
		return host.Exec(m.MustSelectHost("#ctrl"), s).Execute(run)
	}))

	//Start Ziti Controller
	workflow.AddAction(host.GroupExec("#ctrl", 1, "rm -f logs/*"))
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
	workflow.AddAction(host.GroupExec("ctrl", 25, "sudo service filebeat stop; sleep 5; sudo service filebeat start"))
	workflow.AddAction(host.GroupExec("ctrl", 25, "sudo service metricbeat stop; sleep 5; sudo service metricbeat start"))

	// Run DB Creation Shell script
	workflow.AddAction(host.GroupExec("ctrl", 1, "sudo chmod 0755 /home/ubuntu/fablab/bin/db_creator_script_external.sh"))
	workflow.AddAction(host.GroupExec("ctrl", 1, "sudo /home/ubuntu/fablab/bin/db_creator_script_external.sh"))
	return workflow
}
