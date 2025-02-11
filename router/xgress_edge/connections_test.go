/*
	Copyright NetFoundry Inc.

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

package xgress_edge

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/openziti/ziti/common"
	"github.com/openziti/ziti/common/pb/edge_ctrl_pb"
	"github.com/openziti/ziti/router/state"
	"github.com/stretchr/testify/require"
	"net/url"
	"testing"
)

func Test_validateBySpiffeId(t *testing.T) {

	t.Run("validates if SPIFFE IDs match", func(t *testing.T) {
		req := require.New(t)

		handler := &sessionConnectionHandler{
			stateManager:                     nil,
			options:                          nil,
			invalidApiSessionToken:           nil,
			invalidApiSessionTokenDuringSync: nil,
		}

		clientCertPem := `-----BEGIN CERTIFICATE-----
MIIEfTCCAmWgAwIBAgIDA17WMA0GCSqGSIb3DQEBCwUAMIGYMQswCQYDVQQGEwJV
UzELMAkGA1UECBMCTkMxEjAQBgNVBAcTCUNoYXJsb3R0ZTETMBEGA1UEChMKTmV0
Rm91bmRyeTEtMCsGA1UEAxMkMmJiZGMwZjEtMzAyYi00OWMwLWFkNjUtNmRmYmFk
YWQzZjY3MSQwIgYJKoZIhvcNAQkBFhVzdXBwb3J0QG5ldGZvdW5kcnkuaW8wHhcN
MjUwMjA3MTYzMjQyWhcNMjUwMjA4MDQzMjQyWjAAMIGbMBAGByqGSM49AgEGBSuB
BAAjA4GGAAQALyJHEbQURk4dP2xJuO9cZE4zg3ynh4Gty6J04Ci8uDIqZcKmEgzT
t8MAKP38x8McwItcp6XvQbi151RHavTVfpsAfvb3/yxkhKgZ80AWhksJTKe9Zn0D
3l9DakCW4sB6WoMmqfdY5167RBcNT+msD3FnnBgirmb1ONoZTD16nzXDlOKjge4w
geswDgYDVR0PAQH/BAQDAgSwMBMGA1UdJQQMMAoGCCsGAQUFBwMCMB8GA1UdIwQY
MBaAFKXvlYnB9SeiJTKsx3rpSZENEl5cMIGiBgNVHREBAf8EgZcwgZSGgZFzcGlm
ZmU6Ly80OTAxYjI1ZC01OTAxLTQwNmItYmNmNi1kZTRjZGIwYTQ3MzcvaWRlbnRp
dHkvOHhVT09TT3QxUC9hcGlTZXNzaW9uLzc0ZThmZTQ5LTIzMTQtNDkzYS1hMTFm
LWVlYzU3NmM4MDQ4Ny9hcGlTZXNzaW9uQ2VydGlmaWNhdGUvSG4zNEJ1NFlkMA0G
CSqGSIb3DQEBCwUAA4ICAQBST9ptpbkCYRQ7O7VFOPB0zMEBGHx2OYj+ukbILP9f
OLbnC9zB7ACQXSxgAteIaX00/mM6q2W/BWhVby+BtO83TiUYHlwnsWwltjsos29p
0V0+Y/4H54dstGDjfRa9uNx7cIEVT/L4apDfr3IqVzuoCaSm0gN098LxYTD7PbiK
jQwaEWTnn/MroefI4gdVmCSnt8tP+0e+6TOd3a6im6MmZtybGw2CgSsQR71MxIWI
mwazyV8bSYzmajRwI8BTgPJeWG8m0rE1f1huQEJeXYLkzVMRf8zJZGDtRhwPu5cl
GzEEFkDa8+STSfubtP2V+A7NpZ9DVqdNRCJ8kH0RiJQhQAkDTOerFMTakNw96m0C
oxyMrVPVsQkU4Lt5wfnNbmzG8eQZyJz72iDWfCUctuEczeOP1jR1woSYHsUnXii1
TBk/F7z3vD5RGB4ERUhZjeEFqOuqFJAvUgZyWKERNfMPQ1bpz6CWt7paDDSBalgc
zv2TUTiS6Kp55/WtjIlb1b1xo8NoM9CDWTJMSdF3d75d88eOb7pYUzC9VummVZM/
BTpWu19hCcj8j598wBoVfIi9RpA9v99PvYxyiYBzX2pNIL55pHhugwRTh8eoKtF2
k4T8+LWz8X/pW6Ig5JSlDQTJ6hZs8rMdvoTV/q/re3JTQNMh0aar/WLPfPlOVoM4
OA==
-----END CERTIFICATE-----
`

		clientJwtStr := `eyJhbGciOiJSUzI1NiIsImtpZCI6IjAwNzY4ZTkxOTg2MTI3NjJiNzQzZTA2MmVjZDU1MDI1NTQzMTZlYmEiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOlsib3BlbnppdGkiXSwiZXhwIjoxNzM4OTQ3NzYxLCJpYXQiOjE3Mzg5NDU5NjEsImlzcyI6Imh0dHBzOi8vMmJiZGMwZjEtMzAyYi00OWMwLWFkNjUtNmRmYmFkYWQzZjY3LXAuc3RhZ2luZy5uZXRmb3VuZHJ5LmlvOjQ0My9vaWRjIiwianRpIjoiNWViZjM3MWEtM2VhMC00MzY4LTliZTQtNzFhN2E2MDUwNDhiIiwibmJmIjoxNzM4OTQ1OTYxLCJzY29wZXMiOlsib3BlbmlkIiwib2ZmbGluZV9hY2Nlc3MiXSwic3ViIjoiOHhVT09TT3QxUCIsInpfYWlkIjoib3BlbnppdGkiLCJ6X2FzaWQiOiI3NGU4ZmU0OS0yMzE0LTQ5M2EtYTExZi1lZWM1NzZjODA0ODciLCJ6X2NmcyI6bnVsbCwiel9laWQiOiJjdXJ0LnR1ZG9yQG5ldGZvdW5kcnkuaW8iLCJ6X2VudiI6bnVsbCwiel9pY2UiOmZhbHNlLCJ6X3JhIjoiNzMuMzQuMTk0LjIxMjo1MzIyMSIsInpfc2RrIjpudWxsLCJ6X3QiOiJhIn0.aOC1cq1U1tO5w0Z1gTRtV6s1Ay-lLBsZjgLHUAWUmQnuw4Ak6N7uJSg28GCS0xna-LSLRhpLeicHXyxfBD1QnC2oi9kgcOAOAtYyFJLGpfBXSQC2Z4gQIR2xR5inT6F-5NtR53hUmxQvwdYBgQEG_jxPmhtx2A3vHf2puw0XheFao_IQCTQH67KzFW_k33saLZyDKEU33N-x5m0LCuKuozNwsh7KIpANbAFLGtlLpnhaC1V12BKLYPvHx4iwM2F7bUqzXUcz5DR0SAlOxsoOSPhYCsUxGItYAGxMZKITj09JGI_oV4aj2OmjYz_xWRU6B25seZQs7jq0cPeI5ojESMoJ1Q5jTu0T29BpK0jJ7jkO4p6-1BDLKClqDEAsWBRJmoEZJAXVFSYyNUOvFfK46_GqgoLl3NMGJAPcfztQh-zIeT74qOb_KhRLccFqTvcK8CngJ2Y8NWm9AuS1YKmGpdO6DbXw2Q2mtTs1nvQxbZlEQjhGAkKEVVtZqfYCK5ysyYAGtGpu5wn1i4IyxACQKxCmTjatrc6hqcxNbZe3zaPhVESA-oTNrvt0kjloNCMTtfWQZg3hQ91_gOikza1E-lim89GQp1PhMZzfzYaQuTLzZwkYmHGu3ws5sB5aZ17o3xAozJ_W7sIY_CEuv939LnpLnQUj2vCrmcFpuuB8KDQ`

		block, _ := pem.Decode([]byte(clientCertPem))

		req.NotNil(block)

		clientCert, err := x509.ParseCertificate(block.Bytes)
		req.NoError(err)
		req.NotNil(clientCert)

		jwtParser := jwt.NewParser()
		accessClaims := &common.AccessClaims{}

		token, _, err := jwtParser.ParseUnverified(clientJwtStr, accessClaims)
		req.NoError(err)

		apiSession := &state.ApiSession{
			ApiSession: &edge_ctrl_pb.ApiSession{
				Token:            clientJwtStr,
				CertFingerprints: nil,
				Id:               accessClaims.ApiSessionId,
				IdentityId:       accessClaims.Subject,
			},
			JwtToken:     token,
			Claims:       accessClaims,
			ControllerId: "",
		}

		result := handler.validateBySpiffeId(apiSession, clientCert)

		req.True(result)
	})

	t.Run("does not validate if SPIFFE IDs do not match", func(t *testing.T) {
		req := require.New(t)

		handler := &sessionConnectionHandler{
			stateManager:                     nil,
			options:                          nil,
			invalidApiSessionToken:           nil,
			invalidApiSessionTokenDuringSync: nil,
		}

		clientCertPem := `-----BEGIN CERTIFICATE-----
MIIEfTCCAmWgAwIBAgIDA17WMA0GCSqGSIb3DQEBCwUAMIGYMQswCQYDVQQGEwJV
UzELMAkGA1UECBMCTkMxEjAQBgNVBAcTCUNoYXJsb3R0ZTETMBEGA1UEChMKTmV0
Rm91bmRyeTEtMCsGA1UEAxMkMmJiZGMwZjEtMzAyYi00OWMwLWFkNjUtNmRmYmFk
YWQzZjY3MSQwIgYJKoZIhvcNAQkBFhVzdXBwb3J0QG5ldGZvdW5kcnkuaW8wHhcN
MjUwMjA3MTYzMjQyWhcNMjUwMjA4MDQzMjQyWjAAMIGbMBAGByqGSM49AgEGBSuB
BAAjA4GGAAQALyJHEbQURk4dP2xJuO9cZE4zg3ynh4Gty6J04Ci8uDIqZcKmEgzT
t8MAKP38x8McwItcp6XvQbi151RHavTVfpsAfvb3/yxkhKgZ80AWhksJTKe9Zn0D
3l9DakCW4sB6WoMmqfdY5167RBcNT+msD3FnnBgirmb1ONoZTD16nzXDlOKjge4w
geswDgYDVR0PAQH/BAQDAgSwMBMGA1UdJQQMMAoGCCsGAQUFBwMCMB8GA1UdIwQY
MBaAFKXvlYnB9SeiJTKsx3rpSZENEl5cMIGiBgNVHREBAf8EgZcwgZSGgZFzcGlm
ZmU6Ly80OTAxYjI1ZC01OTAxLTQwNmItYmNmNi1kZTRjZGIwYTQ3MzcvaWRlbnRp
dHkvOHhVT09TT3QxUC9hcGlTZXNzaW9uLzc0ZThmZTQ5LTIzMTQtNDkzYS1hMTFm
LWVlYzU3NmM4MDQ4Ny9hcGlTZXNzaW9uQ2VydGlmaWNhdGUvSG4zNEJ1NFlkMA0G
CSqGSIb3DQEBCwUAA4ICAQBST9ptpbkCYRQ7O7VFOPB0zMEBGHx2OYj+ukbILP9f
OLbnC9zB7ACQXSxgAteIaX00/mM6q2W/BWhVby+BtO83TiUYHlwnsWwltjsos29p
0V0+Y/4H54dstGDjfRa9uNx7cIEVT/L4apDfr3IqVzuoCaSm0gN098LxYTD7PbiK
jQwaEWTnn/MroefI4gdVmCSnt8tP+0e+6TOd3a6im6MmZtybGw2CgSsQR71MxIWI
mwazyV8bSYzmajRwI8BTgPJeWG8m0rE1f1huQEJeXYLkzVMRf8zJZGDtRhwPu5cl
GzEEFkDa8+STSfubtP2V+A7NpZ9DVqdNRCJ8kH0RiJQhQAkDTOerFMTakNw96m0C
oxyMrVPVsQkU4Lt5wfnNbmzG8eQZyJz72iDWfCUctuEczeOP1jR1woSYHsUnXii1
TBk/F7z3vD5RGB4ERUhZjeEFqOuqFJAvUgZyWKERNfMPQ1bpz6CWt7paDDSBalgc
zv2TUTiS6Kp55/WtjIlb1b1xo8NoM9CDWTJMSdF3d75d88eOb7pYUzC9VummVZM/
BTpWu19hCcj8j598wBoVfIi9RpA9v99PvYxyiYBzX2pNIL55pHhugwRTh8eoKtF2
k4T8+LWz8X/pW6Ig5JSlDQTJ6hZs8rMdvoTV/q/re3JTQNMh0aar/WLPfPlOVoM4
OA==
-----END CERTIFICATE-----
`

		clientJwtStr := `eyJhbGciOiJSUzI1NiIsImtpZCI6IjAwNzY4ZTkxOTg2MTI3NjJiNzQzZTA2MmVjZDU1MDI1NTQzMTZlYmEiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOlsib3BlbnppdGkiXSwiZXhwIjoxNzM4OTQ3NzYxLCJpYXQiOjE3Mzg5NDU5NjEsImlzcyI6Imh0dHBzOi8vMmJiZGMwZjEtMzAyYi00OWMwLWFkNjUtNmRmYmFkYWQzZjY3LXAuc3RhZ2luZy5uZXRmb3VuZHJ5LmlvOjQ0My9vaWRjIiwianRpIjoiNWViZjM3MWEtM2VhMC00MzY4LTliZTQtNzFhN2E2MDUwNDhiIiwibmJmIjoxNzM4OTQ1OTYxLCJzY29wZXMiOlsib3BlbmlkIiwib2ZmbGluZV9hY2Nlc3MiXSwic3ViIjoiOHhVT09TT3QxUCIsInpfYWlkIjoib3BlbnppdGkiLCJ6X2FzaWQiOiJib29wIiwiel9jZnMiOm51bGwsInpfZWlkIjoiY3VydC50dWRvckBuZXRmb3VuZHJ5LmlvIiwiel9lbnYiOm51bGwsInpfaWNlIjpmYWxzZSwiel9yYSI6IjczLjM0LjE5NC4yMTI6NTMyMjEiLCJ6X3NkayI6bnVsbCwiel90IjoiYSJ9.1anQC_uVyQcKtqkmo-QS0A8iRHm_Z_x8zBE7kInvtsYYjpr-bofUa_0TEsp1cxYRXHbP-inqrvEFsLwpBe-AzoZmTKKybg0x1GxjK-0xd451wT_PdPuPrLDW_ch6q-hZwD-AXLKGtcx6iMi3tK06EBjIPucz-tL6Q5Ejx5aeRjNPJnL3T1l59Jd22CnblEYys9eFjgaMT0QG_KmT1QJOmCjRte-RaZ8uMOPJYhBVJz02F2E3dvL0bq2s9Mv-LjoucY2emi9I1M4wrEjB0fD1A1SwXXBFoS9N_riyqYlTB0VbY-ax4kSURKHk5n4qboFV4GrY-Nip4GkxFQ6C-Djt5A`

		block, _ := pem.Decode([]byte(clientCertPem))

		req.NotNil(block)

		clientCert, err := x509.ParseCertificate(block.Bytes)
		req.NoError(err)
		req.NotNil(clientCert)

		jwtParser := jwt.NewParser()
		accessClaims := &common.AccessClaims{}

		token, _, err := jwtParser.ParseUnverified(clientJwtStr, accessClaims)
		req.NoError(err)

		apiSession := &state.ApiSession{
			ApiSession: &edge_ctrl_pb.ApiSession{
				Token:            clientJwtStr,
				CertFingerprints: nil,
				Id:               accessClaims.ApiSessionId,
				IdentityId:       accessClaims.Subject,
			},
			JwtToken:     token,
			Claims:       accessClaims,
			ControllerId: "",
		}

		result := handler.validateBySpiffeId(apiSession, clientCert)

		req.False(result)
	})

}

func Test_verifySpiffId(t *testing.T) {

	t.Run("a valid format and expected value returns true", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse(fmt.Sprintf("spiffe://example.com/identity/1234/apiSession/%s", expectedApiSessionId))
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.True(result)
	})

	t.Run("extra mid-path slashes returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse(fmt.Sprintf("spiffe://example.com//identity/1234/apiSession/%s", expectedApiSessionId))
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("a trailing slash returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse(fmt.Sprintf("spiffe://example.com/identity/1234/apiSession/%s/", expectedApiSessionId))
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("a valid path, invalid scheme, and expected value returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse(fmt.Sprintf("https://example.com/identity/1234/apiSession/%s", expectedApiSessionId))
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("a valid format and invalid value returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse(fmt.Sprintf("spiffe://example.com/identity/1234/apiSession/%s", "invalidValue"))
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("a identity path and valid value returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse(fmt.Sprintf("spiffe://example.com/identityInvalid/1234/apiSession/%s", expectedApiSessionId))
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("an apiSession path and valid value returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse(fmt.Sprintf("spiffe://example.com/identity/1234/apiSessionInvalid/%s", expectedApiSessionId))
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("a missing path returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse("spiffe://example.com")
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("a root path returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse("spiffe://example.com/")
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("an identity only path returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse("spiffe://example.com/identity")
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("an identity and value only path returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse("spiffe://example.com/identity/1234")
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("an identity, value, apiSession only path returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse("spiffe://example.com/identity/1234/apiSession")
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})

	t.Run("an identity, value, apiSession only path returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse("spiffe://example.com/identity/1234/apiSession")
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})
	t.Run("a too long spiffe id returns false", func(t *testing.T) {
		req := require.New(t)

		const expectedApiSessionId = "4567"
		spiffeId, err := url.Parse(fmt.Sprintf("spiffe://example.com/identity/1234/apiSession/%s/iamtoolong", expectedApiSessionId))
		req.NoError(err)

		result := verifySpiffId(spiffeId, expectedApiSessionId)
		req.False(result)
	})
}
