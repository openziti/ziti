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

package model

import (
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/controller/db"
	"net"
	"sort"
)

type Interface struct {
	Name            string   `json:"name"`
	HardwareAddress string   `json:"hardwareAddress"`
	MTU             int64    `json:"mtu"`
	Index           int64    `json:"index"`
	Flags           uint64   `json:"flags"`
	Addresses       []string `json:"addresses"`
}

func (self *Interface) IsUp() bool {
	return self.IsFlagSet(net.FlagUp)
}

func (self *Interface) IsRunning() bool {
	return self.IsFlagSet(net.FlagRunning)
}

func (self *Interface) IsLoopback() bool {
	return self.IsFlagSet(net.FlagLoopback)
}

func (self *Interface) IsBroadcast() bool {
	return self.IsFlagSet(net.FlagBroadcast)
}

func (self *Interface) IsMulticast() bool {
	return self.IsFlagSet(net.FlagMulticast)
}

func (self *Interface) IsFlagSet(f net.Flags) bool {
	return net.Flags(self.Flags)&f == f
}

func (entity *Interface) ToBolt() *db.Interface {
	return &db.Interface{
		Name:            entity.Name,
		HardwareAddress: entity.HardwareAddress,
		MTU:             entity.MTU,
		Index:           entity.Index,
		Flags:           entity.Flags,
		Addresses:       entity.Addresses,
	}
}

func InterfacesToBolt(val []*Interface) []*db.Interface {
	var result []*db.Interface
	for _, v := range val {
		result = append(result, v.ToBolt())
	}
	return result
}

func InterfaceFromBolt(entity *db.Interface) *Interface {
	return &Interface{
		Name:            entity.Name,
		HardwareAddress: entity.HardwareAddress,
		MTU:             entity.MTU,
		Index:           entity.Index,
		Flags:           entity.Flags,
		Addresses:       entity.Addresses,
	}
}

func InterfacesFromBolt(m []*db.Interface) []*Interface {
	var result []*Interface
	for _, v := range m {
		result = append(result, InterfaceFromBolt(v))
	}
	return result
}

func didInterfacesChange(old, new []*Interface) bool {
	sort.Slice(old, func(i, j int) bool {
		return old[i].Name < old[j].Name
	})

	sort.Slice(new, func(i, j int) bool {
		return new[i].Name < new[j].Name
	})

	if len(old) != len(new) {
		return true
	}

	for idx, iface := range old {
		if didInterfaceChange(iface, new[idx]) {
			return true
		}
	}
	return false
}

func didInterfaceChange(a, b *Interface) bool {
	changed := a.Name != b.Name ||
		a.Index != b.Index ||
		a.Flags != b.Flags ||
		a.MTU != b.MTU ||
		a.HardwareAddress != b.HardwareAddress

	if changed {
		return true
	}

	sort.Strings(a.Addresses)
	sort.Strings(b.Addresses)

	return !stringz.EqualSlices(a.Addresses, b.Addresses)
}
