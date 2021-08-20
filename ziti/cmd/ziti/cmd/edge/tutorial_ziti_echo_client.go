package edge

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func NewZitiEchoClient(identityJson string) (*zitiEchoClient, error) {
	config, err := config.NewFromFile(identityJson)
	if err != nil {
		return nil, err
	}

	zitiContext := ziti.NewContextWithConfig(config)

	dial := func(_ context.Context, _ string, addr string) (net.Conn, error) {
		service := strings.Split(addr, ":")[0] // assume host is service
		return zitiContext.Dial(service)
	}

	zitiTransport := http.DefaultTransport.(*http.Transport).Clone()
	zitiTransport.DialContext = dial

	return &zitiEchoClient{
		httpClient: &http.Client{Transport: zitiTransport},
	}, nil
}

type zitiEchoClient struct {
	httpClient *http.Client
}

func (self *zitiEchoClient) echo(input string) error {
	u := fmt.Sprintf("http://echo?input=%v", url.QueryEscape(input))
	resp, err := self.httpClient.Get(u)
	if err == nil {
		c := color.New(color.FgGreen, color.Bold)
		c.Print("\nziti-http-echo-client: ")
		_, err = io.Copy(os.Stdout, resp.Body)
	}
	return err
}
