<#
.SYNOPSIS
Gets the latest ziti from github and adds it to your path

.DESCRIPTION
This script will:
    - detect the lastest version of ziti
    - download the latest versio of ziti into the folder of your choice, defaulting to $env:userprofile.ziti\bin)
    - unzip the downloaded file
    - optionally add the extracted path to your path if executed with a "dot" as in: . getLatestZiti.ps1

.INPUTS
None.

.OUTPUTS
None. If "dot sourced" this script will add the resultant directory to your path

.EXAMPLE
PS> . .\getZiti.ps1
#>
Add-Type -AssemblyName System.Runtime.InteropServices

$osDescription = [System.Runtime.InteropServices.RuntimeInformation]::OSDescription
$osArchitecture = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
$frameworkDescription = [System.Runtime.InteropServices.RuntimeInformation]::FrameworkDescription

$arch=$osArchitecture.ToString().ToLower()
if($arch -match "x64") {
  $arch = "amd64"
}

if($osDescription.ToLower() -match "windows") {
  $matchFilter="ziti-windows-$arch"
} elseif($osDescription.ToLower() -match "darwin") {
  $matchFilter="ziti-darwin-$arch"
} elseif($osDescription.ToLower() -match "linux") {
  $matchFilter="ziti-linux-$arch"
} else {
  Write-Error "An error occurred. os not detected from osDescription: $osDescription"
  return
}
$pathSeparator = [System.IO.Path]::DirectorySeparatorChar
$latestFromGitHub=(irm https://api.github.com/repos/openziti/ziti/releases/latest)
$version=($latestFromGitHub.tag_name)
$zitidl=($latestFromGitHub).assets | where {$_.browser_download_url -Match "$matchFilter.*zip"}
$downloadUrl=($zitidl.browser_download_url)
$name=$zitidl.name
$homeDirectory = [System.Environment]::GetFolderPath([System.Environment+SpecialFolder]::UserProfile)
$defaultFolder="$homeDirectory${pathSeparator}.ziti${pathSeparator}bin"
$toDir=$(Read-Host "Where should ziti be installed? [default: ${defaultfolder}]")
if($toDir.Trim() -eq "") {
    $toDir=("${defaultfolder}")
}

$zipFile="${toDir}${pathSeparator}${name}"
if($(Test-Path -Path $zipFile -PathType Leaf)) {
    Write-Output "The file has already been downloading. No need to download again"
} else {
    mkdir "${toDir}" -ErrorAction SilentlyContinue
    Write-Output "Downloading file "
    Write-Output "    from: ${downloadUrl} "
    Write-Output "      to: ${zipFile}"
    $SavedProgressPreference=$ProgressPreference
    $ProgressPreference='SilentlyContinue'
    iwr ${downloadUrl} -OutFile "$zipFile"
    $ProgressPreference=$SavedProgressPreference
}

Expand-Archive -Path $zipFile -DestinationPath "${toDir}${pathSeparator}${version}" -ErrorAction SilentlyContinue

Write-Output " "
Write-Output "Extracted binaries to ${toDir}${pathSeparator}${version}${pathSeparator}ziti"
Write-Output " "
$addToPath=$(Read-Host "Would you like to add ziti to this session's path? [default: Y]")
if($addToPath.Trim() -eq "") {
    $addToPath=("Y")
}

if($addToPath -ilike "y*") {
  $env:Path+=";${toDir}${pathSeparator}${version}"
  Write-Output "ziti added to your path!"
}