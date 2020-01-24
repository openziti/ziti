/*
	Copyright 2019 NetFoundry, Inc.

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

// Package agent provides hooks programs can register to retrieve
// diagnostics data by using the Ziti CLI.
package agent

import (
	"os"
	"testing"
)

func TestListen(t *testing.T) {
	err := Listen(Options{})
	if err != nil {
		t.Fatal(err)
	}
	Close()
}

func TestAgentClose(t *testing.T) {
	err := Listen(Options{})
	if err != nil {
		t.Fatal(err)
	}
	Close()
	_, err = os.Stat(portfile)
	if !os.IsNotExist(err) {
		t.Fatalf("portfile = %q doesn't exist; err = %v", portfile, err)
	}
	if portfile != "" {
		t.Fatalf("got = %q; want empty portfile", portfile)
	}
}

func TestUseCustomConfigDir(t *testing.T) {
	err := Listen(Options{
		ConfigDir:       os.TempDir(),
		ShutdownCleanup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	Close()
}

func TestAgentListenMultipleClose(t *testing.T) {
	err := Listen(Options{})
	if err != nil {
		t.Fatal(err)
	}
	Close()
	Close()
	Close()
	Close()
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		val  uint64
		want string
	}{
		{1023, "1023 bytes"},
		{1024, "1.00KB (1024 bytes)"},
		{1024*1024 - 100, "1023.90KB (1048476 bytes)"},
		{1024 * 1024, "1.00MB (1048576 bytes)"},
		{1024 * 1025, "1.00MB (1049600 bytes)"},
		{1024 * 1024 * 1024, "1.00GB (1073741824 bytes)"},
		{1024*1024*1024 + 430*1024*1024, "1.42GB (1524629504 bytes)"},
		{1024 * 1024 * 1024 * 1024 * 1024, "1.00PB (1125899906842624 bytes)"},
		{1024 * 1024 * 1024 * 1024 * 1024 * 1024, "1024.00PB (1152921504606846976 bytes)"},
	}
	for _, tt := range tests {
		result := formatBytes(tt.val)
		if result != tt.want {
			t.Errorf("formatBytes(%v) = %q; want %q", tt.val, result, tt.want)
		}
	}
}
