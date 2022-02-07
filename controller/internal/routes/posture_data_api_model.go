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

package routes

import (
	"github.com/go-openapi/strfmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge/controller/env"
	"github.com/openziti/edge/controller/model"
	"github.com/openziti/edge/rest_model"
	"time"
)

func MapPostureDataToRestModel(_ *env.AppEnv, postureData *model.PostureData) *rest_model.PostureData {
	ret := &rest_model.PostureData{
		Domain:                MapPostureDataDomainToRestModel(&postureData.Domain),
		Mac:                   MapPostureDataMacToRestModel(&postureData.Mac),
		Os:                    MapPostureDataOsToRestModel(&postureData.Os),
		APISessionPostureData: MapPostureDataApiSessionDataToRestModel(postureData.ApiSessions),
		Processes:             MapPostureDataProcessesToRestModel(postureData),
	}

	return ret
}

func MapPostureDataProcessesToRestModel(postureData *model.PostureData) []*rest_model.PostureDataProcess {
	processes := []*rest_model.PostureDataProcess{}

	for _, genericProcess := range postureData.Processes {
		process := &rest_model.PostureDataProcess{
			PostureDataBase: rest_model.PostureDataBase{
				LastUpdatedAt:  toStrFmtDateTimeP(genericProcess.LastUpdatedAt),
				PostureCheckID: &genericProcess.PostureCheckId,
				TimedOut:       &genericProcess.TimedOut,
			},
		}

		if pdProcess, ok := genericProcess.SubType.(*model.PostureResponseProcess); ok {
			process.IsRunning = pdProcess.IsRunning
			process.SignerFingerprints = pdProcess.SignerFingerprints
			process.BinaryHash = pdProcess.BinaryHash
		} else {
			pfxlog.Logger().Errorf("could not convery posture data process (%s) to subtype, got %T", genericProcess.PostureCheckId, genericProcess.SubType)
		}

		processes = append(processes, process)
	}

	return processes
}

func MapPostureDataDomainToRestModel(domain *model.PostureResponseDomain) *rest_model.PostureDataDomain {
	ret := &rest_model.PostureDataDomain{
		PostureDataBase: rest_model.PostureDataBase{
			LastUpdatedAt:  toStrFmtDateTimeP(domain.LastUpdatedAt),
			PostureCheckID: &domain.PostureCheckId,
			TimedOut:       &domain.TimedOut,
		},
	}

	if pdDomain, ok := domain.SubType.(*model.PostureResponseDomain); ok {
		ret.Domain = &pdDomain.Name
	} else {
		pfxlog.Logger().Errorf("could not convery posture data domain to subtype, got %T", domain.SubType)
	}

	return ret
}

func MapPostureDataMacToRestModel(mac *model.PostureResponseMac) *rest_model.PostureDataMac {
	ret := &rest_model.PostureDataMac{
		PostureDataBase: rest_model.PostureDataBase{
			LastUpdatedAt:  toStrFmtDateTimeP(mac.LastUpdatedAt),
			PostureCheckID: &mac.PostureCheckId,
			TimedOut:       &mac.TimedOut,
		},
	}

	if pdMac, ok := mac.SubType.(*model.PostureResponseMac); ok {
		ret.Addresses = pdMac.Addresses
	} else {
		pfxlog.Logger().Errorf("could not convery posture data mac to subtype, got %T", mac.SubType)
	}

	return ret
}

func MapPostureDataOsToRestModel(os *model.PostureResponseOs) *rest_model.PostureDataOs {
	ret := &rest_model.PostureDataOs{
		PostureDataBase: rest_model.PostureDataBase{
			LastUpdatedAt:  toStrFmtDateTimeP(os.LastUpdatedAt),
			PostureCheckID: &os.PostureCheckId,
			TimedOut:       &os.TimedOut,
		},
	}

	if pdOs, ok := os.SubType.(*model.PostureResponseOs); ok {
		ret.Type = &pdOs.Type
		ret.Version = &pdOs.Version
	} else {
		pfxlog.Logger().Errorf("could not convery posture data mac to subtype, got %T", os.SubType)
	}

	return ret
}

