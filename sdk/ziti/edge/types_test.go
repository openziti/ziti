/*
	Copyright 2019 Netfoundry, Inc.

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

package edge

import (
	"reflect"
	"strings"
	"testing"
)

func TestNetworkSessionDecode(t *testing.T) {
	resp := `
{"meta":{},
"data":{"_links":{"self":{"href":"./network-sessions/a7dde565-dec8-4188-90e5-42f5d33bf5a6"}},
"gateways":[
{"hostname":"hermes-host.ziti.netfoundry.io","name":"hermes","urls":{"tls":"tls://hermes-host.ziti.netfoundry.io:3022"}}],
"id":"a7dde565-dec8-4188-90e5-42f5d33bf5a6","token":"75d9aa68-dde3-4243-a062-50fab347b781"}}
`
	ns := new(NetworkSession)

	_, err := ApiResponseDecode(ns, strings.NewReader(resp))
	if err != nil {
		t.Fatal(err)
	}

	gateways := make([]Gateway,1)
	gateways[0].Name = "hermes"
	gateways[0].Hostname = "hermes-host.ziti.netfoundry.io"
	gateways[0].Urls = map[string]string{
		"tls": "tls://hermes-host.ziti.netfoundry.io:3022",
	}
	expected := &NetworkSession{
		Token: "75d9aa68-dde3-4243-a062-50fab347b781",
		Id: "a7dde565-dec8-4188-90e5-42f5d33bf5a6",
		Gateways: gateways,
	}

	if !reflect.DeepEqual(expected, ns) {
		t.Errorf("decode network session = %+v, want %+v", ns, expected)
	}
}
