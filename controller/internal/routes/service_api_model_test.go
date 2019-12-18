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
	"github.com/netfoundry/ziti-edge/controller/model"
	"reflect"
	"testing"
)

func TestServiceApiCreate_ToModelService(t *testing.T) {
	type fields struct {
		EdgeRouterRoles []string
		Dns             *ServiceDnsApiPost
		Name            *string
		HostIds         []string
		Tags            map[string]interface{}
		EgressRouter    *string
		EndpointAddress *string
	}
	tests := []struct {
		name    string
		fields  fields
		want    *model.Service
		wantErr bool
	}{
		{name: "test all fields", fields: fields{
			EdgeRouterRoles: []string{"one", "two"},
			Dns: &ServiceDnsApiPost{
				Hostname: strPtr("foo"),
				Port:     uint16Ptr(1234),
			},
			Name:            strPtr("bar"),
			HostIds:         []string{"id1", "id2"},
			Tags:            map[string]interface{}{"hello": 1, "thing": "hi"},
			EgressRouter:    strPtr("001"),
			EndpointAddress: strPtr("tcp:localhost:8908"),
		}, want: &model.Service{
			BaseModelEntityImpl: model.BaseModelEntityImpl{
				Tags: map[string]interface{}{"hello": 1, "thing": "hi"},
			},
			Name:            "bar",
			DnsHostname:     "foo",
			DnsPort:         1234,
			EgressRouter:    "001",
			EndpointAddress: "tcp:localhost:8908",
			EdgeRouterRoles: []string{"one", "two"},
			HostIds:         []string{"id1", "id2"},
		}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiService := &ServiceApiCreate{
				EdgeRouterRoles: tt.fields.EdgeRouterRoles,
				Dns:             tt.fields.Dns,
				Name:            tt.fields.Name,
				HostIds:         tt.fields.HostIds,
				Tags:            tt.fields.Tags,
				EgressRouter:    tt.fields.EgressRouter,
				EndpointAddress: tt.fields.EndpointAddress,
			}
			got := apiService.ToModel()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToModelService() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServiceApiUpdate_ToModelService(t *testing.T) {
	type fields struct {
		Dns             *ServiceDnsApiPost
		Name            *string
		HostIds         []string
		Tags            map[string]interface{}
		EgressRouter    *string
		EndpointAddress *string
	}
	tests := []struct {
		name    string
		fields  fields
		want    *model.Service
		wantErr bool
	}{
		{name: "test all fields", fields: fields{
			Dns: &ServiceDnsApiPost{
				Hostname: strPtr("foo"),
				Port:     uint16Ptr(1234),
			},
			Name:            strPtr("bar"),
			Tags:            map[string]interface{}{"hello": 1, "thing": "hi"},
			EgressRouter:    strPtr("001"),
			EndpointAddress: strPtr("tcp:localhost:8908"),
		}, want: &model.Service{
			BaseModelEntityImpl: model.BaseModelEntityImpl{
				Tags: map[string]interface{}{"hello": 1, "thing": "hi"},
			},
			Name:            "bar",
			DnsHostname:     "foo",
			DnsPort:         1234,
			EgressRouter:    "001",
			EndpointAddress: "tcp:localhost:8908",
		}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiService := &ServiceApiUpdate{
				Dns:             tt.fields.Dns,
				Name:            tt.fields.Name,
				Tags:            tt.fields.Tags,
				EgressRouter:    tt.fields.EgressRouter,
				EndpointAddress: tt.fields.EndpointAddress,
			}
			got := apiService.ToModel("")
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ToModelService() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func strPtr(val string) *string {
	return &val
}

func uint16Ptr(val uint16) *uint16 {
	return &val
}
