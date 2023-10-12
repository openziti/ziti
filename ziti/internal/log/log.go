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

package log

import (
	"fmt"

	"github.com/fatih/color"
)

func Infof(msg string, args ...interface{}) {
	Info(fmt.Sprintf(msg, args...))
}

func Info(msg string) {
	fmt.Print(msg)
}

func Infoln(msg string) {
	fmt.Println(msg)
}

func Blank() {
	fmt.Println()
}

func Warnf(msg string, args ...interface{}) {
	Warn(fmt.Sprintf(msg, args...))
}

func Warn(msg string) {
	color.Yellow(msg)
}

func Errorf(msg string, args ...interface{}) {
	Error(fmt.Sprintf(msg, args...))
}

func Error(msg string) {
	color.Red(msg)
}

func Fatalf(msg string, args ...interface{}) {
	Fatal(fmt.Sprintf(msg, args...))
}

func Fatal(msg string) {
	color.Red(msg)
}
