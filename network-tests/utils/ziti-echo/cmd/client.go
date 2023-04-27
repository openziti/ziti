package cmd

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/openziti/sdk-golang/ziti"
	"github.com/openziti/sdk-golang/ziti/config"
	"github.com/spf13/cobra"
)

var clientCmd = &cobra.Command{
	Use:   "client {string-to-echo}",
	Short: "Simple ziti enabled echo client",
	Args:  cobra.MinimumNArgs(1),
	Run:   echoClient,
}

func init() {
	clientCmd.Flags().SetInterspersed(true)
	rootCmd.AddCommand(clientCmd)
}

func echoClient(cmd *cobra.Command, args []string) {
	cfg, err := config.NewFromFile(identityFile)
	if err != nil {
		log.Fatal(err)
	}

	zitiContext := ziti.NewContextWithConfig(cfg)

	dial := func(_ context.Context, _, addr string) (net.Conn, error) {
		service := strings.Split(addr, ":")[0]
		return zitiContext.Dial(service)
	}

	zitiTransport := http.DefaultTransport.(*http.Transport).Clone()
	zitiTransport.DialContext = dial

	zec := &zitiEchoClient{
		client: &http.Client{Transport: zitiTransport},
	}

	resp, err := zec.Echo(strings.Join(args, " "))
	if resp != "" {
		fmt.Print(resp)
	}
	if err != nil {
		log.Fatal(err)
	}
}

type zitiEchoClient struct {
	client *http.Client
}

func (zec *zitiEchoClient) Echo(input string) (string, error) {
	u := fmt.Sprintf("http://echo?input=%v", url.QueryEscape(input))
	resp, err := zec.client.Get(u)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), err
}
