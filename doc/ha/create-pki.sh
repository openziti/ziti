# Create the trust root, a self-signed CA
ziti pki create ca --trust-domain ha.test --pki-root ./pki --ca-file ca --ca-name 'HA Example Trust Root'

# Create the controller 1 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl1 --intermediate-name 'Controller One Signing Cert'

# Create the controller 1 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl1 --dns localhost --ip 127.0.0.1 --server-name ctrl1 --spiffe-id 'controller/ctrl1'

# Create the controller 2 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl2 --intermediate-name 'Controller Two Signing Cert'

# Create the controller 2 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl2 --dns localhost --ip 127.0.0.1 --server-name ctrl2 --spiffe-id 'controller/ctrl2'

# Create the controller 3 intermediate/signing cert
ziti pki create intermediate --pki-root ./pki --ca-name ca --intermediate-file ctrl3 --intermediate-name 'Controller Three Signing Cert'

# Create the controller 3 server cert
ziti pki create server --pki-root ./pki --ca-name ctrl3 --dns localhost --ip 127.0.0.1 --server-name ctrl3 --spiffe-id 'controller/ctrl3'
