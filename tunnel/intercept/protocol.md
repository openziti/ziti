# Notes about inter-tunneler communications

## AppData

The tunneler intercept side can send data to the hosting side, out-of-band as part of the connection
setup. This data is passed as 'appData' and sent as a JSON map.

The JSON map may contain the following fields:

### dst_protocol

The protocol (tcp/udp) of the intercepted traffic. Used by the hosting tunneler if the configuration
has `forwardProtocol`. When the service is dialed, it will forward using the same protocol
that was intercepted.

### dst_ip

The destination IP address of the intercepted traffic. Used by the hosting tunneler if the
configuration has `forwardAddress`. When the service is dialed, it will forward to the same
IP address that was intercepted.

### dst_port

The destination port of the intercepted traffic. Used by the hosting tunneler if the configuration
has `forwardPort`. When the service is dialed, it will forward to the same port that was
intercepted.

### src_ip

The source ip of the intercepted traffic. Used on the intercept side to as an input to the source_addr
template. Not currently used on the hosting side.

### src_port

The source port of the intercepted traffic. Used on the intercept side to as an input to the
source_addr template. Not currently used on the hosting side.

### source_addr

The source address to spoof for traffic exiting the hosting tunneler. Used to implement transparent IP.
May contain variable references:
- $src_ip
- $tunneler_id.name
- $tunneler_id.tag[TAGNAME]

`source_addr` may optionally include a semicolon-separated port. The `src_port` and `dst_port` appData
values can be referenced as variables; e.g.:
- $src_ip:$src_port
- $src_ip:$dst_port
