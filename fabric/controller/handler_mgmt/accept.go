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

package handler_mgmt

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/netfoundry/ziti-foundation/channel2"
)

type MgmtAccepter struct {
	listener channel2.UnderlayListener
	options  *channel2.Options
}

func NewMgmtAccepter(listener channel2.UnderlayListener,
	options *channel2.Options) *MgmtAccepter {
	return &MgmtAccepter{
		listener: listener,
		options:  options,
	}
}

func (mgmtAccepter *MgmtAccepter) Run() {
	log := pfxlog.Logger()
	log.Info("started")
	defer log.Warn("exited")

	for {
		ch, err := channel2.NewChannel("mgmt", mgmtAccepter.listener, mgmtAccepter.options)
		if err == nil {
			log.Debugf("accepted mgmt connection [%s]", ch.Id().Token)

		} else {
			log.Errorf("error accepting (%s)", err)
			if err.Error() == "closed" {
				return
			}
		}
	}
}