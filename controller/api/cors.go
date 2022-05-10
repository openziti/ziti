package api

import (
	"github.com/gorilla/handlers"
	"net/http"
)

const (
	// ZitiSession is the header value used to pass Ziti sessions around
	ZitiSession = "zt-session"
)

func WrapCorsHandler(innerHandler http.Handler) http.Handler {
	corsOpts := []handlers.CORSOption{
		handlers.AllowedOrigins([]string{"*"}),
		handlers.OptionStatusCode(200),
		handlers.AllowedHeaders([]string{
			"content-type",
			"accept",
			"authorization",
			// TODO: Not required for pure fabric. Is it worth having separate CorsHandlers for fabric and edge?
			ZitiSession,
		}),
		handlers.AllowedMethods([]string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete}),
		handlers.AllowCredentials(),
	}

	return handlers.CORS(corsOpts...)(innerHandler)
}
