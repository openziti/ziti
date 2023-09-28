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

package env

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
)

type PemProducer struct{}

func (p PemProducer) Produce(writer io.Writer, i interface{}) error {
	if buffer, ok := i.([]byte); ok {
		_, err := writer.Write(buffer)
		return err
	} else if reader, ok := i.(io.Reader); ok {
		buffer, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		_, err = writer.Write(buffer)
		return err
	} else if buffer, ok := i.(string); ok {
		_, err := writer.Write([]byte(buffer))
		return err
	}
	return fmt.Errorf("unsupported type for PEM producer: %T", i)
}

type YamlProducer struct{}

func (p YamlProducer) Produce(writer io.Writer, i interface{}) error {
	enc := yaml.NewEncoder(writer)
	return enc.Encode(i)
}
