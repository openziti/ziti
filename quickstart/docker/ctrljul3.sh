
sudo rm -rf ~/docker-volume/myFirstZitiNetwork
mkdir -p ~/docker-volume/myFirstZitiNetwork/pki
echo "#ziti.env file" > ~/docker-volume/myFirstZitiNetwork/ziti.env
chmod -R 706 ~/docker-volume/myFirstZitiNetwork

docker run --rm \
  -it \
  --name ziti-edge-controller \
  --network myFirstZitiNetwork \
  --network-alias ziti-edge-controller \
  -v ~/docker-volume/myFirstZitiNetwork/pki:/persistent/pki \
  -v ~/docker-volume/myFirstZitiNetwork/ziti.env:/persistent/ziti.env \
  -p 1280:1280 \
  -e ZITI_EDGE_CONTROLLER_IP_OVERRIDE="10.20.40.167" \
  -e ZITI_USER=admin \
  -e ZITI_PWD=admin \
  -e WEB_UID=1005 \
  openziti/quickstart:latest \
  bash
