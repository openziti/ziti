//go:build apitests
// +build apitests

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

package tests

import (
	"github.com/Jeffail/gabs"
	"github.com/google/uuid"
	"github.com/openziti/edge/eid"
	"github.com/openziti/edge/rest_model"
	"net/http"
	"testing"
)

func Test_PostureChecks_ProcessMulti(t *testing.T) {
	ctx := NewTestContext(t)
	defer ctx.Teardown()
	ctx.StartServer()
	ctx.RequireAdminManagementApiLogin()
	ctx.CreateEnrollAndStartEdgeRouter()

	hashes := []string{
		"3AF35956A71C2AFEFBFE356F86C9139725EEECB15F0DE7D98557D4D696C434F51FBC2FA5F7543AEF4F5F1AFB83CAA4A43619973BAE52E1F4F92EC10C39B039E8",
		"a68df267c559fb7a3423c63f554b2510a0e83c5fac293c0ce764c9fc9c5cfb017b8054e106a208d85ac7b1df31b842fdb0a6736fc303e1e608079d4d33938032",

		"3fb4043fb17cc0a2c27b7bff0abec80d66efcb377f6492a6cc184ffcb504daf2646b2bdb1b2995f78012057b4832b6d1751fb4b29cf5efe4d354b5434a070476",
		"6f2665aaf133d99bd216dc767184b65f2cdd5ea5f71ea5b08b5164488a28f756e8cd0f23b066aa8c0d5ec0e18439f25e3e57d60aa683786a2e7b8f6d9ffa5ea6",

		"d1d9b088df8b9cae93ec7775e5ecbe60a18118a539385aa8834bbd46b26cab755eff5423ccb70786028c7aff6e866c82b2828ac9c7e57a76c82619fd50d577d8",
		"9245d5c7f8a03277b03297244c21108e6e148ce7579823273def587cfb5ee0410b415a0f136af9898789083ab952cbbd669cd8459e006a8e9e219d1b494f38aa",
	}
	signerFingerprints := []string{
		"79437F5EDDA13F9C0669B978DD7A9066DD2059F1",
		"8B79ED7675A5D6A8D42FDC35499FB129655A1703",

		"BCA1F4699F52A240E13BDB4381B76848EC0C8789",
		"F356E3150FDBAA6870D27A6F316ED3CEA87F3A5B",

		"47289771B7A914B72588282C6FF5C0127AED2F17",
		"753DBE7F20E91437C33E89E2950BE47F3D0CE8C9",
	}

	osTypeWindows := rest_model.OsTypeWindows

	//Has hashes, has signers
	process01Path := `\path\to\some\binary01`
	process01 := &rest_model.ProcessMulti{
		Hashes: []string{
			hashes[0],
			hashes[1],
		},
		OsType: &osTypeWindows,
		Path:   &process01Path,
		SignerFingerprints: []string{
			signerFingerprints[0],
			signerFingerprints[1],
		},
	}

	//No hashes, has signers
	process02Path := `\path\to\some\binary02`
	process02 := &rest_model.ProcessMulti{
		Hashes: []string{},
		OsType: &osTypeWindows,
		Path:   &process02Path,
		SignerFingerprints: []string{
			signerFingerprints[2],
			signerFingerprints[3],
		},
	}

	//No signers, has hashes
	process03Path := `\path\to\some\binary03`
	process03 := &rest_model.ProcessMulti{
		Hashes: []string{
			hashes[4],
			hashes[5],
		},
		OsType:             &osTypeWindows,
		Path:               &process03Path,
		SignerFingerprints: []string{},
	}

	//No signers, no hashes
	process04Path := `\path\to\some\binary04`
	process04 := &rest_model.ProcessMulti{
		Hashes:             []string{},
		OsType:             &osTypeWindows,
		Path:               &process04Path,
		SignerFingerprints: []string{},
	}

	t.Run("can create a process all of multi posture check (allOf, 1 process) associated to a service", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		processes := []*rest_model.ProcessMulti{
			process01,
		}

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckProcessMulti(rest_model.SemanticAllOf, processes, s(postureCheckRole))

		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

		ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

		ctx.Req.NoError(err)

		t.Run("identity can see service via policies", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(enrolledIdentitySession.isServiceVisibleToUser(service.Id))
		})

		t.Run("service has the posture check in its queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			code, body := enrolledIdentitySession.query("/services/" + service.Id)
			ctx.Req.Equal(http.StatusOK, code)
			entityService, err := gabs.ParseJSON(body)
			ctx.Req.NoError(err)

			querySet, err := entityService.Path("data.postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(querySet, 1)

			postureQueries, err := querySet[0].Path("postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(postureQueries, 1)

			ctx.Req.Equal(*postureCheck.ID(), postureQueries[0].Path("id").Data().(string))
			ctx.Req.Equal(postureCheck.TypeID(), postureQueries[0].Path("queryType").Data().(string))

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
			})
		})

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
		})

		t.Run("providing valid posture data", func(t *testing.T) {
			ctx.testContextChanged(t)

			t.Run("by submitting it", func(t *testing.T) {
				ctx.testContextChanged(t)

				hash := process01.Hashes[0]
				postureResponse := &rest_model.PostureResponseProcessCreate{
					Hash:      hash,
					IsRunning: true,
					Path:      process01Path,
					SignerFingerprints: []string{
						process01.SignerFingerprints[0],
					},
				}
				postureResponse.SetID(postureCheck.ID())
				postureResponse.SetTypeID("PROCESS")

				enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)

				t.Run("allows a new session for the service can be created", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := enrolledIdentitySession.createNewSession(service.Id)
					ctx.Req.NoError(err)

					ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
				})
			})
		})
	})

	t.Run("can create a process all of multi posture check (anyOf, 1 process) associated to a service", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		processes := []*rest_model.ProcessMulti{
			process02,
		}

		postureCheck := ctx.AdminManagementSession.requireNewPostureCheckProcessMulti(rest_model.SemanticAnyOf, processes, s(postureCheckRole))

		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

		ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

		ctx.Req.NoError(err)

		t.Run("identity can see service via policies", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(enrolledIdentitySession.isServiceVisibleToUser(service.Id))
		})

		t.Run("service has the posture check in its queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			code, body := enrolledIdentitySession.query("/services/" + service.Id)
			ctx.Req.Equal(http.StatusOK, code)
			entityService, err := gabs.ParseJSON(body)
			ctx.Req.NoError(err)

			querySet, err := entityService.Path("data.postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(querySet, 1)

			postureQueries, err := querySet[0].Path("postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(postureQueries, 1)

			ctx.Req.Equal(*postureCheck.ID(), postureQueries[0].Path("id").Data().(string))
			ctx.Req.Equal(postureCheck.TypeID(), postureQueries[0].Path("queryType").Data().(string))

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
			})
		})

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
		})

		t.Run("providing valid posture data", func(t *testing.T) {
			ctx.testContextChanged(t)

			t.Run("by submitting it", func(t *testing.T) {
				ctx.testContextChanged(t)
				hash := hashes[1] //02 doesn't check hashes
				postureResponse := &rest_model.PostureResponseProcessCreate{
					Hash:      hash,
					IsRunning: true,
					Path:      process02Path,
					SignerFingerprints: []string{
						process02.SignerFingerprints[1],
					},
				}
				postureResponse.SetID(postureCheck.ID())
				postureResponse.SetTypeID("PROCESS")

				enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)

				t.Run("allows a new session for the service can be created", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := enrolledIdentitySession.createNewSession(service.Id)
					ctx.Req.NoError(err)

					ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
				})
			})
		})
	})

	t.Run("can create a process all of multi posture check (allOf, 4 processes) associated to a service", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		enrolledIdentityId, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		processes := []*rest_model.ProcessMulti{
			process01,
			process02,
			process03,
			process04,
		}

		processPostureCheck := ctx.AdminManagementSession.requireNewPostureCheckProcessMulti(rest_model.SemanticAllOf, processes, s(postureCheckRole))

		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

		ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

		ctx.Req.NoError(err)

		t.Run("identity can see service via policies", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(enrolledIdentitySession.isServiceVisibleToUser(service.Id))
		})

		t.Run("service has the posture check in its queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			code, body := enrolledIdentitySession.query("/services/" + service.Id)
			ctx.Req.Equal(http.StatusOK, code)
			entityService, err := gabs.ParseJSON(body)
			ctx.Req.NoError(err)

			querySet, err := entityService.Path("data.postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(querySet, 1)

			postureQueries, err := querySet[0].Path("postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(postureQueries, 1)

			ctx.Req.Equal(*processPostureCheck.ID(), postureQueries[0].Path("id").Data().(string))
			ctx.Req.Equal(processPostureCheck.TypeID(), postureQueries[0].Path("queryType").Data().(string))

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
			})
		})

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
		})

		t.Run("providing posture data", func(t *testing.T) {
			ctx.testContextChanged(t)

			t.Run("by submitting valid", func(t *testing.T) {
				ctx.testContextChanged(t)

				t.Run("posture 01", func(t *testing.T) {
					ctx.testContextChanged(t)

					hash := process01.Hashes[0]
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: true,
						Path:      process01Path,
						SignerFingerprints: []string{
							process01.SignerFingerprints[1],
						},
					}
					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)

					t.Run("doesn't allow a new session for the service can be created", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := enrolledIdentitySession.createNewSession(service.Id)
						ctx.Req.NoError(err)

						ctx.Req.Equal(http.StatusConflict, resp.StatusCode())

					})
				})

				t.Run("posture 02", func(t *testing.T) {
					ctx.testContextChanged(t)
					hash := hashes[3] //no hashes on 02's check shouldn't matter
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: true,
						Path:      process02Path,
						SignerFingerprints: []string{
							process02.SignerFingerprints[0],
						},
					}
					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)

					t.Run("doesn't allow a new session for the service can be created", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := enrolledIdentitySession.createNewSession(service.Id)
						ctx.Req.NoError(err)

						ctx.Req.Equal(http.StatusConflict, resp.StatusCode())

					})
				})

				t.Run("posture 03", func(t *testing.T) {
					ctx.testContextChanged(t)

					hash := process03.Hashes[1]
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: true,
						Path:      process03Path,
						SignerFingerprints: []string{
							signerFingerprints[3], //no signers on 03's check shoudln't matter
						},
					}

					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)

					t.Run("doesn't allow a new session for the service can be created", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := enrolledIdentitySession.createNewSession(service.Id)
						ctx.Req.NoError(err)

						ctx.Req.Equal(http.StatusConflict, resp.StatusCode())

					})
				})

				t.Run("posture 04", func(t *testing.T) {
					ctx.testContextChanged(t)
					hash := hashes[5] //no hashes on 04's check shouldn't matter
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: true,
						Path:      process04Path,
						SignerFingerprints: []string{
							signerFingerprints[2], //no signers on 04's check shouldn't matter
						},
					}
					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)

					t.Run("allows a new session for the service can be created", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := enrolledIdentitySession.createNewSession(service.Id)
						ctx.Req.NoError(err)

						ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
					})
				})
			})

			t.Run("by submitting invalid", func(t *testing.T) {
				ctx.testContextChanged(t)

				t.Run("posture 03 not running", func(t *testing.T) {
					ctx.testContextChanged(t)

					hash := process03.Hashes[1]
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: false,
						Path:      process03Path,
						SignerFingerprints: []string{
							signerFingerprints[3], //no signers on 03's check shouldn't matter
						},
					}

					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)

					t.Run("doesn't allow a new session for the service can be created", func(t *testing.T) {
						ctx.testContextChanged(t)
						resp, err := enrolledIdentitySession.createNewSession(service.Id)
						ctx.Req.NoError(err)

						ctx.Req.Equal(http.StatusConflict, resp.StatusCode())

					})
				})
			})
		})

		resp, err := ctx.AdminManagementSession.newAuthenticatedRequest().Get("/identities/" + enrolledIdentityId + "/failed-service-requests")
		ctx.Req.NoError(err)
		ctx.Req.NotNil(resp)
	})

	t.Run("can create a process all of multi posture check (anyOf, 4 processes) associated to a service", func(t *testing.T) {
		ctx.testContextChanged(t)

		identityRole := eid.New()
		serviceRole := eid.New()
		postureCheckRole := eid.New()

		_, enrolledIdentityAuthenticator := ctx.AdminManagementSession.requireCreateIdentityOttEnrollment(eid.New(), false, identityRole)
		enrolledIdentitySession, err := enrolledIdentityAuthenticator.AuthenticateClientApi(ctx)

		ctx.Req.NoError(err)

		service := ctx.AdminManagementSession.requireNewService(s(serviceRole), nil)

		processes := []*rest_model.ProcessMulti{
			process01,
			process02,
			process03,
			process04,
		}

		processPostureCheck := ctx.AdminManagementSession.requireNewPostureCheckProcessMulti(rest_model.SemanticAnyOf, processes, s(postureCheckRole))

		ctx.AdminManagementSession.requireNewServicePolicyWithSemantic("Dial", "AllOf", s("#"+serviceRole), s("#"+identityRole), s("#"+postureCheckRole))

		ctx.AdminManagementSession.requireNewEdgeRouterPolicy(s("#all"), s("#"+identityRole))

		ctx.AdminManagementSession.requireNewServiceEdgeRouterPolicy(s("#all"), s("#"+serviceRole))

		ctx.Req.NoError(err)

		t.Run("identity can see service via policies", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.True(enrolledIdentitySession.isServiceVisibleToUser(service.Id))
		})

		t.Run("service has the posture check in its queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			code, body := enrolledIdentitySession.query("/services/" + service.Id)
			ctx.Req.Equal(http.StatusOK, code)
			entityService, err := gabs.ParseJSON(body)
			ctx.Req.NoError(err)

			querySet, err := entityService.Path("data.postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(querySet, 1)

			postureQueries, err := querySet[0].Path("postureQueries").Children()
			ctx.Req.NoError(err)
			ctx.Req.Len(postureQueries, 1)

			ctx.Req.Equal(*processPostureCheck.ID(), postureQueries[0].Path("id").Data().(string))
			ctx.Req.Equal(processPostureCheck.TypeID(), postureQueries[0].Path("queryType").Data().(string))

			t.Run("query is currently failing", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.False(querySet[0].Path("isPassing").Data().(bool))
				ctx.Req.False(postureQueries[0].Path("isPassing").Data().(bool))
			})
		})

		t.Run("cannot create session with failing queries", func(t *testing.T) {
			ctx.testContextChanged(t)
			resp, err := enrolledIdentitySession.createNewSession(service.Id)
			ctx.Req.NoError(err)

			ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
		})

		t.Run("providing posture data", func(t *testing.T) {
			ctx.testContextChanged(t)

			t.Run("by submitting invalid", func(t *testing.T) {
				ctx.testContextChanged(t)

				t.Run("posture 01", func(t *testing.T) {
					ctx.testContextChanged(t)

					hash := "aaaaaaaaaaaaaaaaaaaa"
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: true,
						Path:      process01Path,
						SignerFingerprints: []string{
							"aaaaaaaaaaaaaaaaaaaaaa",
						},
					}

					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)
				})

				t.Run("posture 02", func(t *testing.T) {
					ctx.testContextChanged(t)

					hash := hashes[3] //no hashes on 02's check shouldn't matter
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: true,
						Path:      process02Path,
						SignerFingerprints: []string{
							"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						},
					}

					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)
				})

				t.Run("posture 03", func(t *testing.T) {
					ctx.testContextChanged(t)

					hash := "aaaaaaaaaaaa"
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: true,
						Path:      process03Path,
						SignerFingerprints: []string{
							signerFingerprints[3], //no signers on 03's check shoudln't matter
						},
					}

					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)
				})

				t.Run("posture 04", func(t *testing.T) {
					ctx.testContextChanged(t)

					hash := hashes[5] //no hashes on 04's check shouldn't matter
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: false,
						Path:      process04Path,
						SignerFingerprints: []string{
							signerFingerprints[2], //no signers on 04's check shouldn't matter
						},
					}

					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)
				})
			})

			t.Run("doesn't allow a new session for the service can be created", func(t *testing.T) {
				ctx.testContextChanged(t)
				resp, err := enrolledIdentitySession.createNewSession(service.Id)
				ctx.Req.NoError(err)

				ctx.Req.Equal(http.StatusConflict, resp.StatusCode())
			})

			t.Run("by submitting 1 valid posture", func(t *testing.T) {
				ctx.testContextChanged(t)

				t.Run("posture 04", func(t *testing.T) {
					ctx.testContextChanged(t)

					hash := hashes[5] //no hashes on 04's check shouldn't matter
					postureResponse := &rest_model.PostureResponseProcessCreate{
						Hash:      hash,
						IsRunning: true,
						Path:      process04Path,
						SignerFingerprints: []string{
							signerFingerprints[2], //no signers on 04's check shouldn't matter
						},
					}

					postureResponse.SetID(processPostureCheck.ID())
					postureResponse.SetTypeID("PROCESS")

					enrolledIdentitySession.requireCreateRestModelPostureResponse(postureResponse)
				})

				t.Run("allows a new session for the service can be created", func(t *testing.T) {
					ctx.testContextChanged(t)
					resp, err := enrolledIdentitySession.createNewSession(service.Id)
					ctx.Req.NoError(err)

					ctx.Req.Equal(http.StatusCreated, resp.StatusCode())
				})
			})
		})
	})

	t.Run("a PATCH operation against a process multi posture check", func(t *testing.T) {
		ctx.testContextChanged(t)

		originalProcesses := []*rest_model.ProcessMulti{
			process01,
			process04,
		}

		patchProcesses := []*rest_model.ProcessMulti{
			process02,
			process03,
			process04,
		}

		originalCheck := ctx.AdminManagementSession.requireNewPostureCheckProcessMulti(rest_model.SemanticAnyOf, originalProcesses, nil)

		patchCheck := rest_model.PostureCheckProcessMultiPatch{
			Processes: patchProcesses,
			Semantic:  rest_model.SemanticAllOf,
		}

		newName := uuid.New().String()
		patchCheck.SetName(newName)
		patchCheck.SetTypeID(rest_model.PostureCheckTypePROCESSMULTI)

		patchResp, patchErr := ctx.AdminManagementSession.newAuthenticatedRequestWithBody(patchCheck).Patch("posture-checks/" + *originalCheck.ID())

		t.Run("succeeds", func(t *testing.T) {
			ctx.testContextChanged(t)
			ctx.Req.NoError(patchErr)
			standardJsonResponseTests(patchResp, 200, t)
		})

		t.Run("can retrieve values properly", func(t *testing.T) {
			ctx.testContextChanged(t)
			getResp, getErr := ctx.AdminManagementSession.newAuthenticatedRequest().Get("posture-checks/" + *originalCheck.ID())
			ctx.Req.NoError(getErr)
			envelope := rest_model.DetailPostureCheckEnvelope{
				Meta: &rest_model.Meta{},
			}

			err := envelope.UnmarshalJSON(getResp.Body())

			t.Run("response can be unmarshalled", func(t *testing.T) {
				ctx.testContextChanged(t)
				ctx.Req.NoError(err)

				data := envelope.Data()
				getCheck := data.(*rest_model.PostureCheckProcessMultiDetail)
				ctx.Req.NotNil(getCheck)

				t.Run("retrieved values has the correct name", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.Equal(newName, *getCheck.Name())
				})
				t.Run("retrieved values has the correct semantic", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.Equal(rest_model.SemanticAllOf, *getCheck.Semantic)
				})

				t.Run("retrieved values has the correct number of processes", func(t *testing.T) {
					ctx.testContextChanged(t)
					ctx.Req.Len(getCheck.Processes, len(patchCheck.Processes))
				})

				t.Run("retrieved values has the correct processes", func(t *testing.T) {
					ctx.testContextChanged(t)

					for _, patchProcess := range patchProcesses {
						isMatched := false
						for _, getProcess := range getCheck.Processes {
							if *getProcess.OsType == *patchProcess.OsType && *getProcess.Path == *patchProcess.Path {
								isMatched = true
								ctx.Req.ElementsMatch(patchProcess.Hashes, getProcess.Hashes)
								ctx.Req.ElementsMatch(patchProcess.SignerFingerprints, getProcess.SignerFingerprints)
								break
							}
						}
						ctx.Req.True(isMatched, "a process applied via patch was not found in subsequent get: %s, %s, %v, %v", *patchProcess.OsType, *patchProcess.Path, patchProcess.Hashes, patchProcess.SignerFingerprints)
					}
				})
			})
		})
	})
}
