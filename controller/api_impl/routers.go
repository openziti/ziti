package api_impl

import "github.com/openziti/fabric/rest_server/operations"

var routers []Router

func AddRouter(router Router) {
	routers = append(routers, router)
}

type Router interface {
	Register(fabricApi *operations.ZitiFabricAPI, wrapper RequestWrapper)
}
