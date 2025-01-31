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

package oidc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/gorilla/securecookie"
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/zitadel/oidc/v2/pkg/client/rp"
	"github.com/zitadel/oidc/v2/pkg/client/rp/cli"
	httphelper "github.com/zitadel/oidc/v2/pkg/http"
	"github.com/zitadel/oidc/v2/pkg/oidc"
	"golang.org/x/oauth2"

	"github.com/openziti/ziti/internal"
	ziticobra "github.com/openziti/ziti/internal/cobra"
	"github.com/openziti/ziti/internal/rest/client"
	"github.com/openziti/ziti/ziti/cmd/edge"
)

const (
	DefaultAuthScopes = "openid profile"
	Timeout           = 30 * time.Second
)

func pkceFlow[C oidc.IDClaims](ctx context.Context, relyingParty rp.RelyingParty, config OIDCConfig) *oidc.Tokens[C] {
	log := pfxlog.Logger()
	codeflowCtx, codeflowCancel := context.WithCancel(ctx)
	defer codeflowCancel()

	tokenChan := make(chan *oidc.Tokens[C], 1)

	callback := func(w http.ResponseWriter, r *http.Request, tokens *oidc.Tokens[C], state string, rp rp.RelyingParty) {
		tokenChan <- tokens
		msg := `<!DOCTYPE html>
<html lang="en">
<head>
    <title>OpenZiti: Successful Authentication with External Provider.</title>
    <script>
        function closeWindow() {
            setTimeout(function() {
                window.close(); // Close the current window
            }, 3000);
        }
    </script>
</head>
<script type="text/javascript">closeWindow()</script>
<body onload="closeWindow()">
    <img height="40px" src="https://openziti.io/img/ziti-logo-dark.svg"/>
    <h2>Successfully authenticated with external provider.</h2><p>You may close this page. It will attempt to close itself in 3 seconds.</p>
</body>
</html>`
		_, _ = w.Write([]byte(msg))
	}

	authHandlerWithQueryState := func(party rp.RelyingParty) http.HandlerFunc {
		var urlParamOpts rp.URLParamOpt
		for _, v := range config.AdditionalLoginParams {
			parts := strings.Split(v, "=")
			urlParamOpts = rp.WithURLParam(parts[0], parts[1])
		}
		if urlParamOpts == nil {
			urlParamOpts = func() []oauth2.AuthCodeOption {
				return []oauth2.AuthCodeOption{}
			}
		}
		return func(w http.ResponseWriter, r *http.Request) {
			rp.AuthURLHandler(func() string {
				return uuid.New().String()
			}, party, urlParamOpts)(w, r)
		}
	}

	http.Handle("/login", authHandlerWithQueryState(relyingParty))
	u, urlErr := url.Parse(config.RedirectURL)
	if urlErr != nil {
		log.Errorf("Error parsing redirect URL: %v", urlErr)
		return nil
	}
	http.Handle(u.Path, rp.CodeExchangeHandler(callback, relyingParty))

	httphelper.StartServer(codeflowCtx, ":20314")

	cli.OpenBrowser("http://localhost:20314/login")

	return <-tokenChan
}

// OIDCConfig represents a config for the OIDC auth flow.
type OIDCConfig struct {
	// Issuer is the URL of the OpenID Connect provider.
	Issuer string

	// HashKey is used to authenticate values using HMAC.
	HashKey []byte

	// BlockKey is used to encrypt values using AES.
	BlockKey []byte

	// IDToken is the ID token returned by the OIDC provider.
	IDToken string

	// Logger function for debug.
	Logf func(format string, args ...interface{})

	// Additional params to add to the login request
	AdditionalLoginParams []string

	oauth2.Config
}

type OIDCResponse struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
}

// GetToken starts a local HTTP server, opens the web browser to initiate the OIDC Discovery and
// Token Exchange flow, blocks until the user completes authentication and is redirected back, and returns
// the OIDC tokens.
func GetTokens(ctx context.Context, config OIDCConfig, relyingParty rp.RelyingParty) (*OIDCResponse, error) {
	spinnerCtx, spinnerCancel := context.WithTimeout(ctx, 20*time.Second)
	s := spinnerS{
		message:    "Waiting up to " + Timeout.String() + " for external auth...",
		cancelFunc: spinnerCancel,
	}
	go s.Spin(spinnerCtx)
	resultChan := make(chan *oidc.Tokens[*oidc.IDTokenClaims])

	go func() {
		tokens := pkceFlow[*oidc.IDTokenClaims](ctx, relyingParty, config)
		resultChan <- tokens
	}()

	select {
	case tokens := <-resultChan:
		s.Success()
		return &OIDCResponse{
			IDToken:      tokens.IDToken,
			RefreshToken: tokens.RefreshToken,
			AccessToken:  tokens.AccessToken,
		}, nil
	case <-ctx.Done():
		s.Failure()
		return nil, errors.New("timeout: OIDC authentication took too long")
	}
}

