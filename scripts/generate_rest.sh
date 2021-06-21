#!/bin/bash -e

command -v swagger >/dev/null 2>&1 || {
  echo >&2 "Command 'swagger' not installed. See: https://github.com/go-swagger/go-swagger for installation"
  exit 1
}

scriptPath=$(realpath $0)
scriptDir=$(dirname "$scriptPath")

zitiEdgeDir=$(realpath "$scriptDir/..")

clientSourceSpec=$(realpath "$zitiEdgeDir/specs/source/client.yml")
clientSwagSpec=$(realpath "$zitiEdgeDir/specs/client.yml")

managementSourceSpec=$(realpath "$zitiEdgeDir/specs/source/management.yml")
managementSwagSpec=$(realpath "$zitiEdgeDir/specs/management.yml")

copyrightFile=$(realpath "$scriptDir/template.copyright.txt")

echo "...flattening client spec"
swagger flatten "$clientSourceSpec" -o "$clientSwagSpec" --format yaml
echo "...flattening management spec"
swagger flatten "$managementSourceSpec" -o "$managementSwagSpec" --format yaml


oldServerPath=$(realpath "$zitiEdgeDir/rest_server")
echo "...removing any existing server from $oldServerPath"
rm -rf "$oldServerPath"

oldClientPath=$(realpath "$zitiEdgeDir/rest_client")
echo "...removing any existing client from $oldClientPath"
rm -rf "$oldClientPath"

clientServerPath=$(realpath "$zitiEdgeDir/rest_client_api_server")
echo "...removing any existing server from $clientServerPath"
rm -rf "$clientServerPath"
mkdir -p "$clientServerPath"

clientClientPath=$(realpath "$zitiEdgeDir/rest_client_api_client")
echo "...removing any existing client from $clientClientPath"
rm -rf "$clientClientPath"
mkdir -p "$clientClientPath"

managementServerPath=$(realpath "$zitiEdgeDir/rest_management_api_server")
echo "...removing any existing server from $managementServerPath"
rm -rf "$managementServerPath"
mkdir -p "$managementServerPath"

managementClientPath=$(realpath "$zitiEdgeDir/rest_management_api_client")
echo "...removing any existing client from $managementClientPath"
rm -rf "$managementClientPath"
mkdir -p "$managementClientPath"

modelPath=$(realpath "$zitiEdgeDir/rest_model")
echo "...removing any existing model from $modelPath"
rm -rf "$modelPath"
mkdir -p "$modelPath"


echo "...generating client api server"
swagger generate server --exclude-main -f "$clientSwagSpec" -s rest_client_api_server -t "$zitiEdgeDir" -q -r "$copyrightFile" -m "rest_model"
exit_status=$?
if [ ${exit_status} -ne 0 ]; then
  echo "Failed to generate client api server. See above."
  exit "${exit_status}"
fi

echo "...generating client api client"
swagger generate client -f "$clientSwagSpec" -c rest_client_api_client -t "$zitiEdgeDir" -q -r "$copyrightFile" -m "rest_model"
exit_status=$?
if [ ${exit_status} -ne 0 ]; then
  echo "Failed to generate client api client. See above."
  exit "${exit_status}"
fi

echo "...generating management api server"
swagger generate server --exclude-main -f "$managementSwagSpec" -s rest_management_api_server -t "$zitiEdgeDir" -q -r "$copyrightFile" -m "rest_model"
exit_status=$?
if [ ${exit_status} -ne 0 ]; then
  echo "Failed to generate management api server. See above."
  exit "${exit_status}"
fi

echo "...generating management api management"
swagger generate client -f "$managementSwagSpec" -c rest_management_api_client -t "$zitiEdgeDir" -q -r "$copyrightFile" -m "rest_model"
exit_status=$?
if [ ${exit_status} -ne 0 ]; then
  echo "Failed to generate management api client. See above."
  exit "${exit_status}"
fi
