package entities

import (
	"fmt"
	"github.com/netfoundry/ziti-sdk-golang/ziti/edge"
)

const (
	ClientConfigV1 = "ziti-tunneler-client.v1"
	ServerConfigV1 = "ziti-tunneler-server.v1"
)

type ServiceConfig struct {
	Protocol string
	Hostname string
	Port     int
}

func (s *ServiceConfig) String() string {
	return fmt.Sprintf("%v:%v:%v", s.Protocol, s.Hostname, s.Port)
}

type Service struct {
	edge.Service
	ClientConfig *ServiceConfig
	ServerConfig *ServiceConfig
}
