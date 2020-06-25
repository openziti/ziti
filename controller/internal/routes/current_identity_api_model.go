package routes

import (
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"path"
)

const EntityNameCurrentIdentity = "current-identity"

var CurrentIdentityLinkFactory FullLinkFactory = NewCurrentIdentityLinkFactory()

type CurrentIdentityLinkFactoryImpl struct {
	BasicLinkFactory
}

func NewCurrentIdentityLinkFactory() *CurrentIdentityLinkFactoryImpl {
	return &CurrentIdentityLinkFactoryImpl{
		BasicLinkFactory: *NewBasicLinkFactory(EntityNameCurrentIdentity),
	}
}

func (factory *CurrentIdentityLinkFactoryImpl) SelfUrlString(_ string) string {
	return "./" + factory.entityName
}


func (factory CurrentIdentityLinkFactoryImpl) NewNestedLink(_ models.Entity, elem ...string) rest_model.Link {
	elem = append([]string{factory.SelfUrlString("")}, elem...)
	return NewLink("./" + path.Join(elem...))
}

func (factory *CurrentIdentityLinkFactoryImpl) SelfLink(_ models.Entity) rest_model.Link {
	return NewLink("./" + factory.entityName)
}

func (factory *CurrentIdentityLinkFactoryImpl) Links(entity models.Entity) rest_model.Links {
	return rest_model.Links{
		EntityNameSelf: factory.SelfLink(entity),
		EntityNameAuthenticator: factory.NewNestedLink(nil, EntityNameAuthenticator),
	}
}