func (c *OidcVerificationConfig) NewRelyingParty() (rp.RelyingParty, error) {
	if err := c.ValidateAndSetDefaults(); err != nil {
		return nil, fmt.Errorf("invalid c: %w", err)
	}

	for _, scope := range c.additionalScopes {
		c.OIDCConfig.Scopes = appendIfNotExists(c.OIDCConfig.Scopes, scope)
	}

	cookieHandler := httphelper.NewCookieHandler(c.HashKey, c.BlockKey, httphelper.WithUnsecure())

	options := []rp.Option{
		rp.WithCookieHandler(cookieHandler),
		rp.WithVerifierOpts(rp.WithIssuedAtOffset(5 * time.Second)),
	}
	if c.ClientSecret == "" {
		options = append(options, rp.WithPKCE(cookieHandler))
	}

	relyingParty, err := rp.NewRelyingPartyOIDC(c.Issuer, c.ClientID, c.ClientSecret, c.RedirectURL, c.Scopes, options...)
	if err != nil {
		return nil, fmt.Errorf("error creating relyingParty %s", err.Error())
	}
	return relyingParty, nil
}

// validateAndSetDefaults validates the config and sets default values.
func (c *OIDCConfig) ValidateAndSetDefaults() error {
	if c.ClientID == "" {
		return fmt.Errorf("ClientID must be set")
	}

	c.HashKey = securecookie.GenerateRandomKey(32)
	c.BlockKey = securecookie.GenerateRandomKey(32)

	if c.Logf == nil {
		c.Logf = func(string, ...interface{}) {}
	}

	c.Scopes = strings.Split(DefaultAuthScopes, " ")

	return nil
}

type spinnerS struct {
	message    string
	cancelFunc context.CancelFunc
}

var colorCyan = color.New(color.FgCyan).SprintFunc()