func MapPostureDataApiSessionDataToRestModel(apiSessionData map[string]*model.ApiSessionPostureData) map[string]rest_model.APISessionPostureData {
	ret := map[string]rest_model.APISessionPostureData{}

	for apiSessionId, apiSessionData := range apiSessionData {
		apiSessionPostureData := rest_model.APISessionPostureData{}

		passedMfa := apiSessionData.Mfa != nil && apiSessionData.Mfa.PassedMfaAt != nil
		apiSessionPostureData.Mfa = &rest_model.PostureDataMfa{
			APISessionID: &apiSessionId,
			PassedMfa:    &passedMfa,
		}

		apiSessionPostureData.EndpointState = &rest_model.PostureDataEndpointState{
			UnlockedAt: nil,
			WokenAt:    nil,
		}

		if apiSessionData.EndpointState != nil {
			if apiSessionData.EndpointState.UnlockedAt != nil {
				formattedDate := strfmt.DateTime(*apiSessionData.EndpointState.UnlockedAt)
				apiSessionPostureData.EndpointState.UnlockedAt = &formattedDate
			}

			if apiSessionData.EndpointState.WokenAt != nil {
				formattedDate := strfmt.DateTime(*apiSessionData.EndpointState.WokenAt)
				apiSessionPostureData.EndpointState.WokenAt = &formattedDate
			}
		}

		if apiSessionData.SdkInfo != nil {
			apiSessionPostureData.SdkInfo = &rest_model.SdkInfo{
				AppID:      apiSessionData.SdkInfo.AppId,
				AppVersion: apiSessionData.SdkInfo.AppVersion,
				Branch:     apiSessionData.SdkInfo.Branch,
				Revision:   apiSessionData.SdkInfo.Revision,
				Type:       apiSessionData.SdkInfo.Type,
				Version:    apiSessionData.SdkInfo.Version,
			}
		}

		ret[apiSessionId] = apiSessionPostureData
	}

	return ret
}

func MapPostureCheckFailureProcessToRestModel(failure *model.PostureCheckFailure) *rest_model.PostureCheckFailureProcess {
	ret := &rest_model.PostureCheckFailureProcess{
		ActualValue:   &rest_model.PostureCheckFailureProcessActual{},
		ExpectedValue: &rest_model.Process{},
	}
	ret.SetPostureCheckID(&failure.PostureCheckId)
	ret.SetPostureCheckType(failure.PostureCheckType)
	ret.SetPostureCheckName(&failure.PostureCheckName)

	if val, ok := failure.PostureCheckFailureValues.(*model.PostureCheckFailureValuesProcess); ok {
		ret.ActualValue.Hash = &val.ActualValue.BinaryHash
		ret.ActualValue.SignerFingerprints = val.ActualValue.SignerFingerprints
		ret.ActualValue.IsRunning = &val.ActualValue.IsRunning

		osType := rest_model.OsType(val.ExpectedValue.OsType)
		ret.ExpectedValue.Hashes = val.ExpectedValue.Hashes
		ret.ExpectedValue.Path = &val.ExpectedValue.Path
		ret.ExpectedValue.OsType = &osType
		ret.ExpectedValue.SignerFingerprint = val.ExpectedValue.Fingerprint
	} else {
		pfxlog.Logger().Errorf("could not convert failure values of %T for posture check %s, expected process", failure.PostureCheckFailureValues, failure.PostureCheckId)
	}

	return ret
}

func MapPostureCheckFailureProcessMultiToRestModel(failure *model.PostureCheckFailure) *rest_model.PostureCheckFailureProcessMulti {
	restResult := &rest_model.PostureCheckFailureProcessMulti{
		ActualValue:   []*rest_model.PostureCheckFailureProcessActual{},
		ExpectedValue: []*rest_model.ProcessMulti{},
	}

	restResult.SetPostureCheckID(&failure.PostureCheckId)
	restResult.SetPostureCheckType(failure.PostureCheckType)
	restResult.SetPostureCheckName(&failure.PostureCheckName)

	if val, ok := failure.PostureCheckFailureValues.(*model.PostureCheckFailureValuesProcessMulti); ok {
		semantic := rest_model.Semantic(val.ExpectedValue.Semantic)
		restResult.Semantic = &semantic

		for _, valActual := range val.ActualValue {
			restActual := &rest_model.PostureCheckFailureProcessActual{
				Hash:               &valActual.BinaryHash,
				IsRunning:          &valActual.IsRunning,
				SignerFingerprints: valActual.SignerFingerprints,
				Path:               valActual.Path,
			}

			restResult.ActualValue = append(restResult.ActualValue, restActual)
		}

		for _, valExpected := range val.ExpectedValue.Processes {
			osType := rest_model.OsType(valExpected.OsType)
			restExpected := &rest_model.ProcessMulti{
				Hashes:             valExpected.Hashes,
				OsType:             &osType,
				Path:               &valExpected.Path,
				SignerFingerprints: valExpected.SignerFingerprints,
			}

			restResult.ExpectedValue = append(restResult.ExpectedValue, restExpected)
		}
	} else {
		pfxlog.Logger().Errorf("could not convert failure values of %T for posture check %s, expected process multi", failure.PostureCheckFailureValues, failure.PostureCheckId)
	}

	return restResult
}

