#!/bin/bash -e

GO_SWAGGER_VERSION="v0.33.1"
GO_SWAGGER_HASH="2af7725271cf99ace5d44ab134acb53bffcc5734"
if ! command -v swagger &>/dev/null \
|| [[ "$(swagger version | awk '$1~/^version:/{print $2}')" != "${GO_SWAGGER_VERSION}" \
|| "$(swagger version | awk '$1~/^commit:/{print $2}')" != "${GO_SWAGGER_HASH}" ]]
then
  echo >&2 "Go Swagger executable 'swagger' ${GO_SWAGGER_VERSION} (${GO_SWAGGER_HASH}) is required. Download the binary from GitHub: https://github.com/go-swagger/go-swagger/releases/tag/v0.33.1"
  exit 1
fi

scriptPath=$(realpath $0)
scriptDir=$(dirname "$scriptPath")

zitiEdgeDir=$(realpath "$scriptDir/..")
swagSpec=$(realpath "$zitiEdgeDir/specs/swagger.yml")
copyrightFile=$(realpath "$scriptDir/template.copyright.txt")

serverPath=$(realpath "$zitiEdgeDir/rest_server")
echo "...removing any existing server from $serverPath"
rm -rf "$serverPath"
mkdir -p "$serverPath"

clientPath=$(realpath "$zitiEdgeDir/rest_client")
echo "...removing any existing client from $clientPath"
rm -rf "$clientPath"
mkdir -p "$clientPath"

modelPath=$(realpath "$zitiEdgeDir/rest_model")
echo "...removing any existing model from $modelPath"
rm -rf "$modelPath"
mkdir -p "$modelPath"

echo "...generating server"
swagger generate server --exclude-main -f "$swagSpec" -s rest_server -t "$zitiEdgeDir" -q -r "$copyrightFile" -m "rest_model"
exit_status=$?
if [ ${exit_status} -ne 0 ]; then
  echo "Failed to generate server. See above."
  exit "${exit_status}"
fi

echo "...generating client"
swagger generate client -f "$swagSpec" -c rest_client -t "$zitiEdgeDir" -q -r "$copyrightFile" -m "rest_model"
exit_status=$?
if [ ${exit_status} -ne 0 ]; then
  echo "Failed to generate client. See above."
  exit "${exit_status}"
fi
