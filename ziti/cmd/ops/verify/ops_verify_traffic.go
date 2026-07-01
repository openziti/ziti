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

package verify

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/michaelquigley/pfxlog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/openziti/edge-api/rest_management_api_client"
	"github.com/openziti/edge-api/rest_management_api_client/identity"
	"github.com/openziti/edge-api/rest_management_api_client/service"
	"github.com/openziti/edge-api/rest_management_api_client/service_policy"
	"github.com/openziti/edge-api/rest_management_api_client/terminator"
	"github.com/openziti/edge-api/rest_model"
	edge_apis "github.com/openziti/sdk-golang/v2/edge-apis"
	"github.com/openziti/sdk-golang/v2/ziti"
	"github.com/openziti/sdk-golang/v2/ziti/enroll"
	"github.com/openziti/ziti/v2/internal"
	"github.com/openziti/ziti/v2/internal/rest/mgmt"
	"github.com/openziti/ziti/v2/ziti/cmd/edge"
	"github.com/openziti/ziti/v2/ziti/cmd/ops/verify/ext-jwt-signer/oidc"
)

type traffic struct {
	loginOpts            edge.LoginOptions
	prefix               string
	mode                 string
	cleanup              bool
	verbose              bool
	allowMultipleServers bool
	extJwtSigner         string
	redirectURL          string

	client       *rest_management_api_client.ZitiEdgeManagement
	svcName      string
	serverIdName string
	clientIdName string
	bindSPName   string
	dialSPName   string
}

func NewVerifyTraffic(out io.Writer, errOut io.Writer) *cobra.Command {
	t := &traffic{}
	cmd := &cobra.Command{
		Use:   "traffic",
		Short: "Verifies traffic",
		Long:  "A tool to verify traffic can flow over the overlay properly. You must be authenticated to use this tool.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logLvl := logrus.InfoLevel
			if t.verbose {
				logLvl = logrus.DebugLevel
			}

			pfxlog.GlobalInit(logLvl, pfxlog.DefaultOptions().Color())
			internal.ConfigureLogFormat(logLvl)

			timePrefix := time.Now().Format("2006-01-02-1504")
			if t.prefix == "" {
				if t.mode != "both" {
					log.Warnf("no prefix and mode [%s] is not 'both'. default prefix of %s will be used", t.mode, timePrefix)
				}
				t.prefix = timePrefix
			}
			if t.mode == "" {
				t.mode = "both"
			}

			if t.extJwtSigner != "" && t.loginOpts.ControllerUrl == "" {
				return errors.New("--controller-url is required when using --ext-jwt-signer")
			}

			t.svcName = t.prefix + ".traffic"

			t.serverIdName = t.prefix + ".server"
			extraSeverIdName := ""
			if t.allowMultipleServers {
				extraSeverIdName = fmt.Sprintf("%d", time.Now().UnixNano())
			}
			t.serverIdName = fmt.Sprintf("%s.server%s", t.prefix, extraSeverIdName)
			t.clientIdName = t.prefix + ".client"
			t.bindSPName = t.prefix + ".bind"
			t.dialSPName = t.prefix + ".dial"

			mgmtClient, mgmtClientErr := t.loginOpts.NewManagementClient(true)
			if mgmtClientErr != nil {
				return mgmtClientErr
			}
			t.client = mgmtClient.BaseClient.API.ZitiEdgeManagement

			if t.cleanup {
				log.Info("attempting to cleanup based on parameters. this operation will disconnect the server if it's running.")
				t.cleanupClient()
				t.cleanupServer()
				log.Info("cleanup complete. continuing")
			}

			if t.mode == "both" {
				return t.doBoth()
			} else if t.mode == "server" {
				return t.doServer(context.Background(), true)
			} else if t.mode == "client" {
				_, c := context.WithCancel(context.Background())
				return t.doClient(c)
			} else {
				return fmt.Errorf("unknown mode: %s", t.mode)
			}
		},
	}

	cmd.Flags().StringVarP(&t.prefix, "prefix", "x", "", "[optional] The prefix to apply to generated objects, necessary when not using the 'both' role.")
	cmd.Flags().StringVarP(&t.mode, "mode", "m", "", "[optional, default 'both'] The mode to perform: server, client, both.")
	cmd.Flags().BoolVar(&t.cleanup, "cleanup", false, "Whether to perform cleanup.")
	cmd.Flags().BoolVar(&t.allowMultipleServers, "allow-multiple-servers", false, "Whether to allows the same server multiple times.")
	cmd.Flags().StringVar(&t.loginOpts.ControllerUrl, "controller-url", "", "The url of the controller")
	cmd.Flags().StringVar(&t.extJwtSigner, "ext-jwt-signer", "", "[optional] Authenticate via this ext-jwt-signer (OIDC) instead of a certificate, exercising the certless data-plane path. With --mode both (the default) it is used for BOTH the server (bind) and client (dial). Requires --controller-url.")
	cmd.Flags().StringVar(&t.redirectURL, "ext-jwt-redirect-url", "", "[optional] OIDC redirect URL for --ext-jwt-signer (default http://localhost:20314/auth/callback)")

	edge.AddLoginFlags(cmd, &t.loginOpts)
	t.loginOpts.Out = out
	t.loginOpts.Err = errOut

	return cmd
}

