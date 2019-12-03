/*
	Copyright 2019 Netfoundry, Inc.

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

package logconf

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"strings"
)

type Logging struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func (l *Logging) SetLoggingOptions() error {

	fl := strings.TrimSpace(l.Level)

	if fl == "" {
		fl = "info"
	}

	lvl, err := logrus.ParseLevel(fl)

	if err != nil {
		return fmt.Errorf("invalid logging level [%s] specified", l.Level)
	}

	if logrus.GetLevel() != lvl && (logrus.GetLevel() == logrus.InfoLevel || logrus.GetLevel() < lvl) {
		fmt.Printf("Changing log level from %v to %v\n", logrus.GetLevel(), lvl)
		logrus.SetLevel(lvl)
	}

	fmt, err := parseFormat(l.Format)

	if err != nil {
		return err
	}

	if fmt != nil {
		logrus.SetFormatter(fmt)
	}

	return nil
}

func parseFormat(f string) (logrus.Formatter, error) {
	mf := strings.ToLower(strings.TrimSpace(f))
	switch strings.ToLower(mf) {
	case "":
		return nil, nil
	case "default":
		return nil, nil
	case "text":
		return &logrus.TextFormatter{
			DisableColors: true,
			FullTimestamp: true,
		}, nil
	case "json":
		return &logrus.JSONFormatter{}, nil
	case "console":
		return &logrus.TextFormatter{
			ForceColors: true,
		}, nil
	}

	return nil, fmt.Errorf("invalid log formatter [%s]", f)

}
