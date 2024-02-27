<#
.SYNOPSIS
Gets the latest ziti from github and adds it to your path

.DESCRIPTION
This script will:
    - detect the latest version of ziti
    - download the latest version of ziti into the folder of your choice, defaulting to $env:userprofile.ziti\bin)
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
$frameworkDescription = [System.Runtime.InteropServices.RuntimeInformation]::FrameworkDescription

$arch=${env:PROCESSOR_ARCHITECTURE}.ToString().ToLower()
if($arch -match "x64") {
  $arch = "amd64"
}

if($osDescription.ToLower() -match "windows") {
  $matchFilter="ziti-windows-$arch"
} elseif($osDescription.ToLower() -match "darwin") {
  $matchFilter="ziti-darwin-amd64"
  #todo: replace $arch some day
} elseif($osDescription.ToLower() -match "linux") {
  $matchFilter="ziti-linux-$arch"
} else {
  Write-Error "An error occurred. os not detected from osDescription: $osDescription"
  return
}
$dirSeparator = [System.IO.Path]::DirectorySeparatorChar
$pathSeparator = [System.IO.Path]::PathSeparator
$latestFromGitHub=(irm https://api.github.com/repos/openziti/ziti/releases/latest)
$version=($latestFromGitHub.tag_name)
$zitidl=($latestFromGitHub).assets | where {$_.browser_download_url -Match "$matchFilter.*"}
$downloadUrl=($zitidl.browser_download_url)
$name=$zitidl.name
$homeDirectory = [System.Environment]::GetFolderPath([System.Environment+SpecialFolder]::UserProfile)
$defaultFolder="$homeDirectory${dirSeparator}.ziti${dirSeparator}bin"
$toDir=$(Read-Host "Where should ziti be installed? [default: ${defaultfolder}]")
if($toDir.Trim() -eq "") {
    $toDir=("${defaultfolder}")
}

$zipFile="${toDir}${dirSeparator}${name}"
if($(Test-Path -Path $zipFile -PathType Leaf)) {
    Write-Output "The file has already been downloading. No need to download again"
} else {
    mkdir -p "${toDir}"
    Write-Output "Downloading file "
    Write-Output "    from: ${downloadUrl} "
    Write-Output "      to: ${zipFile}"
    $SavedProgressPreference=$ProgressPreference
    $ProgressPreference='SilentlyContinue'
    iwr ${downloadUrl} -OutFile "$zipFile"
    $ProgressPreference=$SavedProgressPreference
}

if($osDescription.ToLower() -match "windows") {
    Expand-Archive -Path $zipFile -DestinationPath "${toDir}${dirSeparator}${version}" -ErrorAction SilentlyContinue
} else {
    $env:LC_ALL = "en_US.UTF-8"
    mkdir -p "${toDir}${dirSeparator}${version}"
    tar -xvf $zipFile -C "${toDir}${dirSeparator}${version}"
}

Write-Output " "
Write-Output "Extracted binaries to ${toDir}${dirSeparator}${version}${dirSeparator}ziti"
Write-Output " "
$addToPath=$(Read-Host "Would you like to add ziti to this session's path? [default: Y]")
if($addToPath.Trim() -eq "") {
    $addToPath=("Y")
}

if($addToPath -ilike "y*") {
  $env:PATH+="$pathSeparator${toDir}${dirSeparator}${version}"
  Write-Output "ziti added to your path!"
}
