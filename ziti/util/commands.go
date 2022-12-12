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

package util

import (
	"fmt"
	"github.com/openziti/ziti/ziti/internal/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"os"
	"os/exec"
	"strings"
)

const (
	CONFIGFILENAME = "config"
)

func PathWithBinary() string {
	path := os.Getenv("PATH")
	binDir, _ := BinaryLocation()
	return binDir + string(os.PathListSeparator) + path
}

// GetCommandOutput evaluates the given command and returns the trimmed output
func GetCommandOutput(dir string, name string, args ...string) (string, error) {
	os.Setenv("PATH", PathWithBinary())
	e := exec.Command(name, args...)
	if dir != "" {
		e.Dir = dir
	}
	data, err := e.CombinedOutput()
	text := string(data)
	text = strings.TrimSpace(text)
	if err != nil {
		return text, fmt.Errorf("error: command failed  %s %s %s %s", name, strings.Join(args, " "), text, err)
	}
	return text, err
}

// RunCommand evaluates the given command and returns the trimmed output
func RunCommand(dir string, name string, args ...string) error {
	os.Setenv("PATH", PathWithBinary())
	e := exec.Command(name, args...)
	if dir != "" {
		e.Dir = dir
	}
	e.Stdout = os.Stdout
	e.Stderr = os.Stdin
	err := e.Run()
	if err != nil {
		log.Errorf("command failed  %s %s\n", name, strings.Join(args, " "))
	}
	return err
}

// RunAWSCommand evaluates the given AWS CLI command and returns the trimmed output
func RunAWSCommand(args ...string) error {
	os.Setenv("PATH", PathWithBinary())
	e := exec.Command("aws", args...)
	accessKey, err := getAWSConfigValue("AWS_ACCESS_KEY_ID")
	if err != nil {
		return fmt.Errorf("Cannot find AWS_ACCESS_KEY_ID in config; Please run 'ziti init'")
	}
	secretKey, err := getAWSConfigValue("AWS_SECRET_ACCESS_KEY")
	if err != nil {
		return fmt.Errorf("Cannot find AWS_SECRET_ACCESS_KEY in config; Please run 'ziti init'")
	}
	e.Env = os.Environ()
	e.Env = append(e.Env, "AWS_ACCESS_KEY_ID="+accessKey)
	e.Env = append(e.Env, "AWS_SECRET_ACCESS_KEY="+secretKey)
	e.Stdout = os.Stdout
	e.Stderr = os.Stdin
	err = e.Run()
	if err != nil {
		log.Errorf("Command failed  %s\n", strings.Join(args, " "))
	}
	return err
}

// getCommandOutput evaluates the given command and returns the trimmed output
func getCommandOutput(dir string, name string, args ...string) (string, error) {
	err := os.Setenv("PATH", PathWithBinary())

	if err != nil {
		return "", fmt.Errorf("could not set environment variable PATH: %s", err)
	}

	e := exec.Command(name, args...)
	if dir != "" {
		e.Dir = dir
	}
	data, err := e.CombinedOutput()
	text := string(data)
	text = strings.TrimSpace(text)
	if err != nil {
		return "", fmt.Errorf("command failed 'aws %s %s': %s %s", name, strings.Join(args, " "), text, err)
	}
	return text, err
}

// getAWSCommandOutput evaluates the given AWS CLI command and returns the trimmed output
func getAWSCommandOutput(args ...string) (string, error) {
	os.Setenv("PATH", PathWithBinary())
	e := exec.Command("aws", args...)
	accessKey, err := getAWSConfigValue("AWS_ACCESS_KEY_ID")
	if err != nil {
		return "", fmt.Errorf("Cannot find AWS_ACCESS_KEY_ID in config; Please run 'ziti init'")
	}
	secretKey, err := getAWSConfigValue("AWS_SECRET_ACCESS_KEY")
	if err != nil {
		return "", fmt.Errorf("Cannot find AWS_SECRET_ACCESS_KEY in config; Please run 'ziti init'")
	}
	e.Env = os.Environ()
	e.Env = append(e.Env, "AWS_ACCESS_KEY_ID="+accessKey)
	e.Env = append(e.Env, "AWS_SECRET_ACCESS_KEY="+secretKey)
	data, err := e.CombinedOutput()
	text := string(data)
	text = strings.TrimSpace(text)
	if err != nil {
		return "", fmt.Errorf("command failed 'aws %s': %s %s", strings.Join(args, " "), text, err)
	}
	return text, err
}

// getAWSCommandOutput evaluates the given AWS CLI command and returns the trimmed output
func getAWSConfigValue(key string) (string, error) {
	viper := viper.New()
	viper.SetConfigType("json")
	viper.SetConfigName(CONFIGFILENAME)
	zitiConfigDir, err := ZitiAppConfigDir("ziti")
	if err != nil {
		return "", err
	}
	viper.AddConfigPath(zitiConfigDir)
	err = viper.ReadInConfig()
	if err != nil {
		return "", err
	}

	val := viper.GetString(key)

	return val, nil
}

// NewEmptyParentCmd creates a new cobra command with no parent
func NewEmptyParentCmd(name string, description string) *cobra.Command {

	return &cobra.Command{
		Use:   name,
		Short: description,
		Long:  description,
	}
}