func (s *spinnerS) Spin(ctx context.Context) {
	spinnerChars := []string{"|", "/", "-", "\\"}

	fmt.Print(s.message)
	i := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
			fmt.Printf("\r%s %s", colorCyan(s.message), spinnerChars[i%len(spinnerChars)])
			i++
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (s *spinnerS) Success() {
	fmt.Printf("\r%s", colorCyan(s.message))
	fmt.Printf("%s     \n", colorCyan("Done!"))
	s.cancelFunc()
	time.Sleep(10 * time.Millisecond)
}
func (s *spinnerS) Failure() {
	fmt.Printf("%s     \n", colorCyan("Failed!"))
	s.cancelFunc()
	time.Sleep(10 * time.Millisecond)
}

type OidcVerificationConfig struct {
	OIDCConfig
	edge.LoginOptions

	redirectURL      string
	additionalScopes []string
	showIDToken      bool
	showRefrestToken bool
	showAccessToken  bool
}

func NewOidcVerificationCmd(out io.Writer, errOut io.Writer, initialContext context.Context) *cobra.Command {
	opts := &OidcVerificationConfig{}
	opts.Out = out
	opts.Err = errOut

	log := pfxlog.Logger()
	cmd := &cobra.Command{
		Use:   "oidc",
		Short: "test an external JWT signer for OIDC auth",
		Long:  "tests and verifies an external JWT signer is configured correctly to authenticate using OIDC",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			logLvl := logrus.InfoLevel
			if opts.Verbose {
				logLvl = logrus.DebugLevel
			}
			internal.ConfigureLogFormat(logLvl)

			m, merr := opts.NewClientApiClient()
			if merr != nil {
				log.WithError(merr).Fatal("error creating mgmt")
			}

			s := client.ExternalJWTSignerFromFilter(m, `name="`+args[0]+`"`)
			if s == nil {
				log.Fatal("no external JWT signer found with name")
			}

			if opts.redirectURL == "" {
				opts.RedirectURL = "http://localhost:20314/auth/callback"
				log.Infof("using default redirect url: %s", opts.RedirectURL)
			} else {
				log.Infof("using supplied redirect url: %s", opts.RedirectURL)
			}

			log.Infof("found external JWT signer")
			opts.Issuer = *s.ExternalAuthURL
			opts.ClientID = *s.ClientID
			log.Infof("  - issuer: %s", safeValue(s.ExternalAuthURL))
			log.Infof("  - clientId: %s", safeValue(s.ClientID))

			ctx, cancel := context.WithTimeout(initialContext, Timeout)
			defer cancel()

			relyingParty, rpErr := opts.NewRelyingParty()
			if rpErr != nil {
				log.Fatalf("error creating relying party %s", rpErr.Error())
			}

			if opts.Issuer != "" {
				if opts.Issuer == relyingParty.Issuer() {
					log.Infof("supplied issuer matches discovered issuer: %s", opts.Issuer)
				} else {
					log.Infof("discovered issuer [%s] overridden: %s", relyingParty.Issuer(), opts.Issuer)
				}
			} else {
				log.Infof("issuer discovered as: %s", relyingParty.Issuer())
			}

			log.Infof("attempting to authenticate")

			tokens, oidcErr := GetTokens(ctx, opts.OIDCConfig, relyingParty)
			if oidcErr != nil {
				log.Fatalf("error performing OIDC flow: %v", oidcErr)
			}

			log.Tracef("authentication succeeded")

			if opts.showIDToken {
				log.Infof("Raw ID token: %s", tokens.IDToken)
			}
			idParts := strings.Split(tokens.IDToken, ".")
			if len(idParts) < 2 {
				log.Warnf("ID token returned is not a valid JWT! This should not happen, ID tokens are mandated to be JWT per the spec.")
			} else {
				// don't bother trying to parse as a JWT, just pull off the part[1] and base64 decode
				payload := addPadding(idParts[1])
				decoded, decodeErr := base64.StdEncoding.DecodeString(payload)
				if decodeErr != nil {
					log.Warn("access token could not be decoded and is likely an opaque token. This token will require OpenZiti to perform additional work to verify")
				}

				var prettyJSON bytes.Buffer
				err := json.Indent(&prettyJSON, []byte(decoded), "", "  ")
				if err != nil {
					// just write the contents then...
					log.Warnf("ID token could not be decoded. This highly unexpected: %v", string(decoded))
				} else {
					log.Infof("ID token contents:\n%s", prettyJSON.String())
				}
			}
			if opts.showRefrestToken {
				log.Infof("Raw Refresh token: %s", tokens.RefreshToken)
			}
			if opts.showAccessToken {
				log.Infof("Raw Access token: %s", tokens.AccessToken)
			}
			atParts := strings.Split(tokens.AccessToken, ".")
			if len(atParts) < 2 {
				log.Warnf("Access token is opaque. This token will require OpenZiti to perform additional work to verify")
			} else {
				// don't bother trying to parse as a JWT, just pull off the part[1] and base64 decode
				payload := addPadding(idParts[1])
				decoded, decodeErr := base64.StdEncoding.DecodeString(payload)
				if decodeErr != nil {
					log.Warn("access token could not be decoded and is likely an opaque token. This token will require OpenZiti to perform additional work to verify")
				}

				var prettyJSON bytes.Buffer
				err := json.Indent(&prettyJSON, []byte(decoded), "", "  ")
				if err != nil {
					// just write the contents then...
					log.Infof("access token could not be decoded: %v", string(decoded))
				} else {
					log.Infof("access token contents:\n%s", prettyJSON.String())
				}
			}
		},
	}

	edge.AddLoginFlags(cmd, &opts.LoginOptions)
	opts.Out = out
	opts.Err = errOut

	cmd.Flags().BoolVar(&opts.showIDToken, "id-token", false, "Display the full ID Token to the screen. Use caution.")
	cmd.Flags().BoolVar(&opts.showIDToken, "refresh-token", false, "Display the full Refresh Token to the screen. Use caution.")
	cmd.Flags().BoolVar(&opts.showAccessToken, "access-token", false, "Display the full Access Token to the screen. Use caution.")
	cmd.Flags().StringVar(&opts.ControllerUrl, "controller-url", "", "The url of the controller")
	cmd.Flags().StringSliceVarP(&opts.additionalScopes, "additional-scopes", "s", []string{}, "List of additional scopes to add")

	ziticobra.SetHelpTemplate(cmd)
	return cmd
}

func addPadding(input string) string {
	missing := len(input) % 4
	if missing != 0 {
		input += strings.Repeat("=", 4-missing)
	}
	return input
}

func safeValue[T any](value *T) T {
	var zero T
	if value == nil {
		return zero
	}
	return *value
}

func appendIfNotExists(slice []string, value string) []string {
	for _, v := range slice {
		if v == value {
			return slice // Value already exists, return unchanged slice
		}
	}
	return append(slice, value) // Append if not found
}
