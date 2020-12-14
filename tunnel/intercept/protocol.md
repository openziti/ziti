# Notes about inter-tunneler communications

## AppData

The tunneler intercept side can send data to the hosting side, out-of-band as part of the connection
setup. This data is passed as 'appData' and sent as a JSON map.

The JSON map may contain the following fields:

### intercepted_protocol

The protocol (tcp/udp) of the intercepted traffic. Used by the hosting tunneler if the configuration
has `dialInterceptedProtocol`. When the service is dialed, it will forward using the same protocol
that was intercepted.

### intercepted_ip

The destination IP address of the intercepted traffic. Used by the hosting tunneler if the
configuration has `dialInterceptedAddress`. When the service is dialed, it will forward to the same
IP address that was intercepted.

### intercepted_port

The destination port of the intercepted traffic. Used by the hosting tunneler if the configuration
has `dialInterceptedPort`. When the service is dialed, it will forward to the same port that was
intercepted.

### source_ip

The source IP to spoof for traffic exiting the hosting tunneler. Used to implement transparent IP.

### client_ip

The source ip of the intercepted traffic. Used on the intercept side to as an input to the source_ip
template. Not currently used on the hosting side.

### client_port

The source port of the intercepted traffic. Used on the intercept side to as an input to the
source_ip template. Not currently used on the hosting side.
 

