package handler_edge_ctrl

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/controller/change"
	"github.com/openziti/ziti/v2/controller/db"
	"github.com/openziti/ziti/v2/controller/env"
	"github.com/openziti/ziti/v2/controller/model"
	"github.com/openziti/ziti/v2/controller/network"
	"github.com/stretchr/testify/require"
)

type legacySessionTestHandler struct {
	appEnv *env.AppEnv
}

func (self *legacySessionTestHandler) getAppEnv() *env.AppEnv {
	return self.appEnv
}

func (*legacySessionTestHandler) getNetwork() *network.Network {
	return nil
}

func (*legacySessionTestHandler) getChannel() channel.Channel {
	return nil
}

func (*legacySessionTestHandler) Label() string {
	return "legacy-session-test"
}

func newLegacySessionTestEntities(t *testing.T, testCtx *model.TestContext) (*model.Managers, *model.Identity, *model.EdgeService) {
	managers := testCtx.GetManagers()
	identity := &model.Identity{
		Name:           uuid.NewString(),
		IdentityTypeId: db.DefaultIdentityType,
	}
	require.NoError(t, managers.Identity.Create(identity, change.New()))

	service := &model.EdgeService{Name: uuid.NewString()}
	require.NoError(t, managers.EdgeService.Create(service, change.New()))

	edgeRouter := &model.EdgeRouter{Name: uuid.NewString()}
	require.NoError(t, managers.EdgeRouter.Create(edgeRouter, change.New()))

	servicePolicy := &model.ServicePolicy{
		Name:          uuid.NewString(),
		Semantic:      db.SemanticAllOf,
		IdentityRoles: []string{"#all"},
		ServiceRoles:  []string{"#all"},
		PolicyType:    db.PolicyTypeDialName,
	}
	require.NoError(t, managers.ServicePolicy.Create(servicePolicy, change.New()))

	edgeRouterPolicy := &model.EdgeRouterPolicy{
		Name:            uuid.NewString(),
		Semantic:        db.SemanticAllOf,
		IdentityRoles:   []string{"#all"},
		EdgeRouterRoles: []string{"#all"},
	}
	require.NoError(t, managers.EdgeRouterPolicy.Create(edgeRouterPolicy, change.New()))

	serviceEdgeRouterPolicy := &model.ServiceEdgeRouterPolicy{
		Name:            uuid.NewString(),
		Semantic:        db.SemanticAllOf,
		ServiceRoles:    []string{"#all"},
		EdgeRouterRoles: []string{"#all"},
	}
	require.NoError(t, managers.ServiceEdgeRouterPolicy.Create(serviceEdgeRouterPolicy, change.New()))

	return managers, identity, service
}

func TestLoadFromBoltSupportsOpaqueServiceSessionTokens(t *testing.T) {
	testCtx := model.NewTestContext(t)
	defer testCtx.Cleanup()
	testCtx.Init()

	managers, identity, service := newLegacySessionTestEntities(t, testCtx)

	apiSession := &model.ApiSession{
		Token:          uuid.NewString(),
		IdentityId:     identity.Id,
		Identity:       identity,
		LastActivityAt: time.Now(),
	}
	_, err := managers.ApiSession.Create(nil, apiSession, nil)
	require.NoError(t, err)

	legacySession := &model.Session{
		Token:        uuid.NewString(),
		IdentityId:   identity.Id,
		ApiSessionId: apiSession.Id,
		ServiceId:    service.Id,
		Type:         db.SessionTypeDial,
	}
	_, err = managers.Session.Create(legacySession, change.New())
	require.NoError(t, err)

	handler := &legacySessionTestHandler{
		appEnv: &env.AppEnv{Managers: managers},
	}
	requestCtx := &baseSessionRequestContext{handler: handler}

	requestCtx.loadFromBolt(legacySession.Token, apiSession.Token)

	require.NoError(t, requestCtx.err)
	require.Equal(t, legacySession.Id, requestCtx.session.Id)
	require.Equal(t, apiSession.Id, requestCtx.apiSession.Id)
}

func TestLoadFromBoltRejectsOpaqueServiceSessionForDifferentApiSession(t *testing.T) {
	testCtx := model.NewTestContext(t)
	defer testCtx.Cleanup()
	testCtx.Init()

	managers, identity, service := newLegacySessionTestEntities(t, testCtx)

	newApiSession := func() *model.ApiSession {
		apiSession := &model.ApiSession{
			Token:          uuid.NewString(),
			IdentityId:     identity.Id,
			Identity:       identity,
			LastActivityAt: time.Now(),
		}
		_, err := managers.ApiSession.Create(nil, apiSession, nil)
		require.NoError(t, err)
		return apiSession
	}

	ownerApiSession := newApiSession()
	otherApiSession := newApiSession()
	legacySession := &model.Session{
		Token:        uuid.NewString(),
		IdentityId:   identity.Id,
		ApiSessionId: ownerApiSession.Id,
		ServiceId:    service.Id,
		Type:         db.SessionTypeDial,
	}
	_, err := managers.Session.Create(legacySession, change.New())
	require.NoError(t, err)

	handler := &legacySessionTestHandler{
		appEnv: &env.AppEnv{Managers: managers},
	}
	requestCtx := &baseSessionRequestContext{handler: handler}

	requestCtx.loadFromBolt(legacySession.Token, otherApiSession.Token)

	require.IsType(t, InvalidSessionError{}, requestCtx.err)
}
