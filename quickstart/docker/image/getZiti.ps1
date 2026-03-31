<#
.SYNOPSIS
Gets ziti from github and adds it to your path

.DESCRIPTION
This script will:
    - detect the requested version of ziti from github, defaulting to latest
    - download the selected version of ziti into the folder of your choice, defaulting to $env:userprofile.ziti\bin)
    - unzip the downloaded file
    - optionally add the extracted path to your path if executed with a "dot" as in: . getLatestZiti.ps1

.PARAMETER Version
Optional ziti release tag, e.g. v2.0.0-pre8. If omitted, latest is used.

.PARAMETER NonInteractive
Skip all prompts, using defaults (install to $env:USERPROFILE\.ziti\bin and add to session PATH).

.INPUTS
None.

.OUTPUTS
None. If "dot sourced" this script will add the resultant directory to your path

.EXAMPLE
PS> . .\getZiti.ps1

.EXAMPLE
PS> . .\getZiti.ps1 -Version v2.0.0-pre8
#>
param(
    [string]$Version,
    [switch]$NonInteractive
)

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
  $matchFilter="ziti-darwin-$arch"
} elseif($osDescription.ToLower() -match "linux") {
  $matchFilter="ziti-linux-$arch"
} else {
  Write-Error "An error occurred. os not detected from osDescription: $osDescription"
  return
}
$dirSeparator = [System.IO.Path]::DirectorySeparatorChar
$pathSeparator = [System.IO.Path]::PathSeparator

if([string]::IsNullOrWhiteSpace($Version)) {
    $releaseFromGitHub = irm https://api.github.com/repos/openziti/ziti/releases/latest
} else {
    $releaseFromGitHub = irm "https://api.github.com/repos/openziti/ziti/releases/tags/$Version"
}

$version=($releaseFromGitHub.tag_name)
$zitidl=($releaseFromGitHub).assets | where {$_.browser_download_url -Match "$matchFilter.*"}
$downloadUrl=($zitidl.browser_download_url)
$name=$zitidl.name
$checksumAsset=($releaseFromGitHub).assets | where {$_.name -eq "checksums.sha256.txt"}
$checksumUrl=($checksumAsset.browser_download_url)

if([string]::IsNullOrWhiteSpace($downloadUrl)) {
    Write-Error "No matching asset found for version '$version' using filter '$matchFilter'"
    return
}

$homeDirectory = [System.Environment]::GetFolderPath([System.Environment+SpecialFolder]::UserProfile)
$defaultFolder="$homeDirectory${dirSeparator}.ziti${dirSeparator}bin"
if($NonInteractive) {
    $toDir=$defaultFolder
} else {
    $toDir=$(Read-Host "Where should ziti be installed? [default: ${defaultfolder}]")
    if($toDir.Trim() -eq "") {
        $toDir=("${defaultfolder}")
    }
}

$zipFile="${toDir}${dirSeparator}${name}"
if($(Test-Path -Path $zipFile -PathType Leaf)) {
    Write-Output "The distribution has already been downloaded to $zipFile. Not downloading again"
} else {
    New-Item -Force -ItemType Directory -Path "${toDir}"
    Write-Output "Downloading file "
    Write-Output "    from: ${downloadUrl} "
    Write-Output "      to: ${zipFile}"
    $SavedProgressPreference=$ProgressPreference
    $ProgressPreference='SilentlyContinue'
    iwr ${downloadUrl} -OutFile "$zipFile"
    $ProgressPreference=$SavedProgressPreference
}

if(-not [string]::IsNullOrWhiteSpace($checksumUrl)) {
    Write-Output "Verifying checksum..."
    $checksumContent = (irm $checksumUrl)
    $expectedLine = $checksumContent -split "`n" | where { $_ -match [regex]::Escape($name) }
    if($expectedLine) {
        $expectedHash = ($expectedLine -split "\s+")[0].ToUpper()
        if($osDescription.ToLower() -match "windows") {
            $actualHash = (Get-FileHash -Algorithm SHA256 -Path $zipFile).Hash.ToUpper()
        } else {
            $actualHash = (sha256sum $zipFile -split "\s+")[0].ToUpper()
        }
        if($actualHash -ne $expectedHash) {
            Write-Error "Checksum mismatch for $name! Expected $expectedHash but got $actualHash. Aborting."
            Remove-Item -Force $zipFile
            return
        }
        Write-Output "Checksum verified."
    } else {
        Write-Warning "Could not find checksum entry for '$name' in checksums.sha256.txt. Proceeding without verification."
    }
} else {
    Write-Warning "No checksums.sha256.txt asset found in release. Proceeding without verification."
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
if($NonInteractive) {
    $addToPath="Y"
} else {
    $addToPath=$(Read-Host "Would you like to add ziti to this session's path? [default: Y]")
    if($addToPath.Trim() -eq "") {
        $addToPath=("Y")
    }
}

if($addToPath -ilike "y*") {
  $env:PATH+="$pathSeparator${toDir}${dirSeparator}${version}"
  Write-Output "ziti added to your path!"
}