package db

type Stores struct {
	Router  RouterStore
	Service ServiceStore
}

type stores struct {
	router  *routerStoreImpl
	service *serviceStoreImpl
}

func InitStores() *Stores {
	internalStores := &stores{}
	internalStores.router = newRouterStore(internalStores)
	internalStores.service = newServiceStore(internalStores)

	stores := &Stores{
		Router:  internalStores.router,
		Service: internalStores.service,
	}

	return stores
}
