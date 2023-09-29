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
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/ziti/router/forwarder"
	"os"
	"os/signal"
	"syscall"
)

type routerMonitor struct {
	forwarder   *forwarder.Forwarder
	closeNotify <-chan struct{}
}

func newRouterMonitor(forwarder *forwarder.Forwarder, closeNotify <-chan struct{}) *routerMonitor {
	return &routerMonitor{forwarder: forwarder, closeNotify: closeNotify}
}

func (routerMonitor *routerMonitor) Monitor() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGUSR1)
	for {
		select {
		case <-signalChan:
			pfxlog.Logger().Info("\n" + routerMonitor.forwarder.Debug())
		case <-routerMonitor.closeNotify:
			return
		}
	}
}
