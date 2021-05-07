package server

import (
	"github.com/gorilla/handlers"
	"github.com/openziti/foundation/common/constants"
	"net/http"
)

func wrapCorsHandler(innerHandler http.Handler) http.Handler{
corsOpts := []handlers.CORSOption{
		handlers.AllowedOrigins([]string{"*"}),
		handlers.OptionStatusCode(200),
		handlers.AllowedHeaders([]string{
			"content-type",
			"Accept",
			constants.ZitiSession,
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
