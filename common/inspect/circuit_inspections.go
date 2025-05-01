package inspect

import "github.com/openziti/sdk-golang/xgress"

type CircuitsDetail struct {
	Circuits map[string]*CircuitDetail `json:"circuits"`
}

type CircuitDetail struct {
	Id                string            `json:"id"`
	TimeSinceActivity string            `json:"timeSinceActivity"`
	CtrlId            string            `json:"ctrlId"`
	Routes            map[string]string `json:"routes"`
}

type EdgeXgFwdInspectDetail struct {
	ChannelConnId       string            `json:"channelConnId"`
	IdentityId          string            `json:"identityId"`
	CircuitId           string            `json:"circuitId"`
	Originator          string            `json:"originator"`
	Address             string            `json:"address"`
	CtrlId              string            `json:"ctrlId"`
	EdgeConnId          uint32            `json:"edgeConnId"`
	TimeSinceLastLinkRx string            `json:"timeSinceLastLinkRx"`
	Tags                map[string]string `json:"tags"`
}

type EdgeListenerCircuits struct {
	Circuits map[string]*EdgeXgFwdInspectDetail `json:"circuits"`
}

type SdkCircuits struct {
	Circuits map[string]*SdkCircuitDetail `json:"circuits"`
	Errors   []string                     `json:"errors,omitempty"`
}

type SdkCircuitDetail struct {
	IdentityId    string `json:"identityId"`
	ChannelConnId string `json:"channelConnId"`
	*xgress.CircuitDetail
}
