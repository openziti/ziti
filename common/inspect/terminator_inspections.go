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

package inspect

type TerminatorCostDetails struct {
	Terminators []*TerminatorCostDetail `json:"terminators"`
}

type TerminatorCostDetail struct {
	TerminatorId string `json:"terminatorId"`
	CircuitCount uint32 `json:"circuitCount"`
	FailureCost  uint32 `json:"failureCost"`
	CurrentCost  uint32 `json:"currentCost"`
}

type SdkTerminatorInspectResult struct {
	Entries []*SdkTerminatorInspectDetail `json:"entries"`
	Errors  []string                      `json:"errors"`
}

type SdkTerminatorInspectDetail struct {
	Key             string `json:"key"`
	Id              string `json:"id"`
	State           string `json:"state"`
	Token           string `json:"token"`
	ListenerId      string `json:"listenerId"`
	Instance        string `json:"instance"`
	Cost            uint16 `json:"cost"`
	Precedence      string `json:"precedence"`
	AssignIds       bool   `json:"assignIds"`
	V2              bool   `json:"v2"`
	SupportsInspect bool   `json:"supportsInspect"`
	OperationActive bool   `json:"establishActive"`
	CreateTime      string `json:"createTime"`
	LastAttempt     string `json:"lastAttempt"`
}

type ErtTerminatorInspectResult struct {
	Entries []*ErtTerminatorInspectDetail `json:"entries"`
	Errors  []string                      `json:"errors"`
}

type ErtTerminatorInspectDetail struct {
	Key   string `json:"key"`
	Id    string `json:"id"`
	State string `json:"state"`
}
