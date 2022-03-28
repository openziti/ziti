$PSDefaultParameterValues += @{ 'New-RegKey:ErrorAction' = 'Stop' }

$orignalLocaltion = Get-Location
try
{

    Get-Command swagger -ErrorAction "SilentlyContinue" | Out-Null
    if (-not$?)
    {
        throw "Command 'swagger' not installed. See: https://github.com/go-swagger/go-swagger for installation"
    }

    $zitiEdgeDir = Join-Path $PSScriptRoot "../" -Resolve
    $zitiSpecSrcDir = Join-Path $PSScriptRoot "../specs/source" -Resolve

    $copyrightFile = Join-Path $PSScriptRoot "template.copyright.txt" -Resolve

    "...generating Open API 2.0 specs from source"
    Push-Location $zitiSpecSrcDir
    swagger flatten .\client.yml -o ..\client.yml --format yaml
    if (-not$?)
    {
        Pop-Location
        throw "Failed to flatten client.yml. See above."
    }

    swagger flatten .\management.yml -o ..\management.yml --format yaml
    if (-not$?)
    {
        Pop-Location
        throw "Failed to flatten management.yml. See above."
    }

    $clientSpec = Join-Path $zitiEdgeDir "/specs/client.yml" -Resolve
    $managementSpec = Join-Path $zitiEdgeDir "/specs/management.yml" -Resolve


    $oldServerPath = Join-Path $zitiEdgeDir "/rest_server"
    $oldClientPath = Join-Path $zitiEdgeDir "/rest_client"

    "...removing old server path: $oldServerPath"
    Remove-Item $oldServerPath -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null

    "...removing old client path: $oldClientPath"
    Remove-Item $oldClientPath -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null

    $clientServerOutDir = Join-Path $zitiEdgeDir "/rest_client_api_server"
    "...removing any existing server from $clientServerOutDir"
    Remove-Item $clientServerOutDir -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null
    New-Item -ItemType "directory" -Path $clientServerOutDir -ErrorAction "SilentlyContinue" | Out-Null

    $clientClientOutDir = Join-Path $zitiEdgeDir "/rest_client_api_client"
    "...removing any existing client from $clientClientOutDir"
    Remove-Item $clientClientOutDir -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null
    New-Item -ItemType "directory" -Path $clientClientOutDir -ErrorAction "SilentlyContinue" | Out-Null

    $managementServerOutDir = Join-Path $zitiEdgeDir "/rest_management_api_server"
    "...removing any existing server from $managementServerOutDir"
    Remove-Item $managementServerOutDir -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null
    New-Item -ItemType "directory" -Path $managementServerOutDir -ErrorAction "SilentlyContinue" | Out-Null

    $managementClientOutDir = Join-Path $zitiEdgeDir "/rest_management_api_client"
    "...removing any existing client from $managementClientOutDir"
    Remove-Item $managementClientOutDir -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null
    New-Item -ItemType "directory" -Path $managementClientOutDir -ErrorAction "SilentlyContinue" | Out-Null

    $modelPath = Join-Path $zitiEdgeDir "/rest_model"
    "...removing any existing model from $modelPath"
    Remove-Item $modelPath -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null
    New-Item -ItemType "directory" -Path $modelPath -ErrorAction "SilentlyContinue" | Out-Null

    "...generating Client API server"
    swagger generate server --exclude-main --additional-initialism=jwt -f $clientSpec -s rest_client_api_server -t $zitiEdgeDir -q -r $copyrightFile -m "rest_model"
    if (-not$?)
    {
        throw "Failed to generate server. See above."
    }

    "...generating Client API client"
    swagger generate client -f $clientSpec --additional-initialism=jwt -c rest_client_api_client -t $zitiEdgeDir -q -r $copyrightFile -m "rest_model"
    if (-not$?)
    {
        throw "Failed to generate client. See above."
    }

    "...generating Management API server"
    swagger generate server --exclude-main --additional-initialism=jwt -f $managementSpec -s rest_management_api_server -t $zitiEdgeDir -q -r $copyrightFile -m "rest_model"
    if (-not$?)
    {
        throw "Failed to generate server. See above."
    }

    "...generating Management API client"
    swagger generate client -f $managementSpec --additional-initialism=jwt -c rest_management_api_client -t $zitiEdgeDir -q -r $copyrightFile -m "rest_model"
    if (-not$?)
    {
        throw "Failed to generate client. See above."
    }

    "...fixing up windows slashes"
    # This one file has the command used to generate the server in it w/ paths. The path sep is OS specific.
    # On Windows this causes this one file to show changes depending on the OS that generated it. This
    # works around those changes from showing up in commits by switching to forward slashes.
    #
    # There appears to be no option to suppress this line in the `swagger` executable.
    $configureFile = Join-Path $managementServerOutDir "/configure_ziti_edge_management.go" -Resolve

    $content = ""
    foreach ($line in Get-Content $configureFile)
    {
        if ($line -match "^//go:generate swagger generate server")
        {
            $line = $line -replace "\\", "/"
        }

        $content = $content + $line + "`n"
    }

    $content | Set-Content $configureFile -nonewline

    $configureFile = Join-Path $clientServerOutDir "/configure_ziti_edge_client.go" -Resolve

    $content = ""
    foreach ($line in Get-Content $configureFile)
    {
        if ($line -match "^//go:generate swagger generate server")
        {
            $line = $line -replace "\\", "/"
        }

        $content = $content + $line + "`n"
    }

    $content | Set-Content $configureFile -nonewline

}
catch
{
    [Console]::Error.WriteLine($Error[0])
}

Set-Location $orignalLocaltion