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

	url := fmt.Sprintf("https://files-%s%s.s3-us-west-1.amazonaws.ziti/%s.zip", hostType, urlExtra, fileSize)

	filename := uuid.NewString()

	var cmds []string
	cmds = append(cmds, fmt.Sprintf("echo '%s  %s' > checksums", hashes[fileSize], filename))

	var cmd string
	if client == ClientCurl {
		cmd = fmt.Sprintf(`set -o pipefail; curl -k --header "Host: ziti-smoketest-files.s3-us-west-1.amazonaws.com" --fail-early --fail-with-body -SL -o %s %s 2>&1`, filename, url)
	} else if client == ClientWget {
		cmd = fmt.Sprintf(`set -o pipefail; wget --no-check-certificate --header "Host: ziti-smoketest-files.s3-us-west-1.amazonaws.com" -O %s -t 5 -T 5 %s 2>&1`, filename, url)
	}
	cmds = append(cmds, cmd)
	cmds = append(cmds, "md5sum -c checksums")

	timeout := timeouts[fileSize]
	return host.ExecLoggedWithTimeout(timeout, cmds...)
}
