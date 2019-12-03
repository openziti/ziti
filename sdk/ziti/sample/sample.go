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

package main

import (
	"github.com/netfoundry/ziti-edge/sdk/ziti"
	"fmt"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(`USAGE: %s <service-name>
     connect to edge service\n`, os.Args[0])
		return
	}

	context := ziti.NewContext()
	con, err := context.Dial(os.Args[1])

	if err != nil {
		log.Fatal(err)
	}

	_, err = con.Write([]byte("Hello from GO-sdk!\n"))

	if err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 1024)
	for {
		n, err := con.Read(buf)
		if err != nil {
			log.Fatal(err)
			break
		}

		log.Println(string(buf[:n]))
	}
}
