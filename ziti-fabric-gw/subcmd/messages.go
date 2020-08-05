/*
	Copyright NetFoundry, Inc.

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

package subcmd

type circuit struct {
	Path  []string `json:"path"`
	Links []string `json:"links"`
}

type data struct {
	Data []interface{} `json:"data"`
}

type link struct {
	Id         string  `json:"id"`
	Src        string  `json:"src"`
	Dst        string  `json:"dst"`
	State      string  `json:"state"`
	Down       bool    `json:"down"`
	Cost       int32   `json:"cost"`
	SrcLatency float64 `json:"srcLatency"`
	DstLatency float64 `json:"dstLatency"`
}

type router struct {
	Id              string `json:"id"`
	Fingerprint     string `json:"fingerprint"`
	ListenerAddress string `json:"listenerAddress"`
	Connected       bool   `json:"connected"`
}

type service struct {
	Id              string `json:"id"`
	Binding         string `json:"binding"`
	EndpointAddress string `json:"endpointAddress"`
	Egress          string `json:"egress"`
}

type session struct {
	Id        string   `json:"id"`
	ClientId  string   `json:"clientId"`
	ServiceId string   `json:"serviceId"`
	Circuit   *circuit `json:"circuit"`
}
