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

// Package vagrantutil is a high level wrapper around Vagrant which provides an
// idiomatic go API.
package vagrantutil

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

//go:generate stringer -type=Status  -output=stringer.go

type Status int

const (
	// Some possible states:
	// https://github.com/mitchellh/vagrant/blob/master/templates/locales/en.yml#L1504
	Unknown Status = iota
	NotCreated
	Running
	Saved
	PowerOff
	Aborted
	Preparing
)

// Box represents a single line of `vagrant box list` output.
type Box struct {
	Name     string
	Provider string
	Version  string
}

// CommandOutput is the streaming output of a command
type CommandOutput struct {
	Line  string
	Error error
}

type Vagrant struct {
	// VagrantfilePath is the directory with specifies the directory where
	// Vagrantfile is being stored.
	VagrantfilePath string

	// ProviderName overwrites the default provider used for the Vagrantfile.
	ProviderName string

	// ID is the unique ID of the given box.
	ID string

	// State is populated/updated if the Status() or List() method is called.
	State string

	// Log is used for logging output of vagrant commands in debug mode.
	// Log logging.Logger
}

// NewVagrant returns a new Vagrant instance for the given path. The path
// should be unique. If the path already exists in the system it'll be used, if
// not a new setup will be createad.
func NewVagrant(path string) (*Vagrant, error) {
	if path == "" {
		return nil, errors.New("vagrant: path is empty")
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}

	return &Vagrant{
		VagrantfilePath: path,
	}, nil
}

// Create creates the vagrantFile in the pre initialized vagrant path.
func (v *Vagrant) Create(vagrantFile string) error {
	// Recreate the directory in case it was removed between
	// call to NewVagrant and Create.
	if err := os.MkdirAll(v.VagrantfilePath, 0755); err != nil {
		return v.error(err)
	}

	v.debugf("create:\n%s", vagrantFile)

	return v.error(ioutil.WriteFile(v.vagrantfile(), []byte(vagrantFile), 0644))
}

// Version returns the current installed vagrant version
func (v *Vagrant) Version() (string, error) {
	out, err := v.vagrantCommand().run("version", "--machine-readable")
	if err != nil {
		return "", err
	}

	records, err := parseRecords(out)
	if err != nil {
		return "", v.error(err)
	}

	versionInstalled, err := parseData(records, "version-installed")
	if err != nil {
		return "", v.error(err)
	}

	return versionInstalled, nil
}

// Status returns the state of the box, such as "Running", "NotCreated", etc...
func (v *Vagrant) Status() (s Status, err error) {
	defer func() {
		v.State = s.String()
	}()

	var notCreated bool
	cmd := v.vagrantCommand()
	cmd.ignoreErr = func(err error) bool {
		if isNotCreated(err) {
			notCreated = true
			return true
		}

		return false
	}

	out, err := cmd.run("status", "--machine-readable")
	if err != nil {
		return Unknown, err
	}

	if notCreated {
		return NotCreated, nil
	}

	records, err := parseRecords(out)
	if err != nil {
		return Unknown, v.error(err)
	}

	status, err := parseData(records, "state")
	if err != nil {
		return Unknown, v.error(err)
	}

	s, err = toStatus(status)
	if err != nil {
		return Unknown, err
	}

	return s, nil
}

func (v *Vagrant) Provider() (string, error) {
	out, err := v.vagrantCommand().run("status", "--machine-readable")
	if err != nil {
		return "", err
	}

	records, err := parseRecords(out)
	if err != nil {
		return "", v.error(err)
	}

	provider, err := parseData(records, "provider-name")
	if err != nil {
		return "", v.error(err)
	}

	return provider, nil
}

// List returns all available boxes on the system. Under the hood it calls
// "global-status" and parses the output.
func (v *Vagrant) List() ([]*Vagrant, error) {
	// Refresh box status cache. So it does not report that aborted
	// box is running etc.
	_, err := v.vagrantCommand().run("global-status", "--prune")
	if err != nil {
		return nil, err
	}
	out, err := v.vagrantCommand().run("global-status")
	if err != nil {
		return nil, err
	}

	output := make([][]string, 0)

	scanner := bufio.NewScanner(strings.NewReader(out))
	collectStarted := false

	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "--") {
			scanner.Scan() // advance to next line
			collectStarted = true
		}

		if !collectStarted {
			continue
		}

		trimmedLine := strings.TrimSpace(scanner.Text())
		if trimmedLine == "" {
			break // we are finished with collecting the boxes
		}

		output = append(output, strings.Fields(trimmedLine))
	}
	if err := scanner.Err(); err != nil {
		return nil, v.error(err)
	}

	boxes := make([]*Vagrant, len(output))

	for i, box := range output {
		// example box: [0c269f6 default virtualbox aborted /Users/fatih/path]
		boxes[i] = &Vagrant{
			ID:              box[0],
			VagrantfilePath: box[len(box)-1],
			State:           box[3],
		}
	}

	return boxes, nil
}

// Up executes "vagrant up" for the given vagrantfile. The returned channel
// contains the output stream. At the end of the output, the error is put into
// the Error field if there is any.
func (v *Vagrant) Up() (<-chan *CommandOutput, error) {
	if v.ProviderName != "" {
		return v.vagrantCommand().start("up", "--provider", v.ProviderName)
	}

	return v.vagrantCommand().start("up")
}

// Halt executes "vagrant halt". The returned reader contains the output
// stream. The client is responsible of calling the Close method of the
// returned reader.
func (v *Vagrant) Halt() (<-chan *CommandOutput, error) {
	return v.vagrantCommand().start("halt")
}

