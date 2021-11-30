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

package routes

import (
	"encoding/json"
	"github.com/openziti/fabric/controller/api"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

var test = `
{
	"name" : "Foo",
	"terminatorStrategy" : "default",
	"configs" : [ "ssh-config", "ssh-server-config" ]
}
`

func Test_getFields(t *testing.T) {
	assert := require.New(t)
	test2 := map[string]interface{}{
		"roleAttributes":     []string{"foo", "bar"},
		"name":               "Foo",
		"terminatorStrategy": "default",
		"configs":            []string{"ssh-config", "ssh-server-config"},
		"tags": map[string]interface{}{
			"foo": "bar",
			"nested": map[string]interface{}{
				"go": true,
			},
		}}

	test2Bytes, err := json.Marshal(test2)
	assert.NoError(err)

	tests := []struct {
		name    string
		body    []byte
		want    api.JsonFields
		wantErr bool
	}{
		{
			name: "test",
			body: []byte(test),
			want: api.JsonFields{
				"name":               true,
				"terminatorStrategy": true,
				"configs":            true,
			},
			wantErr: false,
		},
		{
			name: "test2",
			body: test2Bytes,
			want: api.JsonFields{
				"name":               true,
				"terminatorStrategy": true,
				"roleAttributes":     true,
				"tags":               true,
				"configs":            true,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := api.GetFields(tt.body)
			got.FilterMaps("tags")
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
		j    api.JsonFields
		want api.JsonFields
	}{
		{"test",
			api.JsonFields{
				"Name":                  true,
				"This.Is.A.Longer.Name": true,
				"EgressRouter":          true,
				"Address":               false,
			},
			api.JsonFields{
				"Name":              true,
				"ThisIsALongerName": true,
				"EgressRouter":      true,
				"Address":           false,
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
