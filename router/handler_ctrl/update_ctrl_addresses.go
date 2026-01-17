package handler_ctrl

import (
	"maps"
	"slices"
	"sync/atomic"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/common/pb/ctrl_pb"
	"github.com/openziti/ziti/router/env"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

type updateCtrlAddressesHandler struct {
	env                    env.RouterEnv
	currentVersion         atomic.Uint64
	waitingForLeaderUpdate atomic.Bool
}

func (handler *updateCtrlAddressesHandler) NotifyIndexReset() {
	handler.currentVersion.Store(0)
}

func (handler *updateCtrlAddressesHandler) ContentType() int32 {
	return int32(ctrl_pb.ContentType_UpdateCtrlAddressesType)
}

func (handler *updateCtrlAddressesHandler) HandleReceive(msg *channel.Message, ch channel.Channel) {
	log := pfxlog.ContextLogger(ch.Label()).Entry
	upd := &ctrl_pb.UpdateCtrlAddresses{}
	if err := proto.Unmarshal(msg.Body, upd); err != nil {
		log.WithError(err).Error("error unmarshalling")
		return
	}

	log = log.WithFields(logrus.Fields{
		"endpoints":     upd.Addresses,
		"localVersion":  handler.currentVersion.Load(),
		"remoteVersion": upd.Index,
		"isLeader":      upd.IsLeader,
		"ctrlId":        ch.Id(),
	})

	log.Info("update ctrl endpoints message received")

	if upd.IsLeader {
		handler.waitingForLeaderUpdate.Store(false)
	}

	if handler.currentVersion.Load() == 0 || handler.currentVersion.Load() < upd.Index {
		if len(upd.Addresses) == 0 {
			log.Info("ctrl list is empty, ignoring and requesting latest set from leader")
			go handler.requestCtrlListFromLeader()
			return
		}

		s := map[string]struct{}{}
		for _, addr := range upd.Addresses {
			s[addr] = struct{}{}
		}

		ctrls := handler.env.GetNetworkControllers()
		endpoints := ctrls.GetAll()

		hasRemovals := false
		for _, ctrl := range endpoints {
			if _, ok := s[ctrl.Address()]; !ok {
				hasRemovals = true
			}
		}

		if !upd.IsLeader && hasRemovals {
			log.Info("updated ctrl list is not from leader, using only additions and requesting latest set from leader")
			if !handler.waitingForLeaderUpdate.Load() {
				go handler.requestCtrlListFromLeader()
			}
			for _, ctrl := range endpoints {
				s[ctrl.Address()] = struct{}{}
			}
			upd.Addresses = slices.Collect(maps.Keys(s))
		}

		if len(upd.Addresses) > 0 {
			log.Info("updating to newer controller endpoints")
			handler.env.UpdateCtrlEndpoints(upd.Addresses)
			handler.currentVersion.Store(upd.Index)
		} else {
			log.Info("ignoring empty controller endpoint list")
		}
	} else {
		log.Info("ignoring outdated controller endpoint list")
	}

	if upd.IsLeader && (handler.currentVersion.Load() == 0 || handler.currentVersion.Load() <= upd.Index) {
		log.Infof("updating current leader to %s", ch.Id())
		handler.env.UpdateLeader(ch.Id())
	}
}

func (handler *updateCtrlAddressesHandler) requestCtrlListFromLeader() {
	if !handler.waitingForLeaderUpdate.CompareAndSwap(false, true) {
		return
	}

	for handler.waitingForLeaderUpdate.Load() {
		log := pfxlog.Logger().Entry
		leader := handler.env.GetNetworkControllers().GetLeader()
		if leader == nil {
			log.Info("no leader, unable to request latest cluster members from leader")
		} else {
			log = log.WithField("ctrlId", leader.Channel().Id())

			if !leader.IsConnected() {
				log.Info("not connected to leader, unable to request latest cluster members from leader")
			} else {
				msg := channel.NewMessage(int32(ctrl_pb.ContentType_RequestClusterMembers), nil)
				if err := leader.Channel().Send(msg); err != nil {
					log.WithError(err).Error("error sending request for latest cluster members to leader")
				}
			}
		}

		time.Sleep(30 * time.Second)
	}
}

func newUpdateCtrlAddressesHandler(env env.RouterEnv) channel.TypedReceiveHandler {
	result := &updateCtrlAddressesHandler{
		env: env,
	}

	env.GetIndexWatchers().AddIndexResetWatcher(result.NotifyIndexReset)

	return result
}
