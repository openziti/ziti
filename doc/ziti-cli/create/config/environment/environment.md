## ziti create config environment

Display config environment variables

### Synopsis

Creates an env file for generating a controller or router config YAML.
The following can be set to override defaults:
ZITI_HOME                                base dirname used to construct paths              
ZITI_NETWORK_NAME                        base filename used to construct paths             
ZITI_PKI_CTRL_CERT                       Path to the controller's default identity client cert
ZITI_PKI_CTRL_SERVER_CERT                Path to the controller's default identity server cert, including partial chain
ZITI_PKI_CTRL_KEY                        Path to the controller's default identity private key
ZITI_PKI_CTRL_CA                         Path to the controller's bundle of trusted root CAs
ZITI_CTRL_DATABASE_FILE                  Path to the controller's database file            
ZITI_CTRL_BIND_ADDRESS                   The address where the controller will listen for router control plane connections
ZITI_CTRL_ADVERTISED_ADDRESS             The address routers will use to connect to the controller
ZITI_CTRL_EDGE_ALT_ADVERTISED_ADDRESS    The controller's edge API alternative address     
ZITI_CTRL_ADVERTISED_PORT                TCP port routers will use to connect to the controller
ZITI_CTRL_EDGE_BIND_ADDRESS              The address where the controller will listen for edge API connections
ZITI_CTRL_EDGE_ADVERTISED_PORT           TCP port of the controller's edge API             
ZITI_PKI_SIGNER_CERT                     Path to the controller's edge signer CA cert      
ZITI_PKI_SIGNER_KEY                      Path to the controller's edge signer CA key       
ZITI_EDGE_IDENTITY_ENROLLMENT_DURATION   The identity enrollment duration in minutes       
ZITI_ROUTER_ENROLLMENT_DURATION          The router enrollment duration in minutes         
ZITI_CTRL_EDGE_ADVERTISED_ADDRESS        The controller's edge API address                 
ZITI_PKI_EDGE_CERT                       Path to the controller's web identity client certificate
ZITI_PKI_EDGE_SERVER_CERT                Path to the controller's web identity server certificate, including partial chain
ZITI_PKI_EDGE_KEY                        Path to the controller's web identity private key 
ZITI_PKI_EDGE_CA                         Path to the controller's web identity root CA cert
ZITI_PKI_ALT_SERVER_CERT                 Path to the controller's default identity alternative server certificate; requires ZITI_PKI_ALT_SERVER_KEY
ZITI_PKI_ALT_SERVER_KEY                  Path to the controller's default identity alternative private key. Requires ZITI_PKI_ALT_SERVER_CERT
ZITI_ROUTER_NAME                         A filename prefix for the router's key and certs  
ZITI_ROUTER_PORT                         TCP port where the router listens for edge connections from endpoint identities
ZITI_ROUTER_LISTENER_BIND_PORT           TCP port where the router will listen for and advertise links to other routers
ZITI_ROUTER_IDENTITY_CERT                Path to the router's client certificate           
ZITI_ROUTER_IDENTITY_SERVER_CERT         Path to the router's server certificate           
ZITI_ROUTER_IDENTITY_KEY                 Path to the router's private key                  
ZITI_ROUTER_IDENTITY_CA                  Path to the router's bundle of trusted root CA certs
ZITI_ROUTER_IP_OVERRIDE                  Additional IP SAN of the router                   
ZITI_ROUTER_ADVERTISED_ADDRESS           The router's advertised address and DNS SAN       
ZITI_ROUTER_TPROXY_RESOLVER              The bind URI to listen for DNS requests in tproxy mode
ZITI_ROUTER_DNS_IP_RANGE                 The CIDR range to use for Ziti DNS in tproxy mode 
ZITI_ROUTER_CSR_C                        The country (C) to use for router CSRs            
ZITI_ROUTER_CSR_ST                       The state/province (ST) to use for router CSRs    
ZITI_ROUTER_CSR_L                        The locality (L) to use for router CSRs           
ZITI_ROUTER_CSR_O                        The organization (O) to use for router CSRs       
ZITI_ROUTER_CSR_OU                       The organization unit to use for router CSRs      
ZITI_ROUTER_CSR_SANS_DNS                 Additional DNS SAN of the router                  


```
ziti create config environment [flags]
```

### Examples

```
  # Display environment variables and their values
  ziti create config environment
  
  # Print an environment file to the console
  ziti create config environment --output stdout
```

### Options

```
  -h, --help            help for environment
      --no-shell        Disable printing assignments prefixed with 'SET' (Windows) or 'export' (Unix)
  -o, --output string   designated output destination for config, use "stdout" or a filepath. (default "stdout")
  -v, --verbose         Enable verbose logging. Logging will be sent to stdout if the config output is sent to a file. If output is sent to stdout, logging will be sent to stderr
```

### SEE ALSO

* [ziti create config](../config.md)	 - Creates a config file for specified Ziti component using environment variables

###### Auto generated by spf13/cobra on 26-Aug-2024