func (t *traffic) startServer(ctx context.Context, serviceName string, zitiCfg *ziti.Config) error {
	c, err := ziti.NewContext(zitiCfg)
	if err != nil {
		return err
	}

	listener, err := c.Listen(serviceName)
	if err != nil {
		return err
	}
	log.Infof("successfully bound service: %s.", serviceName)

	connChan := make(chan net.Conn)
	errChan := make(chan error)
	go func() {
		fmt.Println() // put a line in output for the humans
		log.Info("Server is listening for a connection and will exit when one is received.")
		conn, err := listener.Accept()
		log.Info("Server has accepted a connection and will exit soon.")
		if err != nil {
			errChan <- err
			return
		}
		connChan <- conn
	}()

	select {
	case conn := <-connChan:
		handleConnection(conn)
	case err := <-errChan:
		log.Errorf("Error accepting connection: %v", err)
	case <-ctx.Done():
		log.Info("Server shutting down")
		return ctx.Err()
	}
	_ = listener.Close()
	time.Sleep(1 * time.Second)
	log.Info("Server complete. exiting")
	return nil
}

func handleConnection(conn net.Conn) {
	log.Debug("new connection accepted")

	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)
	rw := bufio.NewReadWriter(reader, writer)

	line, err := rw.ReadString('\n')
	if err != nil {
		log.Errorf("error reading from connection: %v", err)
		return
	}
	if strings.Contains(line, "traffic test") {
		log.Info("traffic test successfully detected")
	}
	log.Debugf("read : %s", strings.TrimSpace(line))
	resp := fmt.Sprintf("you sent me: %s", line)
	_, _ = rw.WriteString(resp)
	_ = rw.Flush()
	log.Debugf("responding with : %s", strings.TrimSpace(resp))
}

func (t *traffic) startClient(client *rest_management_api_client.ZitiEdgeManagement, serviceName string, zitiCfg *ziti.Config) error {
	if err := waitForTerminator(client, serviceName, 10*time.Second); err != nil {
		return err
	}
	c, err := ziti.NewContext(zitiCfg)
	if err != nil {
		return err
	}

	foundSvc, ok := c.GetService(serviceName)
	if !ok {
		return errors.New("error when retrieving all the services for the provided config")
	}
	log.Infof("found service named: %s", *foundSvc.Name)

	svc, err := c.Dial(serviceName) //dial the service using the given name
	if err != nil {
		return fmt.Errorf("error when dialing service name %s. %v", serviceName, err)
	}
	log.Infof("successfully dialed service: %s.", serviceName)

	zitiReader := bufio.NewReader(svc)
	zitiWriter := bufio.NewWriter(svc)

	text := "traffic test\n"
	bytesRead, err := zitiWriter.WriteString(text)
	_ = zitiWriter.Flush()
	if err != nil {
		return err
	} else {
		log.Debugf("wrote %d bytes", bytesRead)
	}
	log.Debugf("sent : %s", text)
	read, err := zitiReader.ReadString('\n')
	if err != nil {
		log.Errorf("error reading from reader: %v", err)
	} else {
		log.Debugf("Received: %s", strings.TrimSpace(read))
	}
	return nil
}

