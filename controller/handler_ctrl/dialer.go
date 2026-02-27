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

package handler_ctrl

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/transport/v2"
	"github.com/openziti/ziti/v2/common/ctrlchan"
	"github.com/openziti/ziti/v2/controller/config"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/sirupsen/logrus"
)

type CtrlDialer struct {
	config       *config.CtrlDialerConfig
	network      *network.Network
	ctrlAccepter *CtrlAccepter
	ctrlId       *identity.TokenId
	headers      map[int32][]byte
	closeNotify  <-chan struct{}
	ctx          context.Context
	ctxCancel    context.CancelFunc
	dialing      sync.Map // routerId -> struct{}, tracks in-progress dial attempts
}

func NewCtrlDialer(
	config *config.CtrlDialerConfig,
	network *network.Network,
	ctrlAccepter *CtrlAccepter,
	ctrlId *identity.TokenId,
	headers map[int32][]byte,
	closeNotify <-chan struct{},
) *CtrlDialer {
	return &CtrlDialer{
		config:       config,
		network:      network,
		ctrlAccepter: ctrlAccepter,
		ctrlId:       ctrlId,
		headers:      headers,
		closeNotify:  closeNotify,
	}
}

func (self *CtrlDialer) Run() {
	self.ctx, self.ctxCancel = context.WithCancel(context.Background())
	defer self.ctxCancel()

	log := pfxlog.Logger().WithField("component", "ctrlDialer")
	log.WithField("scanInterval", self.config.ScanInterval).
		WithField("dialDelay", self.config.DialDelay).
		WithField("groups", self.config.Groups).
		Info("starting ctrl channel dialer")

	ticker := time.NewTicker(self.config.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			self.scan()
		case <-self.closeNotify:
			log.Info("stopping ctrl channel dialer")
			return
		}
	}
}

func (self *CtrlDialer) scan() {
	log := pfxlog.Logger().WithField("component", "ctrlDialer")

	routers, err := self.network.Router.BaseList("limit none")
	if err != nil {
		log.WithError(err).Error("error listing routers for ctrl dialer scan")
		return
	}

	for _, router := range routers.Entities {
		self.checkDialersFor(router.Id, router)
	}
}

func (self *CtrlDialer) checkDialersFor(routerId string, router *model.Router) {
	for _, address := range self.getRouterIdEndpointsNeedingDial(routerId, router) {
		if _, alreadyDialing := self.dialing.LoadOrStore(routerId, struct{}{}); !alreadyDialing {
			go self.dialWithBackoff(routerId, address)
			break // one dial per router per scan
		}
	}
}

func (self *CtrlDialer) getRouterIdEndpointsNeedingDial(routerId string, router *model.Router) []string {
	log := pfxlog.Logger().WithField("routerId", routerId)
	if self.network.GetConnectedRouter(routerId) != nil {
		return nil
	}

	if router == nil {
		var err error
		router, err = self.network.Router.BaseLoad(routerId)
		if err != nil {
			log.WithError(err).Error("error loading router")
			return nil
		}
	}

	if router.Disabled {
		return nil
	}

	var results []string
	for address, groups := range router.CtrlChanListeners {
		if self.groupsMatch(groups) {
			results = append(results, address)
		}
	}

	return results
}

func (self *CtrlDialer) groupsMatch(routerGroups []string) bool {
	if len(routerGroups) == 0 {
		routerGroups = []string{"default"}
	}
	for _, rg := range routerGroups {
		for _, cg := range self.config.Groups {
			if rg == cg {
				return true
			}
		}
	}
	return false
}

func (self *CtrlDialer) dialWithBackoff(routerId, address string) {
	defer self.dialing.Delete(routerId)

	log := pfxlog.Logger().WithField("component", "ctrlDialer").
		WithField("routerId", routerId).
		WithField("address", address)

	addr, err := transport.ParseAddress(address)
	if err != nil {
		log.WithError(err).Error("error parsing ctrl chan listener address")
		return
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = self.config.DialDelay
	expBackoff.MaxInterval = 5 * time.Minute
	expBackoff.MaxElapsedTime = 100 * 365 * 24 * time.Hour

	log.Info("starting dial attempts to router ctrl channel listener")

	operation := func() error {
		// check if the endpoint still needs to be dialed
		endpoints := self.getRouterIdEndpointsNeedingDial(routerId, nil)
		if !slices.Contains(endpoints, address) {
			return backoff.Permanent(errors.New("endpoint no longer needs dial"))
		}

		log.Info("dialing router")
		if err = self.dial(routerId, addr, log); err != nil {
			log.WithError(err).Warn("dial attempt failed, will retry")
			return err
		}
		return nil
	}

	if err := backoff.Retry(operation, backoff.WithContext(expBackoff, self.ctx)); err != nil {
		log.WithError(err).Warn("stopped dial attempts to router ctrl channel listener")
	} else {
		log.Info("successfully connected to router ctrl channel")
	}
}

func (self *CtrlDialer) dial(routerId string, addr transport.Address, log *logrus.Entry) error {
	dialer := channel.NewClassicDialer(channel.DialerConfig{
		Identity: self.ctrlId,
		Endpoint: addr,
		Headers:  self.headers,
		TransportConfig: transport.Configuration{
			"protocol": "ziti-ctrl",
		},
	})

	firstDialHeaders := make(channel.Headers, 3)
	firstDialHeaders.PutBoolHeader(channel.IsGroupedHeader, true)
	firstDialHeaders.PutStringHeader(channel.TypeHeader, ctrlchan.ChannelTypeDefault)
	firstDialHeaders.PutBoolHeader(channel.IsFirstGroupConnection, true)

	underlay, err := dialer.CreateWithHeaders(self.ctrlAccepter.options.ConnectTimeout, firstDialHeaders)
	if err != nil {
		return err
	}

	listenerCtrlChan := ctrlchan.NewListenerCtrlChannel()

	multiConfig := &channel.MultiChannelConfig{
		LogicalName:     "ctrl/" + underlay.Id(),
		Options:         self.ctrlAccepter.options,
		UnderlayHandler: listenerCtrlChan,
		BindHandler: channel.BindHandlerF(func(binding channel.Binding) error {
			binding.AddCloseHandler(channel.CloseHandlerF(func(ch channel.Channel) {
				time.AfterFunc(time.Second, func() {
					self.checkDialersFor(routerId, nil)
				})
			}))
			return self.ctrlAccepter.Bind(binding)
		}),
		Underlay: underlay,
	}

	if _, err = channel.NewMultiChannel(multiConfig); err != nil {
		if closeErr := underlay.Close(); closeErr != nil {
			log.WithError(closeErr).Error("error closing underlay after multi channel creation failure")
		}
		return err
	}

	return nil
}
