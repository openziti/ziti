package network

import (
	"fmt"
	"time"
)

type ServiceCounters interface {
	ServiceDialSuccess(serviceId, terminatorId string)
	ServiceDialFail(serviceId, terminatorId string)
	ServiceDialTimeout(serviceId, terminatorId string)
	ServiceDialOtherError(serviceId string)
}

func (network *Network) ServiceDialSuccess(serviceId, terminatorId string) {
	combinedId := network.joinIds(serviceId, terminatorId)
	network.serviceDialSuccessCounter.Update(combinedId, time.Now(), 1)
}

func (network *Network) ServiceDialFail(serviceId, terminatorId string) {
	combinedId := network.joinIds(serviceId, terminatorId)
	network.serviceDialFailCounter.Update(combinedId, time.Now(), 1)
}

func (network *Network) ServiceDialTimeout(serviceId, terminatorId string) {
	combinedId := network.joinIds(serviceId, terminatorId)
	network.serviceDialTimeoutCounter.Update(combinedId, time.Now(), 1)
}

func (network *Network) ServiceDialOtherError(serviceId string) {
	network.serviceDialOtherErrorCounter.Update(serviceId, time.Now(), 1)
}

func (network *Network) joinIds(serviceId, terminatorId string) string {
	return fmt.Sprintf("%v:%v", serviceId, terminatorId)
}
