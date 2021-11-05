$PSDefaultParameterValues += @{ 'New-RegKey:ErrorAction' = 'Stop' }
try
{
    Get-Command swagger -ErrorAction "SilentlyContinue" | Out-Null
    if (-not$?)
    {
        throw "Command 'swagger' not installed. See: https://github.com/go-swagger/go-swagger for installation"
    }

    $zitiEdgeDir = Join-Path $PSScriptRoot "../" -Resolve

    $copyrightFile = Join-Path $PSScriptRoot "template.copyright.txt" -Resolve

    $swagSpec = Join-Path $zitiEdgeDir "/specs/swagger.yml" -Resolve
    "...reading spec from $swagSpec"

    $serverPath = Join-Path $zitiEdgeDir "/rest_server"
    "...removing any existing server from $serverPath"
    Remove-Item $serverPath -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null
    New-Item -ItemType "directory" -Path $serverPath -ErrorAction "SilentlyContinue" | Out-Null

    $clientPath = Join-Path $zitiEdgeDir "/rest_client"
    "...removing any existing client from $clientPath"
    Remove-Item $clientPath -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null
    New-Item -ItemType "directory" -Path $clientPath -ErrorAction "SilentlyContinue" | Out-Null

    $modelPath = Join-Path $zitiEdgeDir "/rest_model"
    "...removing any existing model from $modelPath"
    Remove-Item $modelPath -Recurse -Force -ErrorAction "SilentlyContinue" | Out-Null
    New-Item -ItemType "directory" -Path $modelPath -ErrorAction "SilentlyContinue" | Out-Null


    "...generating server"
    swagger generate server --exclude-main -f $swagSpec -s rest_server -t $zitiEdgeDir -q -r $copyrightFile -m "rest_model"
    if (-not$?)
    {
        throw "Failed to generate server. See above."
    }
    "...generating client"
    swagger generate client -f $swagSpec  -c rest_client -t $zitiEdgeDir -q -r $copyrightFile -m "rest_model"
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
    $configureFile = Join-Path $serverPath "/configure_ziti_edge.go" -Resolve

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