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
	"github.com/gorilla/handlers"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/debugz"
	"io"
	"log"
	"net/http"
)

type ContextKey string

const (
	WebHandlerContextKey = ContextKey("XWebHandlerContextKey")
	BindPointContextKey  = ContextKey("XWebBindPointContextKey")
)

// Represents all of the http.Server's and http.Handler's necessary to run a single xweb.WebListener
type Server struct {
	httpServers    []*http.Server
	corsOptions    []handlers.CORSOption
	logWriter      *io.PipeWriter
	options        *Options
	config         interface{}
	Handle         http.Handler
	OnHandlerPanic func(writer http.ResponseWriter, request *http.Request, panicVal interface{})
}

// Create a new xweb.Server from an xweb.WebListener. All necessary http.Handler's will be created from the supplied
// DemuxFactory and WebHandlerFactoryRegistry.
func NewServer(webListener *WebListener, demuxFactory DemuxFactory, handlerFactoryRegistry WebHandlerFactoryRegistry) (*Server, error) {
	logWriter := pfxlog.Logger().Writer()

	tlsConfig := webListener.Identity.ServerTLSConfig()
	tlsConfig.ClientAuth = tls.RequestClientCert

	tlsConfig.MinVersion = uint16(webListener.Options.MinTLSVersion)
	tlsConfig.MaxVersion = uint16(webListener.Options.MaxTLSVersion)

	server := &Server{
		logWriter:   logWriter,
		config:      &webListener,
		httpServers: []*http.Server{},
	}

	var webHandlers []WebHandler

	for _, api := range webListener.APIs {
		if factory := handlerFactoryRegistry.Get(api.Binding()); factory != nil {
			if webHandler, err := factory.New(webListener, api.Options()); err != nil {
				pfxlog.Logger().Fatalf("encountered error building handler for api binding [%s]: %v", api.Binding(), err)
			} else {
				webHandlers = append(webHandlers, webHandler)
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
		httpServer := &http.Server{
			Addr:         bindPoint.InterfaceAddress,
			WriteTimeout: webListener.Options.WriteTimeout,
			ReadTimeout:  webListener.Options.ReadTimeout,
			IdleTimeout:  webListener.Options.WriteTimeout,
			Handler:      server.wrap(demuxWebHandler, bindPoint),
			TLSConfig:    tlsConfig,
			ErrorLog:     log.New(logWriter, "", 0),
		}
		server.httpServers = append(server.httpServers, httpServer)
	}

	return server, nil
}

// Wrap a http.Handler with another http.Handler that ensures the BindPoint information is
// embedded in the http.Request, provides CORS support, and panic recovery.
func (server *Server) wrap(handler http.Handler, bindPoint *BindPoint) http.Handler {

	//todo: move all cors functionality external to xweb
	if server.corsOptions != nil {
		corsHandler := handlers.CORS(server.corsOptions...)
		handler = corsHandler(handler)
	}

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

		//store the BindPoint on the request context, useful for logging and responses with server addresses
		ctx := context.WithValue(request.Context(), BindPointContextKey, &bindPoint)
		newRequest := request.WithContext(ctx)
		handler.ServeHTTP(writer, newRequest)
	})

	return wrappedHandler
}

// Start the server and all underlying http.Server's
func (server *Server) Start() error {
	logger := pfxlog.Logger()

	for _, httpServer := range server.httpServers {
		logger.Info("starting API to listen and serve tls on: ", httpServer.Addr)
		err := httpServer.ListenAndServeTLS("", "")
		if err != http.ErrServerClosed {

			return fmt.Errorf("error listening: %s", err)
		}
	}

	return nil
}

// Stop the server and all underlying http.Server's
func (server *Server) Shutdown(ctx context.Context) {
	_ = server.logWriter.Close()

	for _, httpServer := range server.httpServers {
		localServer := httpServer
		func() {
			_ = localServer.Shutdown(ctx)
		}()
	}
}
