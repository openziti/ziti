// +build !linux

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

package router

import (
	"errors"
	"net"
)

func AddLocalAddress(prefix *net.IPNet, ifName string) error {
	return errors.New("AddLocalAddress is not implemented on this operating system")
}

func RemoveLocalAddress(prefix *net.IPNet, ifName string) error {
	return errors.New("RemoveLocalAddress is not implemented on this operating system")
}

func AddPointToPointAddress(localIP net.IP, peerPrefix *net.IPNet, ifName string) error {
	return errors.New("AddPointToPointAddress is not implemented on this operating system")
}

func RemovePointToPointAddress(localIP net.IP, peerPrefix *net.IPNet, ifName string) error {
	return errors.New("RemovePointToPointAddress is not implemented on this operating system")
}
