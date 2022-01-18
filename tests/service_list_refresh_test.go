//go:build apitests
// +build apitests

package tests

import (
	"github.com/openziti/edge/eid"
	"testing"
	"time"
)

func setupServiceListRefreshTest(ctx *TestContext) (string, *identity, *session) {
	identityRole := eid.New()
	identity, userAuth := ctx.AdminManagementSession.requireCreateIdentityWithUpdbEnrollment(eid.New(), "1s2w3e4r5t6", false, identityRole)
	nonAdminUserSession, err := userAuth.AuthenticateClientApi(ctx)
	ctx.Req.NoError(err)

	lastUpdate := nonAdminUserSession.getServiceUpdateTime()
	ctx.Req.Equal(nonAdminUserSession.createdAt, lastUpdate)

	nonAdminUserSession.requireServiceUpdateTimeUnchanged()

	return identityRole, identity, nonAdminUserSession
}

func Test_ServiceListRefresh(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	t.Run("test matched new policy", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service policy deleted", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		ctx.AdminManagementSession.requireDeleteEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service policy identity attr updated, now included", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s(), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		policy.identityRoles = s("#" + identityRole)
		ctx.AdminManagementSession.requireUpdateEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service policy identity attr updated, now excluded", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		policy.identityRoles = s()
		ctx.AdminManagementSession.requireUpdateEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service policy service attr updated, now included", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s(), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		policy.serviceRoles = s("#" + serviceRole)
		ctx.AdminManagementSession.requireUpdateEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service policy service attr updated, now excluded", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		policy.serviceRoles = s()
		ctx.AdminManagementSession.requireUpdateEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test identity addr, now included", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, identity, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		identityRole2 := eid.New()
		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole2), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		identity.roleAttributes = s(identityRole, identityRole2)
		ctx.AdminManagementSession.requireUpdateEntity(identity)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test identity addr, now excluded", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, identity, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		identityRole2 := eid.New()

		identity.roleAttributes = s(identityRole, identityRole2)
		ctx.AdminManagementSession.requireUpdateEntity(identity)
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole2), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		time.Sleep(time.Millisecond)
		identity.roleAttributes = s(identityRole)
		ctx.AdminManagementSession.requireUpdateEntity(identity)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service created", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service role attr changed, now included", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		service.roleAttributes = s(serviceRole)
		ctx.AdminManagementSession.requireUpdateEntity(service)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service role attr changed, now excluded", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		time.Sleep(time.Millisecond)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		service.roleAttributes = nil
		ctx.AdminManagementSession.requireUpdateEntity(service)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service deleted", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		ctx.AdminManagementSession.requireDeleteEntity(service)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service config changed/deleted", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		configType := ctx.AdminManagementSession.requireCreateNewConfigType()
		defer ctx.AdminManagementSession.requireDeleteEntity(configType)

		config := ctx.newConfig(configType.Id, map[string]interface{}{"name": "Alpha"})
		ctx.AdminManagementSession.requireCreateEntity(config)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), s(config.Id))
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		time.Sleep(time.Millisecond)
		config.Data = map[string]interface{}{"name": "Beta"}
		ctx.AdminManagementSession.requireUpdateEntity(config)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		ctx.AdminManagementSession.requireDeleteEntity(config)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test identity config added, updated, deleted", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, identity, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		configType := ctx.AdminManagementSession.requireCreateNewConfigType()
		defer ctx.AdminManagementSession.requireDeleteEntity(configType)

		config := ctx.newConfig(configType.Id, map[string]interface{}{"name": "Alpha"})
		ctx.AdminManagementSession.requireCreateEntity(config)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		time.Sleep(time.Millisecond)
		ctx.AdminManagementSession.requireAssignIdentityServiceConfigs(identity.Id, serviceConfig{
			ServiceId: service.Id,
			ConfigId:  config.Id,
		})

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		time.Sleep(time.Millisecond)
		config.Data = map[string]interface{}{"name": "Beta"}
		ctx.AdminManagementSession.requireUpdateEntity(config)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		ctx.AdminManagementSession.requireDeleteEntity(config)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test identity config removed", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, identity, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		configType := ctx.AdminManagementSession.requireCreateNewConfigType()
		defer ctx.AdminManagementSession.requireDeleteEntity(configType)

		config := ctx.newConfig(configType.Id, map[string]interface{}{"name": "Alpha"})
		ctx.AdminManagementSession.requireCreateEntity(config)
		defer ctx.AdminManagementSession.requireDeleteEntity(config)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
		nonAdminUserSession.requireServiceUpdateTimeUnchanged()

		ctx.AdminManagementSession.requireAssignIdentityServiceConfigs(identity.Id, serviceConfig{
			ServiceId: service.Id,
			ConfigId:  config.Id,
		})

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		ctx.AdminManagementSession.requireRemoveIdentityServiceConfigs(identity.Id, serviceConfig{
			ServiceId: service.Id,
			ConfigId:  config.Id,
		})

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test posture check added", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		postureCheckRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckDomain(s("domain1"), s(postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(postureCheck)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test posture check deleted", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		postureCheckRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckDomain(s("domain1"), s(postureCheckRole))

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		ctx.AdminManagementSession.requireDeleteEntity(postureCheck)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service policy posture roles changed, now included", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		postureCheckRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckDomain(s("domain1"), s(postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(postureCheck)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		policy.postureCheckRoles = s("#" + postureCheckRole)
		ctx.AdminManagementSession.requireUpdateEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("test service policy posture roles changed, now excluded", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		postureCheckRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckDomain(s("domain1"), s(postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(postureCheck)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		policy.postureCheckRoles = s()
		ctx.AdminManagementSession.requireUpdateEntity(policy)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("posture role attributes changed, now included", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		postureCheckRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckDomain(s("domain1"), s())
		defer ctx.AdminManagementSession.requireDeleteEntity(postureCheck)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		postureCheck.roleAttributes = s(postureCheckRole)
		ctx.AdminManagementSession.requireUpdateEntity(postureCheck)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})

	t.Run("posture check role attributes changed, now excluded", func(t *testing.T) {
		ctx.testContextChanged(t)
		identityRole, _, nonAdminUserSession := setupServiceListRefreshTest(ctx)

		serviceRole := eid.New()
		postureCheckRole := eid.New()

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)
		defer ctx.AdminManagementSession.requireDeleteEntity(service)

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckDomain(s("domain1"), s(postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(postureCheck)

		policy := ctx.AdminManagementSession.requireNewServicePolicy("Dial", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))
		defer ctx.AdminManagementSession.requireDeleteEntity(policy)

		nonAdminUserSession.requireServiceUpdateTimeAdvanced()

		postureCheck.roleAttributes = s()
		ctx.AdminManagementSession.requireUpdateEntity(postureCheck)
		nonAdminUserSession.requireServiceUpdateTimeAdvanced()
	})
}
