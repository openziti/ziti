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

package routes

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

var test = `
{
	"name" : "Foo",
	"dns" : {
		"hostname" : "google.com",
		"port" : 6433
	},
	"egressRouter" : "001",
	"endpointAddress" : null
}
      
`

func Test_getFields(t *testing.T) {
	assert := require.New(t)
	test2 := ServiceApiCreate{
		EdgeRouterRoles: []string{"@foo", "2"},
		RoleAttributes:  []string{"foo", "bar"},
		Dns: &ServiceDnsApiPost{
			Hostname: strPtr("google.com"),
			Port:     uint16Ptr(6433),
		},
		Name:            strPtr("Foo"),
		HostIds:         nil,
		Tags:            nil,
		EgressRouter:    nil,
		EndpointAddress: strPtr("tcp:foo:1234"),
	}

	test2Bytes, err := json.Marshal(test2)
	assert.NoError(err)

	tests := []struct {
		name    string
		body    []byte
		want    JsonFields
		wantErr bool
	}{
		{
			name: "test",
			body: []byte(test),
			want: JsonFields{
				"Name":            true,
				"Dns.Hostname":    true,
				"Dns.Port":        true,
				"EgressRouter":    true,
				"EndpointAddress": false,
			},
			wantErr: false,
		},
		{
			name: "test2",
			body: test2Bytes,
			want: JsonFields{
				"Name":            true,
				"EdgeRouterRoles": true,
				"RoleAttributes":  true,
				"Dns.Hostname":    true,
				"Dns.Port":        true,
				"EgressRouter":    false,
				"EndpointAddress": true,
				"HostIds":         false,
				"Tags":            false,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getFields(tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("getFields() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getFields() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJsonFields_ConcatNestedNames(t *testing.T) {
	tests := []struct {
		name string
		j    JsonFields
		want JsonFields
	}{
		{"test",
			JsonFields{
				"Name":            true,
				"Dns.Hostname":    true,
				"Dns.Port":        true,
				"EgressRouter":    true,
				"EndpointAddress": false,
			},
			JsonFields{
				"Name":            true,
				"DnsHostname":     true,
				"DnsPort":         true,
				"EgressRouter":    true,
				"EndpointAddress": false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.j.ConcatNestedNames()
			if !reflect.DeepEqual(tt.j, tt.want) {
				t.Errorf("ConcatNestedNames() got = %v, want %v", tt.j, tt.want)
			}
		})
	}
}
