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

package download

import (
	"bufio"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"

	"io"
	"os"
)

type Output struct {
	outputJson bool
	outputYaml bool
	filename   string
	writer     *bufio.Writer
	verbose    bool
}

func NewOutputToFile(verbose bool, outputJson bool, outputYaml bool, filename string) (*Output, error) {
	file, err := os.Create(filename)
	if err != nil {
		log.WithError(err).Error("Error creating file for writing")
		return nil, err
	}
	writer := bufio.NewWriter(file)
	output, err := NewOutputToWriter(verbose, outputJson, outputYaml, writer)
	output.filename = filename
	return output, err
}

func NewOutputToWriter(verbose bool, outputJson bool, outputYaml bool, writer io.Writer) (*Output, error) {
	output := Output{}
	output.verbose = verbose
	output.outputJson = outputJson
	output.outputYaml = outputYaml
	output.writer = bufio.NewWriter(writer)
	return &output, nil
}

func (output Output) Write(data any) error {
	var formatted []byte
	var err error
	if output.outputYaml {
		if output.verbose {
			_, _ = fmt.Fprintf(os.Stderr, "\u001B[2KFormatting as Yaml\r\n")
		}
		formatted, err = output.ToYaml(data)
	} else {
		if output.verbose {
			_, _ = fmt.Fprintf(os.Stderr, "\u001B[2KFormatting as JSON\r\n")
		}
		formatted, err = output.ToJson(data)
	}
	if err != nil {
		return err
	}

	if output.verbose {
		if output.filename != "" {
			_, _ = fmt.Fprintf(os.Stderr, "\u001B[2KWriting to file: %s\r\n", output.filename)
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "\u001B[2KWriting output to writer\r\n")
		}
	}
	bytes, err := output.writer.Write(formatted)
	if err != nil {
		log.WithError(err).Error("Error writing data to output")
		return err
	}

	if output.verbose {
		if output.filename != "" {
			log.
				WithError(err).
				WithFields(map[string]interface{}{
					"bytes":    bytes,
					"filename": output.filename,
				}).
				Debug("Wrote data")
		} else {
			log.
				WithField("bytes", bytes).
				Debug("Wrote data")
		}
	}

	err = output.writer.Flush()
	if err != nil {
		log.
			WithError(err).
			Error("Error flushing data to output")
		return err
	}

	return nil
}

func (output Output) ToJson(data any) ([]byte, error) {

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.
			WithError(err).
			Error("Error writing data as JSON")
		return nil, err
	}

	return jsonData, nil
}

func (output Output) ToYaml(data any) ([]byte, error) {

	yamlData, err := yaml.Marshal(data)
	if err != nil {
		log.
			WithError(err).
			Error("Error writing data as Yaml")
		return nil, err
	}

	return yamlData, nil
}
