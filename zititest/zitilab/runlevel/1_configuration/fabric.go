/*
	Copyright 2019 NetFoundry Inc.

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

package zitilib_runlevel_1_configuration

import (
	"fmt"
	"github.com/openziti/fablab/kernel/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
	"path"
	"strings"
)

func Fabric(trustDomain string, componentSpecs ...string) model.ConfigurationStage {
	if len(componentSpecs) == 0 {
		componentSpecs = []string{"*"}
	}
	return &fabric{
		trustDomain:    trustDomain,
		componentSpecs: componentSpecs,
	}
}

type fabric struct {
	trustDomain    string
	componentSpecs []string
}

func (f *fabric) getSpiffeId(c *model.Component, id string) string {
	for _, tag := range c.Tags {
		if strings.HasPrefix(tag, "spiffe:") {
			prefix := tag[len("spiffe:"):]
			return prefix + "/" + id
		}
	}
	return id
}

func (f *fabric) Configure(run model.Run) error {
	m := run.GetModel()
	if err := generateCa(f.trustDomain); err != nil {
		return fmt.Errorf("error generating ca (%s)", err)
	}

	ips := map[string]string{}

	processedComponents := map[*model.Component]struct{}{}

	for _, spec := range f.componentSpecs {
		for _, component := range m.SelectComponents(spec) {
			if _, found := processedComponents[component]; found {
				continue
			}
			processedComponents[component] = struct{}{}

			if component.PublicIdentity != "" {
				logrus.Infof("generating public ip identity [%s/%s] on [%s/%s]", component.Id, component.PublicIdentity, component.Region().Id, component.Host.Id)
				if err := generateSigningCert(component.PublicIdentity); err != nil {
					return errors.Wrapf(err, "error generating public identity [%s/%s]", component.Id, component.PublicIdentity)
				}

				if err := generateCert(component.PublicIdentity, component.Host.PublicIp, f.getSpiffeId(component, component.PublicIdentity)); err != nil {
					return fmt.Errorf("error generating public identity [%s/%s]", component.Id, component.PublicIdentity)
				}

				ips[component.GetPathId()+".public"] = component.Host.PublicIp
			}

			if component.PrivateIdentity != "" {
				logrus.Infof("generating private ip identity [%s/%s] on [%s/%s]", component.Id, component.PrivateIdentity, component.Region().Id, component.Host.Id)
				if err := generateSigningCert(component.PrivateIdentity); err != nil {
					return errors.Wrapf(err, "error generating private identity [%s/%s]", component.Id, component.PublicIdentity)
				}

				if err := generateCert(component.PrivateIdentity, component.Host.PrivateIp, f.getSpiffeId(component, component.PrivateIdentity)); err != nil {
					return fmt.Errorf("error generating private identity [%s/%s]", component.Id, component.PrivateIdentity)
				}
				ips[component.GetPathId()+".private"] = component.Host.PrivateIp
			}
		}
	}

	return storeIps(ips)
}

func haveIpsChanged(m *model.Model) (bool, error) {
	ipFile := path.Join(model.PkiBuild(), "ips")
	ips := map[string]string{}
	if _, err := os.Stat(ipFile); err == nil {
		ipData, err := os.ReadFile(ipFile)
		if err != nil {
			return false, err
		}
		if err = yaml.Unmarshal(ipData, &ips); err != nil {
			return false, err
		}
	}

	for _, c := range m.SelectComponents("*") {
		pubIdKey := c.GetPathId() + ".public"
		if c.PublicIdentity != "" {
			if ip, found := ips[pubIdKey]; found {
				if ip == c.Host.PublicIp {
					delete(ips, pubIdKey)
				} else {
					logrus.Infof("public ip for %v/%v has changed from %v -> %v. rebuilding pki", c.Id, c.PublicIdentity, c.Host.PublicIp, ip)
					return true, nil
				}
			} else {
				logrus.Infof("missing cert public identity of %v/%v. rebuilding pki", c.Id, c.PublicIdentity)
				return true, nil
			}
		}

		privIdKey := c.GetPathId() + ".private"
		if c.PrivateIdentity != "" {
			if ip, found := ips[privIdKey]; found {
				if ip == c.Host.PrivateIp {
					delete(ips, privIdKey)
				} else {
					logrus.Infof("private ip for %v/%v has changed from %v -> %v. rebuilding pki", c.Id, c.PrivateIdentity, c.Host.PrivateIp, ip)
					return true, nil
				}
			} else {
				logrus.Infof("missing cert for private identity of %v/%v. rebuilding pki", c.Id, c.PrivateIdentity)
			}
		}
	}

	return false, nil
}

func storeIps(ips map[string]string) error {
	ipFile := path.Join(model.PkiBuild(), "ips")
	data, err := yaml.Marshal(ips)
	if err != nil {
		return err
	}
	return os.WriteFile(ipFile, data, 0600)
}

func hasExisitingPki() (bool, error) {
	if _, err := os.Stat(model.PkiBuild()); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return true, err
	}
	return true, nil
}

func hasExistingCA(name string) (bool, error) {
	if _, err := os.Stat(path.Join(model.PkiBuild(), name)); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return true, err
	}
	return true, nil
}
