package model

import (
	"crypto/x509"
	"encoding/json"
	"github.com/Jeffail/gabs/v2"
	"github.com/openziti/edge/controller/persistence"
	"github.com/openziti/jwks"
	"github.com/openziti/storage/boltz"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

var testJkws = `{
    "keys": [
        {
            "alg": "RS256",
            "kty": "RSA",
            "use": "sig",
            "n": "4SEOORSe1V6Ic-_LSbFJaERGxTwBhHt2zHluYO449sYEi7um4Q-ZodseaUw4R1uLvIG_Eh7mJwGi37-To8woYzCLz3fvdF7G5Pq-tm78A4VLC9_WrvBOgP9PXYaGzPcz60JTJb5Ee94jrWYVwLJUGX_AXnjKUAJXFhAVGlrpeCRMhJx625XIQEchNjdotMxe_kPwM9dgmG_zRe0IH98UbuqYTYUwdkH_INe7IL7jJF3tDm2571yAbH_unqdpTvrrb3CkU0f-AIwb-GlYxR2aQ8jNaGGJSx0EI_G89BHMZAGJpRlPXwjD5qrn2QC06XOG9JDrLyDen2Z2R-TYCfkkjw",
            "e": "AQAB",
            "kid": "nDNaLwW5uTxoHZ5vLiTui",
            "x5t": "MMp-6VIvEYOnYoGjvky-Wxk_h0A",
            "x5c": [
                "MIIDDTCCAfWgAwIBAgIJZgHXXsVCojHCMA0GCSqGSIb3DQEBCwUAMCQxIjAgBgNVBAMTGWRldi1qYTI4b2p6ay51cy5hdXRoMC5jb20wHhcNMjIwMzAzMTgxNjM4WhcNMzUxMTEwMTgxNjM4WjAkMSIwIAYDVQQDExlkZXYtamEyOG9qemsudXMuYXV0aDAuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4SEOORSe1V6Ic+/LSbFJaERGxTwBhHt2zHluYO449sYEi7um4Q+ZodseaUw4R1uLvIG/Eh7mJwGi37+To8woYzCLz3fvdF7G5Pq+tm78A4VLC9/WrvBOgP9PXYaGzPcz60JTJb5Ee94jrWYVwLJUGX/AXnjKUAJXFhAVGlrpeCRMhJx625XIQEchNjdotMxe/kPwM9dgmG/zRe0IH98UbuqYTYUwdkH/INe7IL7jJF3tDm2571yAbH/unqdpTvrrb3CkU0f+AIwb+GlYxR2aQ8jNaGGJSx0EI/G89BHMZAGJpRlPXwjD5qrn2QC06XOG9JDrLyDen2Z2R+TYCfkkjwIDAQABo0IwQDAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBRrO/lcE6fRDss+WAroJw0I3sZQ1zAOBgNVHQ8BAf8EBAMCAoQwDQYJKoZIhvcNAQELBQADggEBAChrBHmfIbEOOKdtOs5zfLgZKjwXMQ3NydYHSCPrZKKNrR2JLYRy3KD3iLUZmqgb3kiWqO+aVABAHNi3H3oGTTIEklRH69NMpbmTs+W9suw/JaIrbj2HCPbTCGMvA5yTo3kABJbVapHCO8cd8FCZVCa5+CbdMBsjvnZvUNriX69VAHIzRIv/AGQbHXWQoU7igRm9nLnO/BKFhWPXiSuYpaBz5uqg+qB3gfyGUQjeAoYo5b3YtZt/GrwcQS5Ku4lhV7jPkAgEyQdHAC6RKw7Gf/p+u58gSMKXeZxW9FxgGNMsQPuKTyIEuikYinT70Y1IUsMAaqS5SrzglvPYgZpYTmc="
            ]
        },
        {
            "alg": "RS256",
            "kty": "RSA",
            "use": "sig",
            "n": "-VLOzBGDO1mRgwz6ZWK4aTyebQI5blRifFrhjax-bH_hbFaNZ1LjFZNUJ7wR1GfrXUtI_2bZF-QBeGPD_rfwrPuAVktyysGWpyTeTUJSbdotWyhDN7v6_ySvQcLjQVajRslGiUUn9eBNDvQm8HyAgmUEOFZk5m0kdSh2sU3fB-Q71OGYHm_uTSENGgtnVp7pvXJVoD26-ZKf_6movrrQ8lPX_SBFL79JIGwcV-Q35PkwKpLDmfR5qsiruQcgAOrcU83UEujrHumgJFM2SV_7pP1lW83itYBizeShUXDkMnEsarenNwBs2ej4CHF4wlg8kvAuvM1etP9wTvQgR8pCTw",
            "e": "AQAB",
            "kid": "9OcLRMTskCwYepHJAgyc4",
            "x5t": "AQFCuQ1CEs-mkKBan4LOQS0AsbM",
            "x5c": [
                "MIIDDTCCAfWgAwIBAgIJUE+YLL7UA3KIMA0GCSqGSIb3DQEBCwUAMCQxIjAgBgNVBAMTGWRldi1qYTI4b2p6ay51cy5hdXRoMC5jb20wHhcNMjIwMzAzMTgxNjM4WhcNMzUxMTEwMTgxNjM4WjAkMSIwIAYDVQQDExlkZXYtamEyOG9qemsudXMuYXV0aDAuY29tMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA+VLOzBGDO1mRgwz6ZWK4aTyebQI5blRifFrhjax+bH/hbFaNZ1LjFZNUJ7wR1GfrXUtI/2bZF+QBeGPD/rfwrPuAVktyysGWpyTeTUJSbdotWyhDN7v6/ySvQcLjQVajRslGiUUn9eBNDvQm8HyAgmUEOFZk5m0kdSh2sU3fB+Q71OGYHm/uTSENGgtnVp7pvXJVoD26+ZKf/6movrrQ8lPX/SBFL79JIGwcV+Q35PkwKpLDmfR5qsiruQcgAOrcU83UEujrHumgJFM2SV/7pP1lW83itYBizeShUXDkMnEsarenNwBs2ej4CHF4wlg8kvAuvM1etP9wTvQgR8pCTwIDAQABo0IwQDAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBSwhiE0zXOLZkCeCDIibq6gx1x9VzAOBgNVHQ8BAf8EBAMCAoQwDQYJKoZIhvcNAQELBQADggEBAMyRiIbglAop3eSX/DzS27OJe2vVMd9pCUBV/WeCSIt41Cv1ZHW15sklCyr5mGN27MrYK50h/vb4JcHRTLrUZ2L0Ib5ogcxeQTWzTpcK8VEKT4bUZhJeOoqWxjBEZi/mo8EqadY0NMzEy0mAUTJzOtfJv8eSoRE1ElwTb6AQiTFLHtcK2MLEDWNIXWVOVew5OTVRJLJd4r5jgL9DcuVFY/sWLn7LgV71P9bjZnvGx8FuWouYsnjMT/YhfUhs+n+JPCX7SEHn3rn5XXGN6KyEYzBLrouQHRu+y3x7aYCWwW1Hr94EbvGaD/dSzH+zAMmk635mrmM1JXXYGeIVp0xKP5s="
            ]
        }
    ]
}`

type staticJwksResponder struct {
	payload       string
	called        bool
	calledWithUrl string
}

func (s *staticJwksResponder) Get(url string) (*jwks.Response, []byte, error) {
	s.called = true
	s.calledWithUrl = url

	jwksResponse := &jwks.Response{}
	if err := json.Unmarshal([]byte(s.payload), jwksResponse); err != nil {
		return nil, nil, err
	}

	return jwksResponse, []byte(testJkws), nil
}

func Test_signerRecord_Resolve(t *testing.T) {
	jwksContainer, err := gabs.ParseJSON([]byte(testJkws))
	require.NoError(t, err)
	require.NotNil(t, jwksContainer)

	t.Run("can resolve and parse a valid JWKS response", func(t *testing.T) {
		req := require.New(t)

		jwksEndpoint := "https://example.com/.well-known/jwks"

		jwksResolver := &staticJwksResponder{
			payload: testJkws,
		}

		signerRec := &signerRecord{
			kidToCertificate: map[string]*x509.Certificate{},
			externalJwtSigner: &persistence.ExternalJwtSigner{
				BaseExtEntity: boltz.BaseExtEntity{
					Id:        "fake-id",
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				Name:         "test1",
				JwksEndpoint: &jwksEndpoint,
				Enabled:      true,
			},
			jwksResolver: jwksResolver,
		}

		req.NoError(signerRec.Resolve(false))
		req.True(jwksResolver.called)
		req.Equal(jwksEndpoint, jwksResolver.calledWithUrl)
		req.Len(signerRec.kidToCertificate, 2)

	})
}
