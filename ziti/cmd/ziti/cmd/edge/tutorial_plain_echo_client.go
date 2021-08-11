package edge

import (
	"fmt"
	"github.com/fatih/color"
	"io"
	"net/http"
	"net/url"
	"os"
)

type plainEchoClient struct {
	host string
	port uint16
}

func (self *plainEchoClient) echo(input string) error {
	input = url.QueryEscape(input)
	u := fmt.Sprintf("http://%v:%v?input=%v", self.host, self.port, input)
	resp, err := (&http.Client{}).Get(u)
	if err == nil {
		c := color.New(color.FgBlue, color.Bold)
		c.Print("\nplain-http-echo-client: ")
		_, err = io.Copy(os.Stdout, resp.Body)
	}
	return err
}
