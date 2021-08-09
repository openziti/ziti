package tunnel

import (
	"encoding/json"
	"github.com/openziti/edge/health"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/sirupsen/logrus"
	"io"
	"net"
)

type HostingContext interface {
	ServiceName() string
	ListenOptions() *ziti.ListenOptions
	Dial(options map[string]interface{}) (net.Conn, bool, error)
	GetHealthChecks() []health.CheckDefinition
	GetInitialHealthState() (ziti.Precedence, uint16)
	OnClose()
	SetCloseCallback(func())
}

type HostControl interface {
	io.Closer
	UpdateCost(cost uint16) error
	UpdatePrecedence(precedence edge.Precedence) error
	UpdateCostAndPrecedence(cost uint16, precedence edge.Precedence) error
	SendHealthEvent(pass bool) error
}

type FabricProvider interface {
	PrepForUse(serviceId string)
	GetCurrentIdentity() (*edge.CurrentIdentity, error)
	TunnelService(service Service, identity string, conn net.Conn, halfClose bool, appInfo []byte) error
	HostService(hostCtx HostingContext) (HostControl, error)
}

func AppDataToMap(appData []byte) (map[string]interface{}, error) {
	result := map[string]interface{}{}
	if len(appData) != 0 {
		if err := json.Unmarshal(appData, &result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func NewContextProvider(context ziti.Context) FabricProvider {
	return &contextProvider{
		Context: context,
	}
}

type contextProvider struct {
	ziti.Context
}

func (self *contextProvider) PrepForUse(serviceId string) {
	if _, err := self.Context.GetSession(serviceId); err != nil {
		logrus.WithError(err).Error("failed to acquire network session")
	} else {
		logrus.Debug("acquired network session")
	}
}

func (self *contextProvider) TunnelService(service Service, identity string, conn net.Conn, halfClose bool, appData []byte) error {
	options := &ziti.DialOptions{
		ConnectTimeout: service.GetDialTimeout(),
		AppData:        appData,
		Identity:       identity,
	}

	zitiConn, err := self.Context.DialWithOptions(service.GetName(), options)
	if err != nil {
		return err
	}

	Run(zitiConn, conn, halfClose)
	return nil
}

func (self *contextProvider) HostService(hostCtx HostingContext) (HostControl, error) {
	logger := logrus.WithField("service", hostCtx.ServiceName())
	listener, err := self.Context.ListenWithOptions(hostCtx.ServiceName(), hostCtx.ListenOptions())
	if err != nil {
		logger.WithError(err).Error("error listening for service")
		return nil, err
	}

	go self.accept(listener, hostCtx)

	return listener, nil
}

func (self *contextProvider) accept(listener edge.Listener, hostCtx HostingContext) {
	defer hostCtx.OnClose()

	logger := logrus.WithField("service", hostCtx.ServiceName())
	for {
		logger.Info("hosting service, waiting for connections")
		conn, err := listener.AcceptEdge()
		if err != nil {
			logger.WithError(err).Error("closing listener for service")
			return
		}

		options, err := AppDataToMap(conn.GetAppData())
		if err != nil {
			logger.WithError(err).Error("dial failed")
			conn.CompleteAcceptFailed(err)
			if closeErr := conn.Close(); closeErr != nil {
				logger.WithError(closeErr).Error("close of ziti connection failed")
			}
			continue
		}

		externalConn, halfClose, err := hostCtx.Dial(options)
		if err != nil {
			logger.WithError(err).Error("dial failed")
			conn.CompleteAcceptFailed(err)
			if closeErr := conn.Close(); closeErr != nil {
				logger.WithError(closeErr).Error("close of ziti connection failed")
			}
			continue
		}

		log.Infof("successful connection %v->%v", conn.LocalAddr(), conn.RemoteAddr())

		if err := conn.CompleteAcceptSuccess(); err != nil {
			logger.WithError(err).Error("complete accept success failed")

			if closeErr := conn.Close(); closeErr != nil {
				logger.WithError(closeErr).Error("close of ziti connection failed")
			}

			if closeErr := externalConn.Close(); closeErr != nil {
				logger.WithError(closeErr).Error("close of external connection failed")
			}
			continue
		}

		go Run(conn, externalConn, halfClose)
	}
}
