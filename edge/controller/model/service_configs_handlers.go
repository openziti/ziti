package model

func NewServiceConfigsHandler(env Env) *ServiceConfigsHandler {
	handler := &ServiceConfigsHandler{
		baseHandler: baseHandler{
			env:   env,
			store: env.GetStores().ServiceConfig,
		},
	}
	handler.impl = handler
	return handler
}

type ServiceConfigsHandler struct {
	baseHandler
}

func (handler *ServiceConfigsHandler) NewModelEntity() BaseModelEntity {
	return &ServiceConfigs{}
}
