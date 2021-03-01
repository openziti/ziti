package tunnel

import (
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/edge"
	"github.com/sirupsen/logrus"
	"io"
	"net"
)

type HostingContext interface {
	ServiceName() string
	ListenOptions() *ziti.ListenOptions
	Dial() (net.Conn, error)
	SupportHalfClose() bool
	OnClose()
}

type HostControl interface {
	io.Closer
	UpdateCost(cost uint16) error
	UpdatePrecedence(precedence edge.Precedence) error
	UpdateCostAndPrecedence(cost uint16, precedence edge.Precedence) error
}

type FabricProvider interface {
	PrepForUse(serviceId string) error
	GetCurrentIdentity() (*edge.CurrentIdentity, error)
	TunnelService(conn net.Conn, service string, halfClose bool) error
	HostService(hostCtx HostingContext) (HostControl, error)
}

func NewContextProvider(context ziti.Context) FabricProvider {
	return &contextProvider{
		Context: context,
	}
}

type contextProvider struct {
	ziti.Context
}

func (self *contextProvider) PrepForUse(serviceId string) error {
	_, err := self.Context.GetSession(serviceId)
	return err
}

func (self *contextProvider) TunnelService(conn net.Conn, service string, halfClose bool) error {
	zitiConn, err := self.Context.Dial(service)
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
		externalConn, err := hostCtx.Dial()
		if err != nil {
			logger.Error("dial failed")
			conn.CompleteAcceptFailed(err)
			if closeErr := conn.Close(); closeErr != nil {
				logger.WithError(closeErr).Error("close of ziti connection failed")
			}
			continue
		}

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

		go Run(conn, externalConn, hostCtx.SupportHalfClose())
	}
}
