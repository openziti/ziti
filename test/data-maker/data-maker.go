// +build utils

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

package main

import (
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/sirupsen/logrus"
	"io"
	"log"
	"os"
	"unicode"
)

func main() {
	service := "data-maker"
	pfxlog.Logger().Infof("args %v", os.Args)
	if len(os.Args) > 1 {
		service = os.Args[1]
	}
	pfxlog.Logger().Infof("Attempting to host service %v", service)
	logrus.SetLevel(logrus.DebugLevel)
	ztContext := ziti.NewContext()
	listener, err := ztContext.Listen(service)
	if err != nil {
		log.Fatalf("failed to host service '%v': %v", service, err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}
		fmt.Println("Connection received")
		go respond(conn)
	}
}

func respond(conn io.ReadWriteCloser) {
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Printf("Error while closing connection: %v\n", err)
		}
		fmt.Println("Connection closed")
	}()

	for {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			fmt.Printf("Error while reading: %v\n", err)
			break
		}
		fmt.Printf("Read %v bytes\n", n)

		size := 0
		sizeUnits := 'b'

		cmd := string(buf[:n])
		for _, ch := range cmd {
			if unicode.IsDigit(ch) {
				num := (int)(ch - '0')
				size = size*10 + num
			} else {
				sizeUnits = ch
				break
			}
		}

		var writeBuf []byte

		if size == 0 {
			writeBuf = []byte("Did not understand input, please try again\n")
		} else {
			switch sizeUnits {
			case 'b':
				// already there
			case 'k':
				size = size * 1024
			case 'm':
				size = size * 1024 * 1024
			default:
				size = 0
				writeBuf = []byte("Did not understand input, please use format <size><unit>. Ex: 1b or 10k or 2m\n")
			}
		}

		if size > 0 {
			writeBuf = make([]byte, size)
			fill(writeBuf)
			fmt.Printf("sending back %v.\n.Data length: %v\n", string(writeBuf), len(writeBuf))
		}

		if _, err := conn.Write(writeBuf); err != nil {
			fmt.Printf("Error while writing: %v\n", err)
			break
		}

		summaryBuf := []byte(fmt.Sprintf("\nWrote %v bytes\n", len(writeBuf)))
		if _, err := conn.Write(summaryBuf); err != nil {
			fmt.Printf("Error while writing: %v\n", err)
			break
		}

		fmt.Printf("Wrote %v bytes\n", n)
	}
}

func fill(buf []byte) {
	for i := 0; i < len(buf); i++ {
		if (i+1)%1024 == 0 {
			k := (i + 1) / 1024
			kBuf := []byte(fmt.Sprintf("%vk", k))
			for j := 0; j < len(kBuf); j++ {
				buf[i-j] = kBuf[len(kBuf)-(j+1)]
			}
		} else {
			buf[i] = '.'
		}
	}
}
