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

package db

import (
	"encoding/json"
	"fmt"
	"github.com/openziti/storage/boltz"
	"net"
)

const (
	FieldInterfaces               = "interfaces"
	FieldInterfaceHardwareAddress = "addr"
	FieldInterfaceMtu             = "mtu"
	FieldInterfaceIndex           = "index"
	FieldInterfaceFlags           = "flags"
	FieldInterfaceAddresses       = "addrs"
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

func (self *Interface) MarshalJSON() ([]byte, error) {
	result := map[string]any{}
	result["name"] = self.Name
	result["hardwareAddress"] = self.HardwareAddress
	result["mtu"] = self.MTU
	result["index"] = self.Index
	result["isBroadcast"] = self.IsBroadcast()
	result["isLoopback"] = self.IsLoopback()
	result["isMulticast"] = self.IsMulticast()
	result["isRunning"] = self.IsRunning()
	result["isUp"] = self.IsUp()
	result["addresses"] = self.Addresses

	return json.Marshal(result)
}

func (self *Interface) toBoltMap() map[string]any {
	result := map[string]any{}
	result[FieldName] = self.Name
	result[FieldInterfaceHardwareAddress] = self.HardwareAddress
	result[FieldInterfaceMtu] = self.MTU
	result[FieldInterfaceIndex] = self.Index
	result[FieldInterfaceFlags] = int64(self.Flags)

	addresses := make([]any, len(self.Addresses))
	for i, address := range self.Addresses {
		addresses[i] = address
	}
	result[FieldInterfaceAddresses] = addresses
	return result
}

func (self *Interface) FillEntity(bucket *boltz.TypedBucket) {
	self.Name = bucket.GetStringOrError(FieldName)
	self.HardwareAddress = bucket.GetStringOrError(FieldInterfaceHardwareAddress)
	self.MTU = bucket.GetInt64WithDefault(FieldInterfaceMtu, 0)
	self.Index = bucket.GetInt64WithDefault(FieldInterfaceIndex, 0)
	self.Flags = uint64(bucket.GetInt64WithDefault(FieldInterfaceFlags, 0))
	addresses := bucket.GetList(FieldInterfaceAddresses)
	for _, address := range addresses {
		self.Addresses = append(self.Addresses, address.(string))
	}
}

func loadInterfaces(bucket *boltz.TypedBucket) []*Interface {
	var result []*Interface
	if interfacesBucket := bucket.GetBucket(FieldInterfaces); interfacesBucket != nil {
		err := interfacesBucket.ForEachTypedBucket(func(_ string, intfBucket *boltz.TypedBucket) error {
			intf := &Interface{}
			intf.FillEntity(intfBucket)
			result = append(result, intf)
			return bucket.Err
		})

		if err != nil {
			bucket.SetError(err)
		}
	}
	return result
}

func storeInterfaces(interfaces []*Interface, ctx *boltz.PersistContext) {
	if ctx.ProceedWithSet(FieldInterfaces) {
		m := map[string]any{}
		for idx, v := range interfaces {
			m[fmt.Sprintf("%d.%s", idx, v.Name)] = v.toBoltMap()
		}
		ctx.SetMap(FieldInterfaces, m)
	}
}
