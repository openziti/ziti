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
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/ziti/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/common"
	"io"
	"net/http"
	"net/url"
	"os"
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
	"github.com/openziti/ziti/ziti/util"
)

const (
	DefaultAuthScopes = "openid"
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
		urlParamOpts := func() []oauth2.AuthCodeOption {
			var r []oauth2.AuthCodeOption
			for _, v := range config.AdditionalLoginParams {
				parts := strings.Split(v, "=")
				r = append(r, oauth2.SetAuthURLParam(parts[0], parts[1]))
			}
			return r
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

func (c *OIDCConfig) AuthUrl(party rp.RelyingParty) string {
	var r []oauth2.AuthCodeOption
	for _, v := range c.AdditionalLoginParams {
		parts := strings.Split(v, "=")
		r = append(r, oauth2.SetAuthURLParam(parts[0], parts[1]))
	}

	return party.OAuthConfig().AuthCodeURL("state", r...)
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
	showRefreshToken bool
	showAccessToken  bool
	attemptAuth      bool
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

			config, _, cfgErr := util.LoadRestClientConfig()
			if cfgErr == nil {
				// any error indicates there are probably no saved credentials.
				id := config.GetIdentity()
				if defaultId := config.EdgeIdentities[id]; defaultId != nil && !opts.IgnoreConfig {
					opts.Token = defaultId.Token
				}
			}

			m, merr := opts.NewClientApiClient()
			if merr != nil {
				log.WithError(merr).Fatal("error creating mgmt")
			}

			s := client.ExternalJWTSignerFromFilter(m, `name="`+args[0]+`"`)
			if s == nil {
				log.Fatal("no external JWT signer found with name")
				return
			}

			if opts.redirectURL == "" {
				opts.RedirectURL = "http://localhost:20314/auth/callback"
				log.Infof("using default redirect url: %s", opts.RedirectURL)
			} else {
				log.Infof("using supplied redirect url: %s", opts.RedirectURL)
			}

			if s.Audience != nil {
				opts.AdditionalLoginParams = append(opts.AdditionalLoginParams, fmt.Sprintf("audience=%s", *s.Audience))
			}

			if s.Scopes != nil {
				opts.additionalScopes = append(opts.additionalScopes, s.Scopes...)
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
				return
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

			log.Info("attempting to authenticate")
			log.Debugf("auth url: %s", opts.OIDCConfig.AuthUrl(relyingParty))
			time.Sleep(100 * time.Millisecond) //allow the logger to log before starting the wait spinner

			tokens, oidcErr := GetTokens(ctx, opts.OIDCConfig, relyingParty)
			if oidcErr != nil {
				log.Fatalf("error performing OIDC flow: %v", oidcErr)
			}

			log.Tracef("authentication succeeded")

			if idToken, tErr := jwtPayload(tokens.IDToken); tErr != nil {
				log.Warnf("ID token not parsed: %v", tErr)
			} else {
				log.Infof("ID token payload:\n%s", idToken)
			}
			if opts.showIDToken {
				log.Infof("Raw ID token: %s", tokens.IDToken)
			}

			if accessToken, tErr := jwtPayload(tokens.AccessToken); tErr != nil {
				log.Warnf("access token not parsed: %v", tErr)
			} else {
				log.Infof("access token payload:\n%s", accessToken)
			}
			if opts.showAccessToken {
				log.Infof("Raw access token: %s", tokens.AccessToken)
			}

			if refreshToken, tErr := jwtPayload(tokens.RefreshToken); tErr != nil {
				log.Warnf("refresh token not parsed: %v", tErr)
			} else {
				log.Infof("refresh token payload:\n%s", refreshToken)
			}
			if opts.showRefreshToken {
				log.Infof("Raw refresh token: %s", tokens.RefreshToken)
			}

			if opts.attemptAuth {
				tmpFile, _ := os.CreateTemp("", "ext-auth-*.txt")
				defer func() {
					_ = tmpFile.Close()
					_ = os.Remove(tmpFile.Name())
					log.Debugf("removed temp file: " + tmpFile.Name())
				}()

				var token string
				if s.TargetToken == nil || *s.TargetToken == rest_model.TargetTokenACCESS {
					token = tokens.AccessToken
				} else if *s.TargetToken == rest_model.TargetTokenID {
					token = tokens.IDToken
				} else {
					log.Fatalf("invalid target token: %s", s.TargetToken)
				}
				newAuth := edge.LoginOptions{
					Options: api.Options{
						CommonOptions: common.CommonOptions{
							Out: opts.Out,
							Err: opts.Err,
						},
					},
					ControllerUrl: opts.ControllerUrl,
					ExtJwtToken:   token,
					IgnoreConfig:  true,
				}
				log.Infof("attempting to authenticate with specified target token type: %s", *s.TargetToken)
				err := newAuth.Run()
				if err != nil {
					log.Fatalf("error authenticating with token: %v", err)
				} else {
					log.Info("login succeeded")
				}
			}
		},
	}

	edge.AddLoginFlags(cmd, &opts.LoginOptions)
	opts.Out = out
	opts.Err = errOut

	cmd.Flags().BoolVar(&opts.showIDToken, "id-token", false, "Display the full ID Token to the screen. Use caution.")
	cmd.Flags().BoolVar(&opts.showRefreshToken, "refresh-token", false, "Display the full Refresh Token to the screen. Use caution.")
	cmd.Flags().BoolVar(&opts.showAccessToken, "access-token", false, "Display the full Access Token to the screen. Use caution.")
	cmd.Flags().StringVar(&opts.ControllerUrl, "controller-url", "", "The url of the controller")
	cmd.Flags().StringSliceVarP(&opts.additionalScopes, "additional-scopes", "s", []string{}, "List of additional scopes to add")
	cmd.Flags().BoolVar(&opts.attemptAuth, "authenticate", false, "Attempt to authenticate using the supplied ext-jwt-signer")

	ziticobra.SetHelpTemplate(cmd)
	return cmd
}

/* jwtPayload extracts the payload from a jwt so the contents can be logged and shown to the user
 */
func jwtPayload(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(token) < 2 {
		return "", errors.New("token doesn't appear to be a jwt/is opaque. cannot display payload")
	} else {
		// don't bother trying to parse as a JWT, just pull off the part[1] and base64 decode
		payload := addPadding(parts[1])
		decoded, decodeErr := base64.StdEncoding.DecodeString(payload)
		if decodeErr != nil {
			return "", errors.New("token could not be decoded")
		}

		var prettyJSON bytes.Buffer
		err := json.Indent(&prettyJSON, []byte(decoded), "", "  ")
		if err != nil {
			return "", fmt.Errorf("token %s could not be decoded", string(decoded))
		} else {
			return prettyJSON.String(), nil
		}
	}
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