func MapPostureCheckFailureDomainToRestModel(failure *model.PostureCheckFailure) *rest_model.PostureCheckFailureDomain {
	ret := &rest_model.PostureCheckFailureDomain{
		ActualValue:   nil,
		ExpectedValue: []string{},
	}
	ret.SetPostureCheckID(&failure.PostureCheckId)
	ret.SetPostureCheckType(failure.PostureCheckType)
	ret.SetPostureCheckName(&failure.PostureCheckName)

	if val, ok := failure.PostureCheckFailureValues.(*model.PostureCheckFailureValuesDomain); ok {
		ret.ActualValue = &val.ActualValue
		ret.ExpectedValue = val.ExpectedValue
	} else {
		pfxlog.Logger().Errorf("could not convert failure values of %T for posture check %s, expected domain", failure.PostureCheckFailureValues, failure.PostureCheckId)
	}

	return ret
}

func MapPostureCheckFailureMacToRestModel(failure *model.PostureCheckFailure) *rest_model.PostureCheckFailureMacAddress {
	ret := &rest_model.PostureCheckFailureMacAddress{
		ActualValue:   nil,
		ExpectedValue: []string{},
	}
	ret.SetPostureCheckID(&failure.PostureCheckId)
	ret.SetPostureCheckType(failure.PostureCheckType)
	ret.SetPostureCheckName(&failure.PostureCheckName)

	if val, ok := failure.PostureCheckFailureValues.(*model.PostureCheckFailureValuesMac); ok {
		ret.ActualValue = val.ActualValue
		ret.ExpectedValue = val.ExpectedValue
	} else {
		pfxlog.Logger().Errorf("could not convert failure values of %T for posture check %s, expected mac", failure.PostureCheckFailureValues, failure.PostureCheckId)
	}

	return ret
}

func MapPostureCheckFailureOsToRestModel(failure *model.PostureCheckFailure) *rest_model.PostureCheckFailureOperatingSystem {
	ret := &rest_model.PostureCheckFailureOperatingSystem{
		ActualValue:   &rest_model.PostureCheckFailureOperatingSystemActual{},
		ExpectedValue: nil,
	}
	ret.SetPostureCheckID(&failure.PostureCheckId)
	ret.SetPostureCheckType(failure.PostureCheckType)
	ret.SetPostureCheckName(&failure.PostureCheckName)

	if val, ok := failure.PostureCheckFailureValues.(*model.PostureCheckFailureValuesOperatingSystem); ok {
		ret.ActualValue.Type = &val.ActualValue.Type
		ret.ActualValue.Version = &val.ActualValue.Version

		ret.ExpectedValue = []*rest_model.OperatingSystem{}

		for _, os := range val.ExpectedValue {
			osType := rest_model.OsType(os.OsType)

			newOs := &rest_model.OperatingSystem{
				Type:     &osType,
				Versions: os.OsVersions,
			}
			ret.ExpectedValue = append(ret.ExpectedValue, newOs)
		}
	} else {
		pfxlog.Logger().Errorf("could not convert failure values of %T for posture check %s, expected mfa", failure.PostureCheckFailureValues, failure.PostureCheckId)
	}

	return ret
}

