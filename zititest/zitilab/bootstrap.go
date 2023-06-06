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

package zitilab

import (
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/openziti/fablab/kernel/model"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type bootstrapWithFallbacks struct {
	fns []model.BootstrapExtension
}

func BootstrapWithFallbacks(boostrapFns ...model.BootstrapExtension) *bootstrapWithFallbacks {
	return &bootstrapWithFallbacks{
		fns: boostrapFns,
	}
}

func (bootstrap *bootstrapWithFallbacks) Bootstrap(m *model.Model) (retErr error) {
	var err error
	defer func() {
		if retErr == nil && err != nil {
			logrus.WithError(err).Info("Bootstrap succeeded, but had to use a fallback")
		}
	}()
	for _, f := range bootstrap.fns {
		e := f.Bootstrap(m)
		if e == nil {
			return nil
		}
		if err == nil {
			err = e
			continue
		}
		err = errors.Wrap(err, e.Error())
	}
	return err
}

type BootstrapFromEnv struct{}

func (bootstrap *BootstrapFromEnv) Bootstrap(m *model.Model) error {
	logrus.Infof("Bootstraping from Env")
	zitiRoot = os.Getenv("ZITI_ROOT")
	if zitiRoot == "" {
		if zitiPath, err := exec.LookPath("ziti"); err == nil {
			zitiRoot = path.Dir(zitiPath)
		} else {
			return fmt.Errorf("ZITI_PATH not set and ziti executable not found in path. please set 'ZITI_ROOT'")
		}
	}

	if fi, err := os.Stat(zitiRoot); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("invalid 'ZITI_ROOT' (!directory)")
		}
	} else {
		return fmt.Errorf("non-existent 'ZITI_ROOT', given %s", zitiRoot)
	}

	logrus.Debugf("ZITI_ROOT = [%s]", zitiRoot)

	return nil
}

type bootstrapFromDir struct {
	sourcePath string
	destPath   string
}

func BootstrapFromDir(sourcePath, destPath string) *bootstrapFromDir {
	return &bootstrapFromDir{
		sourcePath: sourcePath,
		destPath:   destPath,
	}
}

func (b *bootstrapFromDir) Bootstrap(m *model.Model) error {
	logrus.Infof("Bootstraping from Dir")
	zitiRoot = b.sourcePath
	if _, err := os.Stat(zitiRoot); err != nil {
		return fmt.Errorf("non-existent 'ZITI_ROOT', given %s", zitiRoot)
	}
	logrus.Debugf("ZITI_ROOT = [%s]", zitiRoot)

	return nil
}

type BootstrapFromFind struct{}

func (bootstrap *BootstrapFromFind) Bootstrap(m *model.Model) error {
	logrus.Infof("Bootstraping from Find")
	if zitiPath, err := exec.LookPath("ziti"); err == nil {
		zitiRoot = path.Dir(path.Dir(zitiPath))
	} else {
		return fmt.Errorf("ZITI_PATH not set and ziti executable not found in path. please set 'ZITI_ROOT'")
	}

	if fi, err := os.Stat(zitiRoot); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("invalid 'ZITI_ROOT' (!directory)")
		}
	} else {
		return fmt.Errorf("non-existent 'ZITI_ROOT', given %s", zitiRoot)
	}

	logrus.Debugf("ZITI_ROOT = [%s]", zitiRoot)

	return nil
}
