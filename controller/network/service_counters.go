package network

import "time"

type ServiceCounters interface {
	ServiceDialSuccess(serviceId string)
	ServiceDialFail(serviceId string)
	ServiceDialTimeout(serviceId string)
	ServiceDialOtherError(serviceId string)
}

func (network *Network) ServiceDialSuccess(serviceId string) {
	network.serviceDialSuccessCounter.Update(serviceId, time.Now(), 1)
}

func (network *Network) ServiceDialFail(serviceId string) {
	network.serviceDialFailCounter.Update(serviceId, time.Now(), 1)
}

func (network *Network) ServiceDialTimeout(serviceId string) {
	network.serviceDialTimeoutCounter.Update(serviceId, time.Now(), 1)
}

func (network *Network) ServiceDialOtherError(serviceId string) {
	network.serviceDialOtherErrorCounter.Update(serviceId, time.Now(), 1)
}
