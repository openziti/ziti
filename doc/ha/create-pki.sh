# Create the trust root, a self-signed CA
ziti pki create ca --trust-domain ha.test --pki-root ./pki --ca-file ca --ca-name 'HA Example Trust Root'

# Create the signing and server certs for ctrl1
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl1 --intermediate-name 'Controller One Signing Cert'
ziti pki create server --pki-root ./pki --ca-name ctrl1 --dns localhost --ip 127.0.0.1 --spiffe-id 'controller/ctrl1'

# Create the signing and server certs for ctrl2
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl2 --intermediate-name 'Controller Two Signing Cert'
ziti pki create server --pki-root ./pki --ca-name ctrl2 --dns localhost --ip 127.0.0.1 --spiffe-id 'controller/ctrl2'

# Create the signing and server certs for ctrl3
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl3 --intermediate-name 'Controller Three Signing Cert'
ziti pki create server --pki-root ./pki --ca-name ctrl3 --dns localhost --ip 127.0.0.1 --spiffe-id 'controller/ctrl3'
