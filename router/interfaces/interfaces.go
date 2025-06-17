/*
	(c) Copyright NetFoundry Inc.

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

package interfaces

import (
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/channel/v4/protobufs"
	"github.com/openziti/foundation/v2/stringz"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"net"
	"sort"
	"time"
)

func StartInterfaceReporter(ctrls env.NetworkControllers, closeNotify <-chan struct{}, config env.InterfaceDiscoveryConfig) {
	if config.Disabled {
		return
	}

	reporter := &InterfaceReporter{
		closeNotify:       closeNotify,
		checkInterval:     config.CheckInterval,
		minReportInterval: config.MinReportInterval,
		ctrls:             ctrls,
	}

	go reporter.run()
}

type InterfaceReporter struct {
	closeNotify       <-chan struct{}
	checkInterval     time.Duration
	minReportInterval time.Duration
	currentInterfaces []*ctrl_pb.Interface
	lastReportTime    time.Time
	ctrls             env.NetworkControllers
}

func (self *InterfaceReporter) run() {
	for {
		self.report()

		select {
		case <-time.After(self.checkInterval): // using this instead of ticker, as we want to wait the time, not have ticks pile up
		case <-self.closeNotify:
			return
		}
	}
}

func (self *InterfaceReporter) report() {
	interfaces, err := net.Interfaces()
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to list network interfaces")
		return
	}

	var reportInterfaces []*ctrl_pb.Interface
	for _, iface := range interfaces {
		v := &ctrl_pb.Interface{
			Name:            iface.Name,
			HardwareAddress: iface.HardwareAddr.String(),
			Mtu:             int64(iface.MTU),
			Index:           int64(iface.Index),
			Flags:           uint64(iface.Flags),
		}

		addrs, err := iface.Addrs()
		if err != nil {
			pfxlog.Logger().WithError(err).WithField("interface", iface.Name).Error("failed to list interface addresses")
		}

		for _, addr := range addrs {
			v.Addresses = append(v.Addresses, addr.String())
		}
		sort.Strings(v.Addresses)
		reportInterfaces = append(reportInterfaces, v)
	}

	sort.Slice(reportInterfaces, func(i, j int) bool {
		return reportInterfaces[i].Name < reportInterfaces[j].Name
	})

	changed := self.DidInterfacesChange(reportInterfaces)
	if changed || time.Since(self.lastReportTime) >= self.minReportInterval {
		self.reportUpdatedInterfaces(reportInterfaces, changed)
	}
}

func (self *InterfaceReporter) DidInterfacesChange(interfaces []*ctrl_pb.Interface) bool {
	if len(interfaces) != len(self.currentInterfaces) {
		return true
	}

	for idx, iface := range self.currentInterfaces {
		if self.DidInterfaceChange(iface, interfaces[idx]) {
			return true
		}
	}
	return false
}

func (self *InterfaceReporter) DidInterfaceChange(a, b *ctrl_pb.Interface) bool {
	changed := a.Name != b.Name ||
		a.Index != b.Index ||
		a.Flags != b.Flags ||
		a.Mtu != b.Mtu ||
		a.HardwareAddress != b.HardwareAddress

	if changed {
		return true
	}

	return !stringz.EqualSlices(a.Addresses, b.Addresses)
}

func (self *InterfaceReporter) reportUpdatedInterfaces(interfaces []*ctrl_pb.Interface, changed bool) {
	log := pfxlog.Logger()
	req := &ctrl_pb.RouterInterfacesUpdate{
		Interfaces: interfaces,
	}

	if changed {
		log.Infof("reporting %d interfaces because interfaces changed", len(req.Interfaces))
	} else {
		log.Infof("reporting %d interfaces because min report interval has passed", len(req.Interfaces))
	}

	ctrlCh := self.ctrls.GetModelUpdateCtrlChannel()
	if ctrlCh == nil {
		log.Error("no controller available, unable to report updated interfaces")
		self.ctrls.AnyValidCtrlChannel() // wait for a controller to become available again
		return
	}

	reply, err := protobufs.MarshalTyped(req).WithTimeout(self.ctrls.DefaultRequestTimeout()).SendForReply(ctrlCh)
	if err != nil {
		log.WithError(err).Error("failed to send update router interfaces request to controller")
		return
	}

	result := channel.UnmarshalResult(reply)
	if result.Success {
		// don't update current until a controller has been notified
		self.currentInterfaces = interfaces
		self.lastReportTime = time.Now()
		log.Info("router interfaces updated successfully on controller")
	} else {
		log.WithError(errors.New(result.Message)).Error("failed to update router interfaces on controller")
	}
}
