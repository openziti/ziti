/*
	Copyright NetFoundry, Inc.

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

// Package rest_util provides helper functions to generate a client for the Ziti Fabric REST APIs. It is a meat and
// potato API that is meant to be consumed by higher level implementations (e.g. CLIs).
package rest_util

import (
	"github.com/go-openapi/runtime"
	httptransport "github.com/go-openapi/runtime/client"
	fabric_rest_client "github.com/openziti/fabric/rest_client"
	"github.com/openziti/foundation/identity/identity"
	"net/url"
)

// TransportConfig provides information about how to access a REST API
type TransportConfig interface {
	// GetHost returns the host and port
	GetHost() string
	// GetBasePath returns the path to the REST API
	GetBasePath() string
	// GetSchemes returns the schemes (such as http or https) by which the REST API can be accessed
	GetSchemes() []string
}

// TransportFactory will provide a runtime.ClientTransport given a TransportConfig
type TransportFactory interface {
	New(config TransportConfig) (runtime.ClientTransport, error)
}

// TransportFactoryF is a func version of TransportFactory
type TransportFactoryF func(config TransportConfig) (runtime.ClientTransport, error)

func (self TransportFactoryF) New(config TransportConfig) (runtime.ClientTransport, error) {
	return self(config)
}

// NewFabricClientWithIdentity will return a ZitiFabric REST client given an identity and an apiAddress. This
// assumes that the fabric is running without the edge and is using cert based authentication. If the fabric is
// running with the edge, an appropriate edge based TransportFactory will need to be provided
func NewFabricClientWithIdentity(identity identity.Identity, apiAddress string) (*fabric_rest_client.ZitiFabric, error) {
	factory := &IdentityTransportFactory{
		identity: identity,
	}
	return NewFabricClient(factory, apiAddress)
}

// NewFabricClient will return a ZitiFabric REST client given a transport factory and api address
func NewFabricClient(transportFactory TransportFactory, apiAddress string) (*fabric_rest_client.ZitiFabric, error) {
	transportConfig := NewZitiFabricTransportConfig(apiAddress)
	transport, err := transportFactory.New(transportConfig)
	if err != nil {
		return nil, err
	}
	return fabric_rest_client.New(transport, nil), nil
}

// TransportConfigImpl provides a default implementation of TransportConfig
type TransportConfigImpl struct {
	ApiAddress string
	BasePath   string
	Schemes    []string
}

func (self *TransportConfigImpl) GetHost() string {
	return self.ApiAddress
}

func (self *TransportConfigImpl) GetBasePath() string {
	return self.BasePath
}

func (self *TransportConfigImpl) GetSchemes() []string {
	return self.Schemes
}

// NewZitiFabricTransportConfig will create a TransportConfig using the given API address and
// the default ziti fabric rest client values for base path and schema.
func NewZitiFabricTransportConfig(apiAddress string) TransportConfig {
	return &TransportConfigImpl{
		ApiAddress: apiAddress,
		BasePath:   fabric_rest_client.DefaultBasePath,
		Schemes:    fabric_rest_client.DefaultSchemes,
	}
}

// IdentityTransportFactory will use the client TLSConfig provided by the given identity to configure
// a ClientTransport
type IdentityTransportFactory struct {
	identity identity.Identity
}

func (self *IdentityTransportFactory) New(config TransportConfig) (runtime.ClientTransport, error) {
	ctrlUrl, err := url.Parse(config.GetHost())
	if err != nil {
		return nil, err
	}

	tlsConfig := self.identity.ClientTLSConfig()
	httpClient, err := NewHttpClientWithTlsConfig(tlsConfig)
	if err != nil {
		return nil, err
	}

	clientRuntime := httptransport.NewWithClient(ctrlUrl.Host, config.GetBasePath(), config.GetSchemes(), httpClient)
	return clientRuntime, nil
}
