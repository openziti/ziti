package tests

import (
	"crypto/x509"
	"fmt"
	"github.com/dgryski/dgoogauth"
	"github.com/google/uuid"
	"github.com/openziti/edge-api/rest_management_api_client/api_session"
	"github.com/openziti/edge-api/rest_management_api_client/edge_router_policy"
	management_service "github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_management_api_client/service_edge_router_policy"
	management_service_policy "github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/edge-api/rest_util"
	edge_apis "github.com/openziti/sdk-golang/edge-apis"
	"github.com/openziti/sdk-golang/ziti"
	"net/url"
	"testing"
	"time"
)

func Test_SDK_Events(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()

	clientApiUrl := "https://" + ctx.ApiHost + EdgeClientApiPath

	managementApiUrl, err := url.Parse("https://" + ctx.ApiHost + EdgeManagementApiPath)
	ctx.Req.NoError(err)

	adminCreds := edge_apis.NewUpdbCredentials(ctx.AdminAuthenticator.Username, ctx.AdminAuthenticator.Password)
	adminClient := edge_apis.NewManagementApiClient(managementApiUrl, ctx.ControllerConfig.Id.CA(), nil)
	apiSession, err := adminClient.Authenticate(adminCreds, nil)
	ctx.NoError(err)
	ctx.NotNil(apiSession)

	t.Run("EventAuthenticationStateFull emitted after full authentication", func(t *testing.T) {
		//setup
		ctx.testContextChanged(t)
		testId := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
		testIdCerts := ctx.completeOttEnrollment(testId.Id)

		testIdCreds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		testIdCreds.CaPool = ctx.ControllerConfig.Id.CA()

		cfg := &ziti.Config{
			ZtAPI:       clientApiUrl,
			ConfigTypes: nil,
			Credentials: testIdCreds,
		}

		ztx, err := ziti.NewContext(cfg)

		defer func() {
			ztx.Close()
		}()

		ctx.Req.NoError(err)
		ctx.Req.NotNil(ztx)

		called := make(chan *edge_apis.ApiSession, 1)

		removeFullListener := ztx.Events().AddAuthenticationStateFullListener(func(ztx ziti.Context, detail *edge_apis.ApiSession) {
			ctx.Req.NotNil(ztx)
			called <- detail
		})

		//perform
		err = ztx.Authenticate()

		ctx.Req.NoError(err)

		//test
		select {
		case newApiSession := <-called:
			ctx.Req.NotNil(newApiSession)
			ctx.Req.NotEmpty(newApiSession.GetToken())
			ctx.Req.Empty(newApiSession.AuthQueries, "expected 0 auth queries")
		case <-time.After(time.Second * 5):
			ctx.Req.Fail("time out, full auth event never encountered")
		}

		t.Run("calling listener removers results in no listeners", func(t *testing.T) {
			ctx.testContextChanged(t)
			removeFullListener()

			eventNames := ztx.Events().EventNames()

			for _, eventName := range eventNames {
				ctx.Req.Zero(ztx.Events().ListenerCount(eventName), "expected 0 listeners for event %s", eventName)
			}
		})

		t.Run("EventAuthenticationStatePartial is emitted", func(t *testing.T) {
			ctx.testContextChanged(t)

			mfa, err := ztx.EnrollZitiMfa()

			ctx.Req.NoError(err)

			ctx.Req.NotEmpty(mfa.ProvisioningURL)

			parsedUrl, err := url.Parse(mfa.ProvisioningURL)
			ctx.Req.NoError(err)

			queryParams, err := url.ParseQuery(parsedUrl.RawQuery)
			ctx.Req.NoError(err)

			secrets := queryParams["secret"]
			ctx.Req.NotNil(secrets)
			ctx.Req.NotEmpty(secrets)

			mfaSecret := secrets[0]

			now := time.Now().UTC().Unix() / 30
			codeInt := dgoogauth.ComputeCode(mfaSecret, now)
			code := fmt.Sprintf("%06d", codeInt)

			err = ztx.VerifyZitiMfa(code)
			ctx.Req.NoError(err)

			ztxPostMfa, err := ziti.NewContext(cfg)
			ctx.Req.NoError(err)

			defer func() {
				ztxPostMfa.Close()
			}()

			partialChan := make(chan *edge_apis.ApiSession, 1)

			removePartialListener := ztxPostMfa.Events().AddAuthenticationStatePartialListener(func(ztx ziti.Context, detail *edge_apis.ApiSession) {
				ctx.Req.NotNil(ztx)
				partialChan <- detail
			})

			err = ztxPostMfa.Authenticate()
			ctx.Req.NoError(err)

			select {
			case newApiSession := <-partialChan:
				ctx.Req.Len(newApiSession.AuthQueries, 1, "expected 1 auth query")
			case <-time.After(5 * time.Second):
				ctx.Req.Fail("time out, partial auth event not received")
			}

			t.Run("calling listener removers results in no listeners", func(t *testing.T) {
				ctx.testContextChanged(t)
				removePartialListener()

				eventNames := ztx.Events().EventNames()

				for _, eventName := range eventNames {
					ctx.Req.Zero(ztx.Events().ListenerCount(eventName), "expected 0 listeners for event %s", eventName)
				}
			})

			t.Run("EventAuthenticationStateFull emitted after providing MFA TOTP Code", func(t *testing.T) {
				ctx.testContextChanged(t)

				fullChan := make(chan *edge_apis.ApiSession, 1)

				fullListenerRemover := ztxPostMfa.Events().AddAuthenticationStateFullListener(func(ztx ziti.Context, detail *edge_apis.ApiSession) {
					ctx.Req.NotNil(ztx)
					fullChan <- detail
				})

				mfaListenerRemover := ztxPostMfa.Events().AddMfaTotpCodeListener(func(ztx ziti.Context, query *rest_model.AuthQueryDetail, response ziti.MfaCodeResponse) {
					ctx.Req.NotNil(ztx)
					ctx.Req.NotNil(query)

					now := time.Now().UTC().Unix() / 30
					codeInt := dgoogauth.ComputeCode(mfaSecret, now)
					authCode := fmt.Sprintf("%06d", codeInt)

					err := response(authCode)
					ctx.Req.NoError(err)
				})
				ztxImpl := ztxPostMfa.(*ziti.ContextImpl)
				err = ztxImpl.Reauthenticate()
				ctx.Req.NoError(err)

				select {
				case newApiSession := <-fullChan:
					ctx.Req.NotNil(newApiSession)
					ctx.Req.NotEmpty(newApiSession.GetToken())
					ctx.Req.Empty(newApiSession.AuthQueries, "expected 0 auth queries")
				case <-time.After(time.Second * 5):
					ctx.Req.Fail("time out")
				}

				t.Run("EventAuthenticationStateUnauthenticated emitted if the current API Session is deleted", func(t *testing.T) {
					ctx.testContextChanged(t)

					unauthCalled := make(chan *edge_apis.ApiSession, 1)

					removeUnauthedListener := ztxPostMfa.Events().AddAuthenticationStateUnauthenticatedListener(func(ztx ziti.Context, detail *edge_apis.ApiSession) {
						ctx.Req.NotNil(ztx)
						ctx.Req.NotNil(detail)

						unauthCalled <- detail
					})

					defer removeUnauthedListener()

					implZtx := ztxPostMfa.(*ziti.ContextImpl)
					apiSessionId := *implZtx.CtrlClt.ApiSession.Load().ID

					deleteParams := api_session.NewDeleteAPISessionsParams()
					deleteParams.ID = apiSessionId

					resp, err := adminClient.API.APISession.DeleteAPISessions(deleteParams, nil)
					err = rest_util.WrapErr(err)
					ctx.Req.NoError(err)
					ctx.Req.NotNil(resp)

					_ = ztxPostMfa.RefreshServices()

					select {
					case apiSession := <-unauthCalled:
						ctx.Req.NotNil(apiSession)
						ctx.Req.NotEmpty(apiSession.GetToken())
					case <-time.After(time.Second * 5):
						ctx.Req.Fail("time out")
					}

				})

				t.Run("calling listener removers results in no listeners", func(t *testing.T) {
					ctx.testContextChanged(t)

					fullListenerRemover()
					mfaListenerRemover()

					eventNames := ztx.Events().EventNames()

					for _, eventName := range eventNames {
						ctx.Req.Zero(ztx.Events().ListenerCount(eventName), "expected 0 listeners for event %s", eventName)
					}
				})
			})
		})
	})

	t.Run("EventServiceAdded is emitted", func(t *testing.T) {
		//setup
		ctx.testContextChanged(t)
		testId := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
		testIdCerts := ctx.completeOttEnrollment(testId.Id)

		testIdCreds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		testIdCreds.CaPool = ctx.ControllerConfig.Id.CA()

		serviceName := uuid.NewString()
		serviceParams := management_service.NewCreateServiceParams()
		serviceParams.Service = &rest_model.ServiceCreate{
			Name:               &serviceName,
			EncryptionRequired: ToPtr(true),
		}

		serviceResp, err := adminClient.API.Service.CreateService(serviceParams, nil)
		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		//service.CreateServiceBadRequest, Payload() rest_model.APIError > Error rest_model.APIError
		policyParams := management_service_policy.NewCreateServicePolicyParams()
		policyParams.Policy = &rest_model.ServicePolicyCreate{
			IdentityRoles: []string{"@" + testId.Id},
			ServiceRoles:  []string{"@" + serviceResp.Payload.Data.ID},
			Name:          ToPtr(uuid.NewString()),
			Type:          ToPtr(rest_model.DialBindBind),
			Semantic:      ToPtr(rest_model.SemanticAnyOf),
		}

		_, err = adminClient.API.ServicePolicy.CreateServicePolicy(policyParams, nil)
		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)

		cfg := &ziti.Config{
			ZtAPI:       clientApiUrl,
			ConfigTypes: nil,
			Credentials: testIdCreds,
		}

		ztx, err := ziti.NewContext(cfg)

		defer func() {
			ztx.Close()
		}()

		ctx.Req.NoError(err)
		ctx.Req.NotNil(ztx)

		called := make(chan *rest_model.ServiceDetail, 1)

		serviceAddedRemover := ztx.Events().AddServiceAddedListener(func(ztx ziti.Context, detail *rest_model.ServiceDetail) {
			ctx.Req.NotNil(ztx)
			called <- detail
		})

		//perform
		err = ztx.Authenticate()

		ctx.Req.NoError(err)

		//test
		select {
		case serviceDetail := <-called:
			ctx.Req.Equal(serviceName, *serviceDetail.Name)
		case <-time.After(time.Second * 5):
			ctx.Req.Fail("time out")
		}

		t.Run("calling listener removers results in no listeners", func(t *testing.T) {
			ctx.testContextChanged(t)

			serviceAddedRemover()

			eventNames := ztx.Events().EventNames()

			for _, eventName := range eventNames {
				ctx.Req.Zero(ztx.Events().ListenerCount(eventName), "expected 0 listeners for event %s", eventName)
			}
		})
	})

	t.Run("EventServiceChanged is emitted", func(t *testing.T) {
		//setup
		ctx.testContextChanged(t)
		testId := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
		testIdCerts := ctx.completeOttEnrollment(testId.Id)

		testIdCreds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		testIdCreds.CaPool = ctx.ControllerConfig.Id.CA()

		serviceName := uuid.NewString()
		serviceParams := management_service.NewCreateServiceParams()
		serviceParams.Service = &rest_model.ServiceCreate{
			Name:               &serviceName,
			EncryptionRequired: ToPtr(true),
		}

		serviceResp, err := adminClient.API.Service.CreateService(serviceParams, nil)
		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		//service.CreateServiceBadRequest, Payload() rest_model.APIError > Error rest_model.APIError
		policyParams := management_service_policy.NewCreateServicePolicyParams()
		policyParams.Policy = &rest_model.ServicePolicyCreate{
			IdentityRoles: []string{"@" + testId.Id},
			ServiceRoles:  []string{"@" + serviceResp.Payload.Data.ID},
			Name:          ToPtr(uuid.NewString()),
			Type:          ToPtr(rest_model.DialBindBind),
			Semantic:      ToPtr(rest_model.SemanticAnyOf),
		}

		_, err = adminClient.API.ServicePolicy.CreateServicePolicy(policyParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)

		cfg := &ziti.Config{
			ZtAPI:       clientApiUrl,
			ConfigTypes: nil,
			Credentials: testIdCreds,
		}

		ztx, err := ziti.NewContext(cfg)

		defer func() {
			ztx.Close()
		}()

		ctx.Req.NoError(err)
		ctx.Req.NotNil(ztx)

		serviceAddedChan := make(chan *rest_model.ServiceDetail, 1)
		serviceChangedChan := make(chan *rest_model.ServiceDetail, 1)

		serviceAddedRemover := ztx.Events().AddServiceAddedListener(func(ztx ziti.Context, detail *rest_model.ServiceDetail) {
			ctx.Req.NotNil(ztx)
			serviceAddedChan <- detail
		})

		serviceChangedRemover := ztx.Events().AddServiceChangedListener(func(ztx ziti.Context, detail *rest_model.ServiceDetail) {
			ctx.Req.NotNil(ztx)
			serviceChangedChan <- detail
		})

		//perform
		err = ztx.Authenticate()
		ctx.Req.NoError(err)

		select {
		case <-serviceAddedChan:
			//wait for added so we can trigger an update/change later
		case <-time.After(5 * time.Second):
			ctx.Req.Fail("time out, service added event not received")
		}

		patchServiceParams := management_service.NewPatchServiceParams()
		patchServiceParams.ID = serviceResp.Payload.Data.ID
		patchServiceParams.Service = &rest_model.ServicePatch{
			TerminatorStrategy: "weighted",
		}

		_, err = adminClient.API.Service.PatchService(patchServiceParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)

		err = ztx.RefreshServices()
		ctx.Req.NoError(err)

		//test
		select {
		case serviceDetail := <-serviceChangedChan:
			ctx.Req.Equal(serviceName, *serviceDetail.Name)
		case <-time.After(time.Second * 5):
			ctx.Req.Fail("time out, service changed event never received")
		}

		t.Run("calling listener removers results in no listeners", func(t *testing.T) {
			ctx.testContextChanged(t)

			serviceAddedRemover()
			serviceChangedRemover()

			eventNames := ztx.Events().EventNames()

			for _, eventName := range eventNames {
				ctx.Req.Zero(ztx.Events().ListenerCount(eventName), "expected 0 listeners for event %s", eventName)
			}
		})
	})

	t.Run("EventServiceRemoved is emitted", func(t *testing.T) {
		//setup
		ctx.testContextChanged(t)
		testId := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
		testIdCerts := ctx.completeOttEnrollment(testId.Id)

		testIdCreds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		testIdCreds.CaPool = ctx.ControllerConfig.Id.CA()

		serviceName := uuid.NewString()
		serviceParams := management_service.NewCreateServiceParams()
		serviceParams.Service = &rest_model.ServiceCreate{
			Name:               &serviceName,
			EncryptionRequired: ToPtr(true),
		}

		serviceResp, err := adminClient.API.Service.CreateService(serviceParams, nil)
		err = rest_util.WrapErr(err)

		ctx.Req.NoError(err)
		//service.CreateServiceBadRequest, Payload() rest_model.APIError > Error rest_model.APIError
		policyParams := management_service_policy.NewCreateServicePolicyParams()
		policyParams.Policy = &rest_model.ServicePolicyCreate{
			IdentityRoles: []string{"@" + testId.Id},
			ServiceRoles:  []string{"@" + serviceResp.Payload.Data.ID},
			Name:          ToPtr(uuid.NewString()),
			Type:          ToPtr(rest_model.DialBindBind),
			Semantic:      ToPtr(rest_model.SemanticAnyOf),
		}

		_, err = adminClient.API.ServicePolicy.CreateServicePolicy(policyParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)

		cfg := &ziti.Config{
			ZtAPI:       clientApiUrl,
			ConfigTypes: nil,
			Credentials: testIdCreds,
		}

		ztx, err := ziti.NewContext(cfg)

		ctx.Req.NoError(err)
		ctx.Req.NotNil(ztx)

		serviceAddedChan := make(chan *rest_model.ServiceDetail, 1)
		serviceRemovedChan := make(chan *rest_model.ServiceDetail, 1)

		serviceAddedRemover := ztx.Events().AddServiceAddedListener(func(ztx ziti.Context, detail *rest_model.ServiceDetail) {
			ctx.Req.NotNil(ztx)
			serviceAddedChan <- detail
		})

		serviceRemovedRemover := ztx.Events().AddServiceRemovedListener(func(ztx ziti.Context, detail *rest_model.ServiceDetail) {
			ctx.Req.NotNil(ztx)
			serviceRemovedChan <- detail
		})

		//perform
		err = ztx.Authenticate()
		ctx.Req.NoError(err)

		select {
		case <-serviceAddedChan:
			//wait for added so we can trigger an update/change later
		case <-time.After(5 * time.Second):
			ctx.Req.Fail("time out, service added event not received")
		}

		patchServiceParams := management_service.NewDeleteServiceParams()
		patchServiceParams.ID = serviceResp.Payload.Data.ID

		_, err = adminClient.API.Service.DeleteService(patchServiceParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)

		err = ztx.RefreshServices()
		ctx.Req.NoError(err)

		//test
		select {
		case serviceDetail := <-serviceRemovedChan:
			ctx.Req.Equal(serviceName, *serviceDetail.Name)
		case <-time.After(time.Second * 5):
			ctx.Req.Fail("time out, service removed event never received")
		}

		t.Run("calling listener removers results in no listeners", func(t *testing.T) {
			ctx.testContextChanged(t)

			serviceAddedRemover()
			serviceRemovedRemover()

			eventNames := ztx.Events().EventNames()

			for _, eventName := range eventNames {
				ctx.Req.Zero(ztx.Events().ListenerCount(eventName), "expected 0 listeners for event %s", eventName)
			}
		})
	})

	t.Run("edge router connects", func(t *testing.T) {
		//setup
		ctx.testContextChanged(t)
		ctx.CreateEnrollAndStartEdgeRouter()

		testId := ctx.AdminManagementSession.RequireNewIdentityWithOtt(false)
		testIdCerts := ctx.completeOttEnrollment(testId.Id)

		testIdCreds := edge_apis.NewCertCredentials([]*x509.Certificate{testIdCerts.cert}, testIdCerts.key)
		testIdCreds.CaPool = ctx.ControllerConfig.Id.CA()

		serviceName := uuid.NewString()
		serviceParams := management_service.NewCreateServiceParams()
		serviceParams.Service = &rest_model.ServiceCreate{
			Name:               &serviceName,
			EncryptionRequired: ToPtr(true),
		}

		serviceResp, err := adminClient.API.Service.CreateService(serviceParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)

		servicePolicyParams := management_service_policy.NewCreateServicePolicyParams()
		servicePolicyParams.Policy = &rest_model.ServicePolicyCreate{
			IdentityRoles: []string{"@" + testId.Id},
			ServiceRoles:  []string{"@" + serviceResp.Payload.Data.ID},
			Name:          ToPtr(uuid.NewString()),
			Type:          ToPtr(rest_model.DialBindDial),
			Semantic:      ToPtr(rest_model.SemanticAnyOf),
		}

		_, err = adminClient.API.ServicePolicy.CreateServicePolicy(servicePolicyParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)

		erPolicyParams := edge_router_policy.NewCreateEdgeRouterPolicyParams()
		erPolicyParams.Policy = &rest_model.EdgeRouterPolicyCreate{
			IdentityRoles:   []string{"@" + testId.Id},
			EdgeRouterRoles: []string{"#all"},
			Name:            ToPtr(uuid.NewString()),
			Semantic:        ToPtr(rest_model.SemanticAnyOf),
		}

		_, err = adminClient.API.EdgeRouterPolicy.CreateEdgeRouterPolicy(erPolicyParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)

		serPolicyParams := service_edge_router_policy.NewCreateServiceEdgeRouterPolicyParams()
		serPolicyParams.Policy = &rest_model.ServiceEdgeRouterPolicyCreate{
			ServiceRoles:    []string{"@" + serviceResp.Payload.Data.ID},
			EdgeRouterRoles: []string{"#all"},
			Name:            ToPtr(uuid.NewString()),
			Semantic:        ToPtr(rest_model.SemanticAnyOf),
		}

		_, err = adminClient.API.ServiceEdgeRouterPolicy.CreateServiceEdgeRouterPolicy(serPolicyParams, nil)
		err = rest_util.WrapErr(err)
		ctx.Req.NoError(err)

		cfg := &ziti.Config{
			ZtAPI:       clientApiUrl,
			ConfigTypes: nil,
			Credentials: testIdCreds,
		}

		ztx, err := ziti.NewContext(cfg)

		defer func() {
			ztx.Close()
		}()

		ctx.Req.NoError(err)
		ctx.Req.NotNil(ztx)

		connectedCalled := make(chan []string, 1)

		routerConRemover := ztx.Events().AddRouterConnectedListener(func(ztx ziti.Context, name, key string) {
			ctx.Req.NotNil(ztx)
			connectedCalled <- []string{name, key}
		})

		disconnectedCalled := make(chan []string, 1)

		rouerDisconRemover := ztx.Events().AddRouterDisconnectedListener(func(ztx ziti.Context, name, key string) {
			ctx.Req.NotNil(ztx)
			disconnectedCalled <- []string{name, key}
		})

		//perform
		err = ztx.Authenticate()

		ctx.Req.NoError(err)

		_, _ = ztx.Dial(serviceName)

		//test
		select {
		case data := <-connectedCalled:
			ctx.Req.Len(data, 2)
		case <-time.After(time.Second * 5):
			ctx.Req.Fail("time out")
		}

		t.Run("disconnect event", func(t *testing.T) {
			ctx.testContextChanged(t)

			ztxImpl := ztx.(*ziti.ContextImpl)
			ztxImpl.CloseAllEdgeRouterConns()

			//test
			select {
			case data := <-disconnectedCalled:
				ctx.Req.Len(data, 2)
			case <-time.After(time.Second * 5):
				ctx.Req.Fail("time out")
			}
		})

		t.Run("calling listener removers results in no listeners", func(t *testing.T) {
			ctx.testContextChanged(t)

			routerConRemover()
			rouerDisconRemover()

			eventNames := ztx.Events().EventNames()

			for _, eventName := range eventNames {
				ctx.Req.Zero(ztx.Events().ListenerCount(eventName), "expected 0 listeners for event %s", eventName)
			}
		})
	})
}
