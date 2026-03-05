package edge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/fablab/kernel/lib/parallel"
	"github.com/openziti/fablab/kernel/libssh"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/enroll"
	"github.com/openziti/ziti/v2/zitirest"
	zitilib_actions "github.com/openziti/ziti/zititest/zitilab/actions"
	"github.com/openziti/ziti/zititest/zitilab/cli"
	"github.com/openziti/ziti/zititest/zitilab/models"
	zitiutil "github.com/openziti/foundation/v2/util"
)

func InitIdentities(componentSpec string, concurrency int) model.Action {
	return &initIdentitiesAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
	}
}

func (action *initIdentitiesAction) Execute(run model.Run) error {
	return run.GetModel().ForEachComponent(action.componentSpec, action.concurrency, func(c *model.Component) error {
		// delete may fail if identity doesn't exist yet (first run), that's ok
		_ = zitilib_actions.EdgeExec(run.GetModel(), "delete", "identity", c.Id)

		return action.createAndEnrollIdentity(run, c)
	})
}

func (action *initIdentitiesAction) createAndEnrollIdentity(run model.Run, c *model.Component) error {
	ssh := c.GetHost().NewSshConfigFactory()

	jwtFileName := filepath.Join(run.GetTmpDir(), c.Id+".jwt")

	err := zitilib_actions.EdgeExec(c.GetModel(), "create", "identity", c.Id,
		"--jwt-output-file", jwtFileName,
		"-a", strings.Join(c.Tags, ","))

	if err != nil {
		return err
	}

	configFileName := filepath.Join(run.GetTmpDir(), c.Id+".json")

	_, err = cli.Exec(c.GetModel(), "edge", "enroll", "--jwt", jwtFileName, "--out", configFileName)

	if err != nil {
		return err
	}

	remoteConfigFile := "/home/ubuntu/fablab/cfg/" + c.Id + ".json"
	return libssh.SendFile(ssh, configFileName, remoteConfigFile)
}

type initIdentitiesAction struct {
	componentSpec string
	concurrency   int
}

func InitIdentitiesWithClients(componentSpec string, concurrency int, clientsF clientsInitializer) model.Action {
	return &initIdentitiesWithClientsAction{
		componentSpec: componentSpec,
		concurrency:   concurrency,
		clientsF:      clientsF,
	}
}

func (action *initIdentitiesWithClientsAction) Execute(run model.Run) error {
	clients, err := action.clientsF(run)
	if err != nil {
		return err
	}

	var tasks []parallel.LabeledTask
	for _, c := range run.GetModel().SelectComponents(action.componentSpec) {
		tasks = append(tasks, createIdentityTask(run, c, clients))
	}

	return parallel.ExecuteLabeled(tasks, int64(action.concurrency), models.RetryPolicy)
}

type initIdentitiesWithClientsAction struct {
	componentSpec string
	concurrency   int
	clientsF      clientsInitializer
}

func createIdentityTask(run model.Run, c *model.Component, clients *zitirest.Clients) parallel.LabeledTask {
	configFileName := filepath.Join(run.GetTmpDir(), c.Id+".json")
	remoteConfigFile := "/home/ubuntu/fablab/cfg/" + c.Id + ".json"
	sshFactory := c.GetHost().NewSshConfigFactory()

	task := func() error {
		log := pfxlog.Logger().WithField("identity", c.Id)

		// Delete existing identity if present
		existingId, err := models.GetIdentityId(clients, c.Id, 5*time.Second)
		if err == nil {
			if err = models.DeleteIdentity(clients, existingId, 15*time.Second); err != nil {
				log.WithError(err).Warn("failed to delete existing identity, continuing")
			}
		}

		// Create identity with OTT enrollment
		identityType := rest_model.IdentityTypeDefault
		newId, err := models.CreateIdentity(clients, &rest_model.IdentityCreate{
			Enrollment:     &rest_model.IdentityCreateEnrollment{Ott: true},
			IsAdmin:        zitiutil.Ptr(false),
			Name:           zitiutil.Ptr(c.Id),
			RoleAttributes: zitiutil.Ptr(rest_model.Attributes(c.Tags)),
			Type:           &identityType,
		}, 15*time.Second)
		if err != nil {
			return fmt.Errorf("failed to create identity %s: %w", c.Id, err)
		}

		// Fetch identity detail to get enrollment JWT
		detail, err := models.DetailIdentity(clients, newId, 5*time.Second)
		if err != nil {
			return fmt.Errorf("failed to get identity detail for %s: %w", c.Id, err)
		}

		if detail.Enrollment == nil || detail.Enrollment.Ott == nil || detail.Enrollment.Ott.JWT == "" {
			return fmt.Errorf("identity %s has no OTT enrollment JWT", c.Id)
		}

		// Enroll using the SDK
		jwtStr := detail.Enrollment.Ott.JWT
		claims, jwtToken, err := enroll.ParseToken(jwtStr)
		if err != nil {
			return fmt.Errorf("failed to parse enrollment JWT for %s: %w", c.Id, err)
		}

		var keyAlg ziti.KeyAlgVar
		_ = keyAlg.Set("RSA")

		conf, err := enroll.Enroll(enroll.EnrollmentFlags{
			Token:     claims,
			JwtToken:  jwtToken,
			JwtString: jwtStr,
			KeyAlg:    keyAlg,
		})
		if err != nil {
			return fmt.Errorf("failed to enroll identity %s: %w", c.Id, err)
		}

		// Write config to file
		output, err := os.Create(configFileName)
		if err != nil {
			return fmt.Errorf("failed to create config file for %s: %w", c.Id, err)
		}

		enc := json.NewEncoder(output)
		enc.SetEscapeHTML(false)
		encErr := enc.Encode(conf)
		_ = output.Close()

		if encErr != nil {
			return fmt.Errorf("failed to write config for %s: %w", c.Id, encErr)
		}

		// Send config to remote host
		if err = libssh.SendFile(sshFactory, configFileName, remoteConfigFile); err != nil {
			return fmt.Errorf("failed to send config to host for %s: %w", c.Id, err)
		}

		log.Info("identity initialized successfully")
		return nil
	}

	return parallel.TaskWithLabel("create.identity", fmt.Sprintf("init identity %s", c.Id), task)
}
