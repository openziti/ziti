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
	"io/fs"

	"github.com/openziti/fablab/kernel/model"
)

const (
	DefaultConfigId = "default"
	ConfigIdKey     = "configId"
)

type Config interface {
	GetSource(c *model.Component) (fs.FS, string)
	GetName(c *model.Component) string
}

type DefaultConfig struct {
	sourceFs fs.FS
	source   string
}

func (self *DefaultConfig) GetSource(c *model.Component) (fs.FS, string) {
	return self.sourceFs, self.source
}

func (self *DefaultConfig) GetName(c *model.Component) string {
	return c.Id + ".yml"
}

func NewConfig(source string) *DefaultConfig {
	return &DefaultConfig{source: source}
}

func NewConfigWithFS(sourceFs fs.FS, source string) *DefaultConfig {
	return &DefaultConfig{sourceFs: sourceFs, source: source}
}

type NamedConfig struct {
	sourceFs fs.FS
	source   string
	suffix   string
}

func NewNamedConfig(source, suffix string) *NamedConfig {
	return &NamedConfig{source: source, suffix: suffix}
}

func NewNamedConfigWithFS(sourceFs fs.FS, source, suffix string) *NamedConfig {
	return &NamedConfig{sourceFs: sourceFs, source: source, suffix: suffix}
}

func (self *NamedConfig) GetSource(c *model.Component) (fs.FS, string) {
	return self.sourceFs, self.source
}

func (self *NamedConfig) GetName(c *model.Component) string {
	return c.Id + self.suffix
}