func MapPostureCheckFailureMfaToRestModel(failure *model.PostureCheckFailure) *rest_model.PostureCheckFailureMfa {
	ret := &rest_model.PostureCheckFailureMfa{
		ActualValue:   nil,
		ExpectedValue: nil,
	}
	ret.SetPostureCheckID(&failure.PostureCheckId)
	ret.SetPostureCheckType(failure.PostureCheckType)
	ret.SetPostureCheckName(&failure.PostureCheckName)

	if val, ok := failure.PostureCheckFailureValues.(*model.PostureCheckFailureValuesMfa); ok {
		ret.ActualValue = &rest_model.PostureChecksFailureMfaValues{
			PassedMfa:      val.ActualValue.PassedMfa,
			PassedOnUnlock: val.ActualValue.PassedOnUnlock,
			PassedOnWake:   val.ActualValue.PassedOnWake,
			TimedOut:       val.ActualValue.TimedOutSeconds,
		}
		ret.ExpectedValue = &rest_model.PostureChecksFailureMfaValues{
			PassedMfa:      val.ExpectedValue.PassedMfa,
			PassedOnUnlock: val.ExpectedValue.PassedOnUnlock,
			PassedOnWake:   val.ExpectedValue.PassedOnWake,
			TimedOut:       val.ExpectedValue.TimedOutSeconds,
		}

		ret.Criteria = &rest_model.PostureChecksFailureMfaCriteria{
			PassedMfaAt:             DateTimePtrOrNil(val.Criteria.PassedMfaAt),
			TimeoutSeconds:          &val.Criteria.TimeoutSeconds,
			TimeoutRemainingSeconds: &val.Criteria.TimeoutRemainingSeconds,
			UnlockedAt:              DateTimePtrOrNil(val.Criteria.UnlockedAt),
			WokenAt:                 DateTimePtrOrNil(val.Criteria.WokenAt),
		}
	} else {
		pfxlog.Logger().Errorf("could not convert failure values of %T for posture check %s, expected mfa", failure.PostureCheckFailureValues, failure.PostureCheckId)
	}
	return ret
}

func MapPostureDataFailedSessionRequestToRestModel(modelFailedSessionRequests []*model.PostureSessionRequestFailure) []*rest_model.FailedServiceRequest {
	ret := []*rest_model.FailedServiceRequest{}

	for _, modelFailedSessionRequest := range modelFailedSessionRequests {
		failedSessionRequest := &rest_model.FailedServiceRequest{
			APISessionID:   modelFailedSessionRequest.ApiSessionId,
			PolicyFailures: []*rest_model.PolicyFailure{},
			ServiceID:      modelFailedSessionRequest.ServiceId,
			ServiceName:    modelFailedSessionRequest.ServiceName,
			SessionType:    rest_model.DialBind(modelFailedSessionRequest.SessionType),
			When:           toStrFmtDateTime(modelFailedSessionRequest.When),
		}

		for _, modelPolicyFailure := range modelFailedSessionRequest.PolicyFailures {
			policyFailure := &rest_model.PolicyFailure{
				PolicyID:   modelPolicyFailure.PolicyId,
				PolicyName: modelPolicyFailure.PolicyName,
			}

			checks := []rest_model.PostureCheckFailure{}

			for _, pdCheck := range modelPolicyFailure.Checks {
				switch pdCheck.PostureCheckType {
				case string(rest_model.PostureCheckTypePROCESS):
					checks = append(checks, MapPostureCheckFailureProcessToRestModel(pdCheck))
				case string(rest_model.PostureCheckTypePROCESSMULTI):
					checks = append(checks, MapPostureCheckFailureProcessMultiToRestModel(pdCheck))
				case string(rest_model.PostureCheckTypeDOMAIN):
					checks = append(checks, MapPostureCheckFailureDomainToRestModel(pdCheck))
				case string(rest_model.PostureCheckTypeMAC):
					checks = append(checks, MapPostureCheckFailureMacToRestModel(pdCheck))
				case string(rest_model.PostureCheckTypeOS):
					checks = append(checks, MapPostureCheckFailureOsToRestModel(pdCheck))
				case string(rest_model.PostureCheckTypeMFA):
					checks = append(checks, MapPostureCheckFailureMfaToRestModel(pdCheck))
				}
				policyFailure.SetChecks(checks)
			}

			failedSessionRequest.PolicyFailures = append(failedSessionRequest.PolicyFailures, policyFailure)
		}

		ret = append(ret, failedSessionRequest)
	}

	return ret
}

func toStrFmtDateTime(time time.Time) strfmt.DateTime {
	return strfmt.DateTime(time)
}

func toStrFmtDateTimeP(time time.Time) *strfmt.DateTime {
	if time.IsZero() {
		return nil
	}

	ret := strfmt.DateTime(time)
	return &ret
}