// Destroy executes "vagrant destroy". The returned reader contains the output
// stream. The client is responsible of calling the Close method of the
// returned reader.
func (v *Vagrant) Destroy() (<-chan *CommandOutput, error) {
	if _, err := os.Stat(v.VagrantfilePath); os.IsNotExist(err) {
		// Makes Destroy idempotent if called consecutively multiple times on
		// the same path.
		//
		// Returning closed channel to not make existing like the one
		// below hang:
		//
		//   ch, err := vg.Destroy()
		//   if err != nil {
		//     ...
		//   }
		//   for line := range ch {
		//     ...
		//   }
		//
		ch := make(chan *CommandOutput)
		close(ch)

		return ch, nil
	}

	cmd := v.vagrantCommand()
	cmd.onSuccess = func() {
		// cleanup vagrant directory on success, as it's no longer needed;
		// after destroy it should be not possible to call vagrant up
		// again, call to Create is required first
		if err := os.RemoveAll(v.VagrantfilePath); err != nil {
			v.debugf("failed to cleanup %q after destroy: %s", v.VagrantfilePath, err)
		}

		// We leave empty directory to not make other commands fail
		// due to missing cwd.
		//
		// TODO(rjeczalik): rework lookup to use box id instead
		if err := os.MkdirAll(v.VagrantfilePath, 0755); err != nil {
			v.debugf("failed to create empty dir %q after destroy: %s", v.VagrantfilePath, err)
		}
	}
	// if vagrant box is not created, return success - the destroy
	// should be effectively a nop
	cmd.ignoreErr = isNotCreated

	return cmd.start("destroy", "--force")
}

var stripFmt = strings.NewReplacer("(", "", ",", "", ")", "")

// BoxList executes "vagrant box list", parses the output and returns all
// available base boxes.
func (v *Vagrant) BoxList() ([]*Box, error) {
	out, err := v.vagrantCommand().run("box", "list")
	if err != nil {
		return nil, err
	}

	var boxes []*Box
	scanner := bufio.NewScanner(strings.NewReader(out))

	for scanner.Scan() {
		line := strings.TrimSpace(stripFmt.Replace(scanner.Text()))
		if line == "" {
			continue
		}

		var box Box
		n, err := fmt.Sscanf(line, "%s %s %s", &box.Name, &box.Provider, &box.Version)
		if err != nil {
			return nil, v.errorf("%s for line: %s", err, line)
		}
		if n != 3 {
			return nil, v.errorf("unable to parse output line: %s", line)
		}

		boxes = append(boxes, &box)
	}
	if err := scanner.Err(); err != nil {
		return nil, v.error(err)
	}

	return boxes, nil
}

// BoxAdd executes "vagrant box add" for the given box. The returned channel
// contains the output stream. At the end of the output, the error is put into
// the Error field if there is any.
//
// TODO(rjeczalik): BoxAdd does not support currently adding boxes directly
// from files.
func (v *Vagrant) BoxAdd(box *Box) (<-chan *CommandOutput, error) {
	args := append([]string{"box", "add"}, toArgs(box)...)
	return v.vagrantCommand().start(args...)
}

// BoxRemove executes "vagrant box remove" for the given box. The returned channel
// contains the output stream. At the end of the output, the error is put into
// the Error field if there is any.
func (v *Vagrant) BoxRemove(box *Box) (<-chan *CommandOutput, error) {
	args := append([]string{"box", "remove"}, toArgs(box)...)
	return v.vagrantCommand().start(args...)
}

// SSH executes "vagrant ssh" for the given vagrantfile. The returned channel
// contains the output stream. At the end of the output, the error is put into
// the Error field if there is any.
func (v *Vagrant) SSH(command string) (<-chan *CommandOutput, error) {
	args := []string{"ssh", "-c", command}
	return v.vagrantCommand().start(args...)
}

// vagrantfile returns the Vagrantfile path
func (v *Vagrant) vagrantfile() string {
	return filepath.Join(v.VagrantfilePath, "Vagrantfile")
}

// vagrantfileExists checks if a Vagrantfile exists in the given path. It
// returns a nil error if exists.
func (v *Vagrant) vagrantfileExists() error {
	if _, err := os.Stat(v.vagrantfile()); os.IsNotExist(err) {
		return err
	}
	return nil
}

// vagrantCommand creates a command which is setup to be run next to
// Vagrantfile
func (v *Vagrant) vagrantCommand() *command {
	return newCommand(v.VagrantfilePath)
}

func (v *Vagrant) debugf(format string, args ...interface{}) {
	// if v.Log != nil {
	log.Printf(format, args...)
	// }
}

func (v *Vagrant) errorf(format string, args ...interface{}) error {
	err := fmt.Errorf(format, args...)
	v.debugf("%s", err)
	return err
}

func (v *Vagrant) error(err error) error {
	if err != nil {
		v.debugf("%s", err)
	}
	return err
}

// Runs "ssh-config" and returns the output.
func (v *Vagrant) SSHConfig() (string, error) {
	out, err := v.vagrantCommand().run("ssh-config")
	if err != nil {
		return "", err
	}
	return out, nil
}

// toArgs converts the given box to argument list for `vagrant box add/remove`
// commands
func toArgs(box *Box) (args []string) {
	if box.Provider != "" {
		args = append(args, "--provider", box.Provider)
	}
	if box.Version != "" {
		args = append(args, "--box-version", box.Version)
	}
	return append(args, box.Name)
}

// toStatus converts the given state string to Status type
func toStatus(state string) (Status, error) {
	switch state {
	case "running":
		return Running, nil
	case "not_created":
		return NotCreated, nil
	case "saved":
		return Saved, nil
	case "poweroff":
		return PowerOff, nil
	case "aborted":
		return Aborted, nil
	case "preparing":
		return Preparing, nil
	default:
		return Unknown, fmt.Errorf("Unknown state: %s", state)
	}

}