func terminatorExists(client *rest_management_api_client.ZitiEdgeManagement, serviceName string) (bool, error) {
	filter := "service.name=\"" + serviceName + "\""
	params := &terminator.ListTerminatorsParams{
		Filter:  &filter,
		Context: context.Background(),
	}

	resp, err := client.Terminator.ListTerminators(params, nil)
	if err != nil {
		return false, err
	}

	return len(resp.Payload.Data) > 0, nil
}

func waitForTerminator(client *rest_management_api_client.ZitiEdgeManagement, serviceName string, timeout time.Duration) error {
	log.Infof("waiting %s for terminator for service: %s", timeout, serviceName)
	startTime := time.Now()
	for {
		exists, err := terminatorExists(client, serviceName)
		if err != nil {
			return err
		}
		if exists {
			log.Infof("found terminator for service: %s", serviceName)
			return nil
		}
		if time.Since(startTime) >= timeout {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("terminator not found for service: %s", serviceName)
}

func createIdentity(client *rest_management_api_client.ZitiEdgeManagement, name string, roleAttributes rest_model.Attributes) (*identity.CreateIdentityCreated, error) {
	falseVar := false
	usrType := rest_model.IdentityTypeUser
	i := &rest_model.IdentityCreate{
		Enrollment: &rest_model.IdentityCreateEnrollment{
			Ott: true,
		},
		IsAdmin:        &falseVar,
		Name:           &name,
		RoleAttributes: &roleAttributes,
		Type:           &usrType,
	}
	p := identity.NewCreateIdentityParams()
	p.Identity = i

	// Create the identity
	ident, err := client.Identity.CreateIdentity(p, nil)
	if err != nil {
		id := mgmt.IdentityFromFilter(client, mgmt.NameFilter(name))
		if id != nil {
			return nil, fmt.Errorf("identity named %s exists, remove it before trying again or use --cleanup", name)
		} else {
			return nil, fmt.Errorf("failed to create the identity: %v", err)
		}
	}
	return ident, nil
}

func createServicePolicy(client *rest_management_api_client.ZitiEdgeManagement, name string, servType rest_model.DialBind, identityRoles rest_model.Roles, serviceRoles rest_model.Roles) (*rest_model.CreateLocation, error) {
	defaultSemantic := rest_model.SemanticAllOf
	servicePolicy := &rest_model.ServicePolicyCreate{
		IdentityRoles: identityRoles,
		Name:          &name,
		Semantic:      &defaultSemantic,
		ServiceRoles:  serviceRoles,
		Type:          &servType,
	}
	params := &service_policy.CreateServicePolicyParams{
		Policy:  servicePolicy,
		Context: context.Background(),
	}
	params.SetTimeout(5 * time.Second)
	resp, err := client.ServicePolicy.CreateServicePolicy(params, nil)
	if resp == nil || err != nil {
		return nil, fmt.Errorf("failed to create service policy: %s", name)
	}
	return resp.Payload.Data, nil
}

func createService(client *rest_management_api_client.ZitiEdgeManagement, name string, serviceConfigs []string, roles rest_model.Attributes) (*rest_model.CreateLocation, error) {
	encryptOn := true
	serviceCreate := &rest_model.ServiceCreate{
		Configs:            serviceConfigs,
		EncryptionRequired: &encryptOn,
		MaxIdleTimeMillis:  0,
		Name:               &name,
		RoleAttributes:     roles,
		Tags:               nil,
		TerminatorStrategy: "",
	}
	serviceParams := &service.CreateServiceParams{
		Service: serviceCreate,
		Context: context.Background(),
	}
	serviceParams.SetTimeout(5 * time.Second)
	resp, err := client.Service.CreateService(serviceParams, nil)
	if resp == nil || err != nil {
		return nil, fmt.Errorf("failed to create service %s: %v", name, err)
	}
	return resp.Payload.Data, nil
}

func deleteIdentity(client *rest_management_api_client.ZitiEdgeManagement, toDelete *rest_model.IdentityDetail) {
	if toDelete == nil {
		return
	}
	idToDel := *toDelete.ID
	deleteParams := &identity.DeleteIdentityParams{
		ID: idToDel,
	}
	deleteParams.SetTimeout(5 * time.Second)
	_, err := client.Identity.DeleteIdentity(deleteParams, nil)
	if err != nil {
		log.Errorf("Failed to delete identity: %s. %v", idToDel, err)
	}
}

func deleteService(client *rest_management_api_client.ZitiEdgeManagement, toDelete *rest_model.ServiceDetail) {
	if toDelete == nil {
		return
	}
	idToDel := *toDelete.ID
	deleteParams := &service.DeleteServiceParams{
		ID: idToDel,
	}
	deleteParams.SetTimeout(5 * time.Second)
	_, err := client.Service.DeleteService(deleteParams, nil)
	if err != nil {
		log.Errorf("Failed to delete service: %s. %v", idToDel, err)
	}
}

func deleteServicePolicy(client *rest_management_api_client.ZitiEdgeManagement, sp *rest_model.ServicePolicyDetail) {
	if sp == nil {
		return
	}
	id := *sp.ID
	deleteParams := &service_policy.DeleteServicePolicyParams{
		ID: id,
	}
	deleteParams.SetTimeout(5 * time.Second)
	_, err := client.ServicePolicy.DeleteServicePolicy(deleteParams, nil)
	if err != nil {
		log.Errorf("Failed to delete the service policy: %s. %v", id, err)
	}
}

func enrollIdentity(client *rest_management_api_client.ZitiEdgeManagement, id string) (*ziti.Config, error) {
	// Get the identity object
	params := &identity.DetailIdentityParams{
		Context: context.Background(),
		ID:      id,
	}
	params.SetTimeout(5 * time.Second)
	resp, err := client.Identity.DetailIdentity(params, nil)

	if err != nil {
		return nil, err
	}

	// Enroll the identity
	tkn, _, err := enroll.ParseToken(resp.Payload.Data.Enrollment.Ott.JWT)
	if err != nil {
		return nil, err
	}

	flags := enroll.EnrollmentFlags{
		Token:  tkn,
		KeyAlg: "EC",
	}
	conf, err := enroll.Enroll(flags)

	if err != nil {
		return nil, err
	}

	return conf, nil
}

func (t *traffic) bindAttr() string {
	return t.svcName + ".binders"
}

func (t *traffic) dialAttr() string {
	return t.svcName + ".dialers"
}

func (t *traffic) svcAttr() string {
	return t.svcName
}

func (t *traffic) configureService() error {
	svc := mgmt.ServiceFromFilter(t.client, mgmt.NameFilter(t.svcName))
	if svc != nil && t.allowMultipleServers {
		log.Debugf("service already exists. not creating: %s", t.svcName)
	} else if _, err := createService(t.client, t.svcName, nil, []string{t.svcAttr()}); err != nil {
		return err
	}

	// As with the dialer, a cert-based binder matches the bind policy by attribute; an
	// ext-jwt binder is a pre-existing OIDC identity granted bind later against its id.
	if t.extJwtSigner == "" {
		bind := mgmt.ServicePolicyFromFilter(t.client, mgmt.NameFilter(t.bindSPName))
		if bind != nil && t.allowMultipleServers {
			log.Debugf("service policy already exists. not creating: %s", t.bindSPName)
		} else if _, err := createServicePolicy(t.client, t.bindSPName, rest_model.DialBindBind, rest_model.Roles{"#" + t.bindAttr()}, rest_model.Roles{"#" + t.svcAttr()}); err != nil {
			return err
		}
	}

	// The cert-based dialer matches the dial policy by attribute. When authenticating via
	// ext-jwt instead, the dialer is a pre-existing OIDC identity granted dial against its id.
	if t.extJwtSigner == "" {
		dial := mgmt.ServicePolicyFromFilter(t.client, mgmt.NameFilter(t.dialSPName))
		if dial != nil && t.allowMultipleServers {
			log.Debugf("service policy already exists. not creating: %s", t.dialSPName)
		} else if _, err := createServicePolicy(t.client, t.dialSPName, rest_model.DialBindDial, rest_model.Roles{"#" + t.dialAttr()}, rest_model.Roles{"#" + t.svcAttr()}); err != nil {
			return err
		}
	}
	return nil
}

func (t *traffic) configureServer() (*ziti.Config, error) {
	serverIdent, err := createIdentity(t.client, t.serverIdName, []string{t.bindAttr()})
	if err != nil {
		return nil, err
	}
	return enrollIdentity(t.client, serverIdent.Payload.Data.ID)
}

func (t *traffic) configureClient() (*ziti.Config, error) {
	clientIdent, err := createIdentity(t.client, t.clientIdName, []string{t.dialAttr()})
	if err != nil {
		return nil, err
	}
	return enrollIdentity(t.client, clientIdent.Payload.Data.ID)
}

func (t *traffic) cleanupServer() {
	if t.allowMultipleServers {
		if exists, _ := terminatorExists(t.client, t.svcName); exists {
			log.Debugf("found terminator for service: %s. cleanup will be skipped.", t.svcName)
			return
		}
	}
	dial := mgmt.ServicePolicyFromFilter(t.client, mgmt.NameFilter(t.dialSPName))
	bind := mgmt.ServicePolicyFromFilter(t.client, mgmt.NameFilter(t.bindSPName))
	deleteServicePolicy(t.client, dial)
	deleteServicePolicy(t.client, bind)
	svc := mgmt.ServiceFromFilter(t.client, mgmt.NameFilter(t.svcName))
	deleteService(t.client, svc)

	id := mgmt.IdentityFromFilter(t.client, mgmt.NameFilter(t.serverIdName))
	deleteIdentity(t.client, id)
}

func (t *traffic) cleanupClient() {
	id := mgmt.IdentityFromFilter(t.client, mgmt.NameFilter(t.clientIdName))
	deleteIdentity(t.client, id)
}

func (t *traffic) doBoth() error {
	if t.extJwtSigner != "" {
		return t.doBothExtJwt()
	}
	if err := t.configureService(); err != nil {
		return err
	}
	wg := &sync.WaitGroup{}
	wg.Add(2)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer wg.Done()
		if err := t.doServer(ctx, false); err != nil {
			log.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := t.doClient(cancel); err != nil {
			log.Error(err)
		}
	}()
	wg.Wait()
	return nil
}

func (t *traffic) doServer(ctx context.Context, configureServices bool) error {
	if t.extJwtSigner != "" {
		return t.doServerExtJwt(ctx, configureServices)
	}

	if configureServices {
		if err := t.configureService(); err != nil {
			return err
		}
	}
	serverCfg, err := t.configureServer()
	if err != nil {
		return err
	}
	defer t.cleanupServer()
	if err := t.startServer(ctx, t.svcName, serverCfg); err != nil {
		return fmt.Errorf("unexpected error: %v", err)
	}
	return nil
}

func (t *traffic) doClient(cancel context.CancelFunc) error {
	if t.extJwtSigner != "" {
		return t.doClientExtJwt(cancel)
	}

	clientCfg, err := t.configureClient()
	if err != nil {
		return err
	}
	defer t.cleanupClient()
	if err := t.startClient(t.client, t.svcName, clientCfg); err != nil {
		return err
	}

	log.Debug("client received expected response. stopping server if it's running")
	cancel() //end the server
	time.Sleep(1 * time.Second)
	log.Info("client complete")
	return nil
}

// doBothExtJwt runs the server (bind) and client (dial) in one process as the SAME certless
// ext-jwt identity. It performs a single OIDC login and reuses the token for both sides,
// which avoids a second browser flow colliding on the OIDC redirect port and the
// bind-completes-after-the-client-already-gave-up-waiting race. The bind/dial grants use the
// base policy names so cleanupServer (and a later --cleanup) reclaim them.
func (t *traffic) doBothExtJwt() error {
	log.Infof("--ext-jwt-signer %q will be used for BOTH the server (bind) and the client (dial)", t.extJwtSigner)
	if err := t.configureService(); err != nil {
		return err
	}
	defer t.cleanupServer()

	token, identityID, err := t.extJwtSession(t.extJwtSigner)
	if err != nil {
		return err
	}

	if _, err := createServicePolicy(t.client, t.bindSPName, rest_model.DialBindBind,
		rest_model.Roles{"@" + identityID}, rest_model.Roles{"#" + t.svcAttr()}); err != nil {
		return err
	}
	if _, err := createServicePolicy(t.client, t.dialSPName, rest_model.DialBindDial,
		rest_model.Roles{"@" + identityID}, rest_model.Roles{"#" + t.svcAttr()}); err != nil {
		return err
	}

	serverCfg, err := t.extJwtConfig(token)
	if err != nil {
		return err
	}
	clientCfg, err := t.extJwtConfig(token)
	if err != nil {
		return err
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer wg.Done()
		log.Infof("binding %s as certless ext-jwt identity (signer %q)", t.svcName, t.extJwtSigner)
		if err := t.startServer(ctx, t.svcName, serverCfg); err != nil {
			log.Errorf("ext-jwt server error: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		log.Infof("dialing %s as certless ext-jwt identity (signer %q)", t.svcName, t.extJwtSigner)
		if err := t.startClient(t.client, t.svcName, clientCfg); err != nil {
			log.Errorf("ext-jwt client error: %v", err)
		}
		cancel() // end the server
		time.Sleep(1 * time.Second)
		log.Info("client complete")
	}()
	wg.Wait()
	return nil
}

// doClientExtJwt runs the dialing client as a certless ext-jwt (OIDC) identity instead of
// a cert-enrolled one: it authenticates via the signer, grants the matched identity dial
// access to the test service, then dials. This exercises the certless dial path.
func (t *traffic) doClientExtJwt(cancel context.CancelFunc) error {
	defer func() {
		cancel() // end the server
		time.Sleep(1 * time.Second)
		log.Info("client complete")
	}()

	token, identityID, err := t.extJwtSession(t.extJwtSigner)
	if err != nil {
		return err
	}

	if _, err := createServicePolicy(t.client, t.dialSPName, rest_model.DialBindDial,
		rest_model.Roles{"@" + identityID}, rest_model.Roles{"#" + t.svcAttr()}); err != nil {
		return err
	}
	defer func() {
		deleteServicePolicy(t.client, mgmt.ServicePolicyFromFilter(t.client, mgmt.NameFilter(t.dialSPName)))
	}()

	cfg, err := t.extJwtConfig(token)
	if err != nil {
		return err
	}

	log.Infof("dialing %s as certless ext-jwt identity (signer %q)", t.svcName, t.extJwtSigner)
	if err := t.startClient(t.client, t.svcName, cfg); err != nil {
		return err
	}
	log.Debug("client received expected response. stopping server if it's running")
	return nil
}

// doServerExtJwt runs the hosting server as a certless ext-jwt (OIDC) identity: it
// authenticates via the signer, grants the matched identity bind access to the test
// service, then binds and serves. This exercises the certless bind path on the router.
func (t *traffic) doServerExtJwt(ctx context.Context, configureServices bool) error {
	if configureServices {
		if err := t.configureService(); err != nil {
			return err
		}
	}
	defer t.cleanupServer()

	token, identityID, err := t.extJwtSession(t.extJwtSigner)
	if err != nil {
		return err
	}

	if _, err := createServicePolicy(t.client, t.bindSPName, rest_model.DialBindBind,
		rest_model.Roles{"@" + identityID}, rest_model.Roles{"#" + t.svcAttr()}); err != nil {
		return err
	}

	cfg, err := t.extJwtConfig(token)
	if err != nil {
		return err
	}

	log.Infof("binding %s as certless ext-jwt identity (signer %q)", t.svcName, t.extJwtSigner)
	if err := t.startServer(ctx, t.svcName, cfg); err != nil {
		return fmt.Errorf("ext-jwt server failed: %v", err)
	}
	return nil
}

// extJwtSession performs the OIDC flow for the named signer and returns the bearer token
// plus the id of the existing identity the controller will match it to.
func (t *traffic) extJwtSession(signerName string) (string, string, error) {
	oidcOpts := &oidc.OidcVerificationConfig{}
	oidcOpts.LoginOptions = t.loginOpts
	oidcOpts.RedirectURL = t.redirectURL

	octx, ocancel := context.WithTimeout(context.Background(), oidc.Timeout)
	defer ocancel()
	tokens, signer, err := oidcOpts.AuthenticateWithSigner(octx, signerName)
	if err != nil {
		return "", "", fmt.Errorf("OIDC authentication with signer %q failed: %v", signerName, err)
	}
	token := oidc.TokenForSigner(tokens, signer)
	if token == "" {
		return "", "", errors.New("IdP returned no usable token for the signer's target token type")
	}
	identityID, err := t.findExtJwtIdentity(signerName, token)
	if err != nil {
		return "", "", err
	}
	return token, identityID, nil
}

// extJwtConfig builds a certless SDK config that authenticates with the given bearer token.
func (t *traffic) extJwtConfig(token string) (*ziti.Config, error) {
	ctrlUrl := t.loginOpts.ControllerUrl
	if !strings.HasPrefix(ctrlUrl, "http") {
		ctrlUrl = "https://" + ctrlUrl
	}
	ctrlUrl = strings.TrimRight(ctrlUrl, "/")

	caPool, err := ziti.GetControllerWellKnownCaPool(ctrlUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch controller CA pool: %v", err)
	}
	creds := edge_apis.NewJwtCredentials(token)
	creds.CaPool = caPool
	return &ziti.Config{ZtAPI: ctrlUrl + "/edge/client/v1", Credentials: creds}, nil
}

// findExtJwtIdentity decodes the token and matches it to an existing identity the same way
// the controller will. This mirrors the controller's ext-jwt identity matching
// (controller/model AuthModuleExtJwt: the signer's claimsProperty selects the claim,
// useExternalId picks externalId vs internal id), and must stay in sync with it.
// ext-jwt auth does not auto-provision on a plain authenticate, so the identity must exist.
func (t *traffic) findExtJwtIdentity(signerName, token string) (string, error) {
	signer := mgmt.ExternalJWTSignerFromFilter(t.client, mgmt.NameFilter(signerName))
	if signer == nil {
		return "", fmt.Errorf("ext-jwt-signer %q not found via management api", signerName)
	}

	claimName := "sub"
	if signer.ClaimsProperty != nil && *signer.ClaimsProperty != "" {
		claimName = *signer.ClaimsProperty
	}
	claims := decodeJwtClaims(token)
	claimVal, _ := claims[claimName].(string)
	if claimVal == "" {
		return "", fmt.Errorf("token has no %q claim to match an identity", claimName)
	}
	// claimVal comes from the IdP token and is interpolated into a controller filter
	// below; reject filter-breaking characters rather than risk an injected query.
	// Legitimate sub/externalId values (UUIDs, emails, usernames) never contain these.
	if strings.ContainsAny(claimVal, "\"\\") {
		return "", fmt.Errorf("claim %q value contains illegal characters", claimName)
	}

	useExternal := signer.UseExternalID == nil || *signer.UseExternalID
	var id *rest_model.IdentityDetail
	if useExternal {
		id = mgmt.IdentityFromFilter(t.client, fmt.Sprintf("externalId=\"%s\"", claimVal))
	} else {
		id = mgmt.IdentityFromFilter(t.client, fmt.Sprintf("id=\"%s\"", claimVal))
	}
	if id == nil {
		return "", fmt.Errorf("no identity matches claim %s=%q; the ext-jwt identity must already exist", claimName, claimVal)
	}
	log.Infof("matched identity %s (%s) for service %s", *id.Name, *id.ID, t.svcName)
	return *id.ID, nil
}

// decodeJwtClaims returns the JWT's claims without verifying its signature. This is only
// used to route to the matching identity; the controller re-validates the token's
// signature when the SDK authenticates on dial/bind, so a forged claim here at worst
// resolves the wrong identity, it cannot bypass authentication.
func decodeJwtClaims(token string) jwt.MapClaims {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return nil
	}
	return claims
}
