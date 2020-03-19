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

package subcmd

import (
	"github.com/netfoundry/ziti-foundation/channel2"
	"github.com/netfoundry/ziti-foundation/common/constants"
	"github.com/netfoundry/ziti-foundation/identity/identity"
	"github.com/netfoundry/ziti-foundation/transport"
	"github.com/gorilla/mux"
	"github.com/michaelquigley/pfxlog"
	"github.com/rs/cors"
	"github.com/spf13/cobra"
	"github.com/urfave/negroni"
	"net/http"
)

func init() {
	root.AddCommand(runCmd)
}

var (
	mgmtCh      mgmtChannel
	mgmtAddress transport.Address
)

var runCmd = &cobra.Command{
	Use:   "run <config>",
	Short: "Run configuration",
	Args:  cobra.ExactArgs(1),
	Run:   run,
}

func run(_ *cobra.Command, args []string) {
	if config, err := loadConfig(args[0]); err == nil {
		if id, err := identity.LoadClientIdentity(config.MgmtCertPath, config.MgmtKeyPath, config.MgmtCaCertPath); err == nil {
			dialer := channel2.NewClassicDialer(id, config.mgmtAddress, nil)
			if mc, err := channel2.NewChannel("ziti-fabric-gw", dialer, nil); err == nil {
				log := pfxlog.Logger()

				SetMgmtCh(mc)
				router := NewRouter()

				/**
				 *	Configure the CORS handler
				 */
				c := cors.New(cors.Options{
					AllowedOrigins: []string{"*"},
					AllowedMethods: []string{
						"GET", "POST", "PATCH", "DELETE", "OPTIONS",
					},
					AllowedHeaders: []string{
						"Content-Type",
						"Accept",
						constants.ZitiSession,
					},
					Debug: false, // Enable Debugging for testing, disable this in production
				})

				n := negroni.New()
				n.Use(negroni.NewLogger()) // Insert the REST tracing middleware
				n.Use(c)                   // Insert the CORS middleware
				n.UseHandler(router)

				log.Info("started")
				if config.MgmtGwCertPath != "" && config.MgmtGwKeyPath != "" {
					log.Fatal(http.ListenAndServeTLS(config.MgmtGwListenAddress, config.MgmtGwCertPath, config.MgmtGwKeyPath, n))
				} else {
					log.Fatal(http.ListenAndServe(config.MgmtGwListenAddress, n))
				}
			} else {
				panic(err)
			}
		} else {
			panic(err)
		}
	} else {
		panic(err)
	}
}

type mgmtChannel interface {
	SendAndWait(requestMsg *channel2.Message) (chan *channel2.Message, error)
}

// SetMgmtCh sets the channel between this fabric-gw server and the controller. We export this method for testability
func SetMgmtCh(c mgmtChannel) {
	mgmtCh = c
}

// NewRouter builds the ziti-router for the server. We export this method for testability
func NewRouter() http.Handler {
	router := mux.NewRouter()
	router.Use(addJsonContentType)
	router.HandleFunc("/ctrl/links", handleListLinks).Methods("GET")
	router.HandleFunc("/ctrl/routers", handleListRouters).Methods("GET")
	router.HandleFunc("/ctrl/services", handleListServices).Methods("GET")
	router.HandleFunc("/ctrl/services", handleCreateService).Methods("POST")
	router.HandleFunc("/ctrl/services/{id}", handleGetService).Methods("GET")
	router.HandleFunc("/ctrl/services/{id}", handleRemoveService).Methods("DELETE")
	router.HandleFunc("/ctrl/sessions", handleListSessions).Methods("GET")
	return router
}

func addJsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}
