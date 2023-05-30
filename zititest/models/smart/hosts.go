/*
	Copyright 2020 NetFoundry Inc.

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

package main

import "github.com/openziti/fablab/kernel/model"

func newHostsFactory() model.Factory {
	return &hostsFactory{}
}

func (_ *hostsFactory) Build(m *model.Model) error {
	for _, host := range m.SelectHosts("*") {
		if host.InstanceType == "" {
			host.InstanceType = "t2.micro"
		}
	}

	instanceType, found := m.GetStringVariable("instance_type")
	if found {
		for _, host := range m.SelectHosts("*") {
			host.InstanceType = instanceType
		}
	}

	instanceResourceType, found := m.GetStringVariable("instance_resource_type")
	if found {
		for _, host := range m.SelectHosts("*") {
			host.InstanceResourceType = instanceResourceType
		}
	}

	spotPrice, found := m.GetStringVariable("spot_price")
	if found {
		for _, host := range m.SelectHosts("*") {
			host.SpotPrice = spotPrice
		}
	}

	spotType, found := m.GetStringVariable("spot_type")
	if found {
		for _, host := range m.SelectHosts("*") {
			host.SpotType = spotType
		}
	}

	return nil
}

type hostsFactory struct{}
