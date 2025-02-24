package xgress_edge_tunnel

import (
	"errors"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v3"
	"github.com/openziti/foundation/v2/concurrenz"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/config"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/router"
	"github.com/openziti/ziti/router/env"
	"github.com/openziti/ziti/router/state"
	"github.com/openziti/ziti/router/xgress"
	"github.com/openziti/ziti/router/xgress_edge_tunnel_v2"
	"os"
	"time"
)

type FactoryWrapper struct {
	env          env.RouterEnv
	routerConfig *router.Config
	stateManager state.Manager
	initDone     chan struct{}
	delegate     concurrenz.AtomicValue[XrctrlFactory]

	listenerOptions chan xgress.OptionsData
	listenerArgs    chan listenArgs
}

func (self *FactoryWrapper) LoadConfig(map[interface{}]interface{}) error {
	// both v1/v2 currently have empty LoadConfig methods. Will need to update this if that changes.
	return nil
}

func (self *FactoryWrapper) BindChannel(binding channel.Binding) error {
	// v1 bindings
	binding.AddReceiveHandlerF(int32(edge_ctrl_pb.ContentType_ServiceListType), self.handleV1ServiceListType)
	binding.AddReceiveHandlerF(int32(edge_ctrl_pb.ContentType_CreateTunnelTerminatorResponseType), self.handleV1CreateTunnelTerminatorResponse)

	// v2 bindings
	binding.AddReceiveHandlerF(int32(edge_ctrl_pb.ContentType_CreateTunnelTerminatorResponseV2Type), self.handleV2CreateTunnelTerminatorResponse)

	return nil
}

func (self *FactoryWrapper) handleV1ServiceListType(msg *channel.Message, ch channel.Channel) {
	if delegate := self.delegate.Load(); delegate != nil {
		if v1, ok := delegate.(*Factory); ok {
			v1.serviceListHandler.HandleReceive(msg, ch)
		}
	}
}

func (self *FactoryWrapper) handleV1CreateTunnelTerminatorResponse(msg *channel.Message, ch channel.Channel) {
	if delegate := self.delegate.Load(); delegate != nil {
		if v1, ok := delegate.(*Factory); ok {
			v1.tunneler.fabricProvider.HandleTunnelResponse(msg, ch)
		}
	}
}

func (self *FactoryWrapper) handleV2CreateTunnelTerminatorResponse(msg *channel.Message, ch channel.Channel) {
	if delegate := self.delegate.Load(); delegate != nil {
		if v2, ok := delegate.(*xgress_edge_tunnel_v2.Factory); ok {
			v2.HandleCreateTunnelTerminatorResponse(msg, ch)
		}
	}
}

func (self *FactoryWrapper) Enabled() bool {
	return true
}

func (self *FactoryWrapper) Run(env.RouterEnv) error {
	// we'll call run when initialization is complete
	return nil
}

func (self *FactoryWrapper) NotifyOfReconnect(ch channel.Channel) {
	if delegate := self.delegate.Load(); delegate != nil {
		delegate.NotifyOfReconnect(ch)
	}
}

func NewFactoryWrapper(env env.RouterEnv, routerConfig *router.Config, stateManager state.Manager) XrctrlFactory {
	wrapper := &FactoryWrapper{
		env:             env,
		routerConfig:    routerConfig,
		stateManager:    stateManager,
		initDone:        make(chan struct{}),
		listenerOptions: make(chan xgress.OptionsData, 5),
		listenerArgs:    make(chan listenArgs, 5),
	}

	env.GetRouterDataModelEnabledConfig().AddListener(config.ListenerFunc[bool](func(init bool, old bool, new bool) {
		if !init && old != new {
			if new {
				pfxlog.Logger().Error("controller has moved from legacy mode to router data model mode, restarting so the router can work with router data model")
			} else {
				pfxlog.Logger().Error("controller no longer supports the router data model, restarting so the router can work in legacy mode")
			}
			os.Exit(0)
		}
	}))

	go func() {
		defer close(wrapper.initDone)

		log := pfxlog.Logger()

		select {
		case <-env.GetRouterDataModelEnabledConfig().GetInitNotifyChannel():
		case <-env.GetCloseNotify():
			return
		}

		var factory XrctrlFactory
		if env.GetRouterDataModelEnabledConfig().Load() {
			log.Info("router data model enabled, using xgress_edge_tunnel_v2")
			factory = xgress_edge_tunnel_v2.NewFactory(env, routerConfig, stateManager)
		} else {
			log.Info("router data model NOT enabled, using xgress_edge_tunnel")
			factory = NewV1Factory(env, routerConfig, stateManager)
		}

		wrapper.delegate.Store(factory)
		xgress.GlobalRegistry().Register(common.TunnelBinding, factory)

		done := false
		for !done {
			select {
			case options := <-wrapper.listenerOptions:
				listener, err := factory.CreateListener(options)
				if err != nil {
					log.WithField("binding", common.TunnelBinding).WithError(err).Fatal("error creating listener")
					return
				}

				select {
				case args := <-wrapper.listenerArgs:
					args.delegate.delegate.Store(listener)
					err = listener.Listen(args.address, args.bindHandler)
					if err != nil {
						log.WithField("binding", common.TunnelBinding).WithError(err).Fatal("error starting listener")
						return
					}
				case <-time.After(time.Second * 5):
					log.WithField("binding", common.TunnelBinding).WithError(err).Fatal("timeout waiting for start to be called on listener")
					return
				}
			default:
				done = true
			}

		}

		_ = env.GetNetworkControllers().AnyValidCtrlChannel()
		if err := factory.Run(env); err != nil {
			log.WithError(err).Fatal("error starting")
		}
	}()

	return wrapper
}

func (self *FactoryWrapper) CreateListener(optionsData xgress.OptionsData) (xgress.Listener, error) {
	self.listenerOptions <- optionsData
	return &delegatingListener{
		factory: self,
		options: optionsData,
	}, nil
}

func (self *FactoryWrapper) CreateDialer(optionsData xgress.OptionsData) (xgress.Dialer, error) {
	// wait till delegate is created. Once delegate is created, we should also be calling CreateDialer on
	// the delegate directly, as the factory will get replaced in the registry
	start := time.Now()
	for {
		if delegate := self.delegate.Load(); delegate != nil {
			return delegate.CreateDialer(optionsData)
		}
		if time.Since(start) > 2*time.Minute {
			return nil, errors.New("timeout waiting for dialer to be created")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type delegatingListener struct {
	factory  *FactoryWrapper
	options  xgress.OptionsData
	delegate concurrenz.AtomicValue[xgress.Listener]
}

type listenArgs struct {
	address     string
	bindHandler xgress.BindHandler
	delegate    *delegatingListener
}

func (self *delegatingListener) Listen(address string, bindHandler xgress.BindHandler) error {
	self.factory.listenerArgs <- listenArgs{
		address:     address,
		bindHandler: bindHandler,
		delegate:    self,
	}
	return nil
}

func (self *delegatingListener) Close() error {
	if listener := self.delegate.Load(); listener != nil {
		return listener.Close()
	}
	return nil
}
