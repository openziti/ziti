package smoke

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/openziti/fablab/kernel/model"
	"time"
)

var hashes = map[string]string{
	"1KB":   "0f343b0931126a20f133d67c2b018a3b",
	"100KB": "4c6426ac7ef186464ecbb0d81cbfcb1e",
	"20MB":  "8f4e33f3dc3e414ff94e5fb6905cba8c",
}

var timeouts = map[string]time.Duration{
	"1KB":   10 * time.Second,
	"100KB": 10 * time.Second,
	"20MB":  40 * time.Second,
}

type HttpClient string

const (
	ClientCurl HttpClient = "curl"
	ClientWget HttpClient = "wget"
)

var FileSizes = []string{"1KB", "100KB", "20MB"}
var HttpClients = []HttpClient{ClientCurl, ClientWget}

func TestFileDownload(hostSelector string, client HttpClient, hostType string, encrypted bool, fileSize string) (string, error) {
	host := model.GetModel().MustSelectHost("." + hostSelector + "-client")

	urlExtra := ""
	if !encrypted {
		urlExtra = "-unencrypted"
	}

	url := fmt.Sprintf("http://files-%s%s.ziti/%s.zip", hostType, urlExtra, fileSize)

	filename := uuid.NewString() + ".tmp"

	var cmds []string
	cmds = append(cmds, fmt.Sprintf("echo '%s  %s' > checksums", hashes[fileSize], filename))

	var cmd string
	if client == ClientCurl {
		cmd = fmt.Sprintf(`set -o pipefail; rm -f *.tmp; curl -k --fail-early --fail-with-body -SL -o %s %s 2>&1`, filename, url)
	} else if client == ClientWget {
		cmd = fmt.Sprintf(`set -o pipefail; rm -f *.tmp; wget --no-check-certificate -O %s -t 5 -T 5 %s 2>&1`, filename, url)
	}
	cmds = append(cmds, cmd)
	cmds = append(cmds, "md5sum -c checksums")

	timeout := timeouts[fileSize]
	return host.ExecLoggedWithTimeout(timeout, cmds...)
}

func TestIperf(clientHostSelector, hostType string, encrypted, reversed bool, run model.Run) (string, error) {
	c, err := model.GetModel().SelectComponent(".iperf." + hostType)
	if err != nil {
		return "", err
	}
	if err = c.Type.Stop(run, c); err != nil {
		return "", err
	}
	iperfServer := c.Type.(model.ServerComponent)
	if err = iperfServer.Start(run, c); err != nil {
		return "", err
	}

	host, err := model.GetModel().SelectHost("." + clientHostSelector + "-client")
	if err != nil {
		return "", err
	}

	urlExtra := ""
	if !encrypted {
		urlExtra = "-unencrypted"
	}

	addr := fmt.Sprintf("iperf-%s%s.ziti", hostType, urlExtra)

	extraOptions := ""
	if reversed {
		extraOptions += " -R"
	}

	cmd := fmt.Sprintf(`set -o pipefail; iperf3 -c %s -P 1 -t 10 %s`, addr, extraOptions)

	return host.ExecLoggedWithTimeout(40*time.Second, cmd)
}
