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

package xweb

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/debugz"
	"io"
	"log"
	"net"
	"net/http"
)

type ContextKey string

const (
	WebHandlerContextKey = ContextKey("XWebHandlerContextKey")
	WebContextKey        = ContextKey("XWebContext")

	ZitiCtrlAddressHeader = "ziti-ctrl-address"
)

type XWebContext struct {
	BindPoint   *BindPoint
	WebListener *WebListener
	XWebConfig  *Config
}

type namedHttpServer struct {
	*http.Server
	ApiBindingList []string
	BindPoint      *BindPoint
	WebListener    *WebListener
	XWebConfig     *Config
}

func (s namedHttpServer) NewBaseContext(_ net.Listener) context.Context {
	xwebContext := &XWebContext{
		BindPoint:   s.BindPoint,
		WebListener: s.WebListener,
		XWebConfig:  s.XWebConfig,
	}
	ctx := context.Background()
	ctx = context.WithValue(ctx, WebContextKey, xwebContext)

	return ctx
}

// Server represents all of the http.Server's and http.Handler's necessary to run a single xweb.WebListener
type Server struct {
	httpServers       []*namedHttpServer
	logWriter         *io.PipeWriter
	options           *Options
	config            interface{}
	Handle            http.Handler
	OnHandlerPanic    func(writer http.ResponseWriter, request *http.Request, panicVal interface{})
	ParentWebListener *WebListener
}

// NewServer creates a new xweb.Server from an xweb.WebListener. All necessary http.Handler's will be created from the supplied
// DemuxFactory and WebHandlerFactoryRegistry.
func NewServer(webListener *WebListener, demuxFactory DemuxFactory, handlerFactoryRegistry WebHandlerFactoryRegistry, config *Config) (*Server, error) {
	logWriter := pfxlog.Logger().Writer()

	tlsConfig := webListener.Identity.ServerTLSConfig()
	tlsConfig.ClientAuth = tls.RequestClientCert
	tlsConfig.MinVersion = uint16(webListener.Options.MinTLSVersion)
	tlsConfig.MaxVersion = uint16(webListener.Options.MaxTLSVersion)

	server := &Server{
		logWriter:         logWriter,
		config:            &webListener,
		httpServers:       []*namedHttpServer{},
		ParentWebListener: webListener,
	}

	var webHandlers []WebHandler
	var apiBindingList []string

	for _, api := range webListener.APIs {
		if factory := handlerFactoryRegistry.Get(api.Binding()); factory != nil {
			if webHandler, err := factory.New(webListener, api.Options()); err != nil {
				pfxlog.Logger().Fatalf("encountered error building handler for api binding [%s]: %v", api.Binding(), err)
			} else {
				webHandlers = append(webHandlers, webHandler)
				apiBindingList = append(apiBindingList, api.binding)
			}
		} else {
			pfxlog.Logger().Fatalf("encountered api binding [%s] which has no associated factory registered", api.Binding())
		}
	}

	demuxWebHandler, err := demuxFactory.Build(webHandlers)

	if err != nil {
		return nil, fmt.Errorf("error creating server: %v", err)
	}

	for _, bindPoint := range webListener.BindPoints {
		namedServer := &namedHttpServer{
			ApiBindingList: apiBindingList,
			WebListener:    webListener,
			BindPoint:      bindPoint,
			XWebConfig:     config,
			Server: &http.Server{
				Addr:         bindPoint.InterfaceAddress,
				WriteTimeout: webListener.Options.WriteTimeout,
				ReadTimeout:  webListener.Options.ReadTimeout,
				IdleTimeout:  webListener.Options.IdleTimeout,
				Handler:      server.wrapHandler(webListener, bindPoint, demuxWebHandler),
				TLSConfig:    tlsConfig,
				ErrorLog:     log.New(logWriter, "", 0),
			},
		}

		namedServer.BaseContext = namedServer.NewBaseContext

		server.httpServers = append(server.httpServers, namedServer)
	}

	return server, nil
}

func (server *Server) wrapHandler(listener *WebListener, point *BindPoint, handler http.Handler) http.Handler {
	handler = server.wrapPanicRecovery(handler)
	handler = server.wrapSetCtrlAddressHeader(point, handler)

	return handler
}

// wrapPanicRecovery wraps a http.Handler with another http.Handler that provides recovery.
func (server *Server) wrapPanicRecovery(handler http.Handler) http.Handler {
	wrappedHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if panicVal := recover(); panicVal != nil {
				if server.OnHandlerPanic != nil {
					server.OnHandlerPanic(writer, request, panicVal)
					return
				}
				pfxlog.Logger().Errorf("panic caught by server handler: %v\n%v", panicVal, debugz.GenerateLocalStack())
			}
		}()

		handler.ServeHTTP(writer, request)
	})

	return wrappedHandler
}

// wrapSetCtrlAddressHeader will check to see if the bindPoint is configured to advertise a "new address". If so
// the value is added to the ZitiCtrlAddressHeader which will be sent out on every response. Clients can check this
// header to be notified that the controller is or will be moving from one ip/hostname to another. When the
// new address value is set, both the old and new addresses should be valid as the clients will begin using the
// new address on their next connect.
func (server *Server) wrapSetCtrlAddressHeader(point *BindPoint, handler http.Handler) http.Handler {
	wrappedHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if point.NewAddress != "" {
			address := "https://" + point.NewAddress
			writer.Header().Set(ZitiCtrlAddressHeader, address)
		}

		handler.ServeHTTP(writer, request)
	})

	return wrappedHandler
}

// Start the server and all underlying http.Server's
func (server *Server) Start() error {
	logger := pfxlog.Logger()

	for _, httpServer := range server.httpServers {
		logger.Infof("starting API to listen and serve tls on %s for web listener %s with APIs: %v", httpServer.Addr, httpServer.WebListener.Name, httpServer.ApiBindingList)
		err := httpServer.ListenAndServeTLS("", "")
		if err != http.ErrServerClosed {

			return fmt.Errorf("error listening: %s", err)
		}
	}

	return nil
}

// Shutdown stops the server and all underlying http.Server's
func (server *Server) Shutdown(ctx context.Context) {
	_ = server.logWriter.Close()

	for _, httpServer := range server.httpServers {
		localServer := httpServer
		func() {
			_ = localServer.Shutdown(ctx)
		}()
	}
}
