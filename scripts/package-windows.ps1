# package-windows.ps1 — Build the Windows release folder and zip archive.
#
# Bundles svpchain-gui.exe and svpchain-mcp.exe into one folder so the GUI
# can auto-detect the MCP signer binary path.
#
# Usage:
#   .\scripts\package-windows.ps1
#   $env:VERSION = "1.0.0"; .\scripts\package-windows.ps1
#
# Output:
#   build\SVPChain Agent\svpchain-gui.exe
#   build\SVPChain Agent\svpchain-mcp.exe
#   build\svpchain-agent-<version>-windows-amd64.zip

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

$AppDirName = "SVPChain Agent"
$ReleaseStem = "svpchain-agent"
$BuildDir = if ($env:BUILDDIR) { $env:BUILDDIR } else { Join-Path $Root "build" }
$AppDir = Join-Path $BuildDir $AppDirName

function Resolve-Version {
	if ($env:VERSION) {
		return ($env:VERSION -replace '^v', '')
	}
	try {
		$tag = git describe --tags --exact-match 2>$null
		if ($tag) { return ($tag -replace '^v', '') }
	} catch {}
	return "0.1.0-dev"
}

$Version = Resolve-Version
$ZipPath = Join-Path $BuildDir "$ReleaseStem-$Version-windows-amd64.zip"

Write-Host "==> Packaging $AppDirName $Version"

if (Test-Path $BuildDir) {
	Remove-Item -Recurse -Force $BuildDir
}
New-Item -ItemType Directory -Force -Path $BuildDir | Out-Null

Write-Host "==> Locating wails CLI"
$Wails = $env:WAILS
if (-not $Wails) {
	$WailsCmd = Get-Command wails -ErrorAction SilentlyContinue
	if ($WailsCmd) {
		$Wails = $WailsCmd.Source
	} else {
		$GoPath = go env GOPATH
		$Candidate = Join-Path $GoPath "bin\wails.exe"
		if (Test-Path $Candidate) {
			$Wails = $Candidate
		} else {
			throw "wails CLI not found; install with: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
		}
	}
}

Write-Host "==> Building MCP signer binary (CGO enabled)"
$env:CGO_ENABLED = "1"
go build -mod=readonly -trimpath -o (Join-Path $BuildDir "svpchain-mcp.exe") ./cmd/svpchain-mcp

Write-Host "==> Syncing Wails app icon"
$WailsBuild = Join-Path $Root "cmd\svpchain-gui\build"
New-Item -ItemType Directory -Force -Path $WailsBuild | Out-Null
Copy-Item (Join-Path $Root "packaging\logo-svp1.png") (Join-Path $WailsBuild "appicon.png") -Force
foreach ($dir in @("windows", "darwin")) {
	$target = Join-Path $WailsBuild $dir
	if (Test-Path $target) {
		Remove-Item -Recurse -Force $target
	}
}

Write-Host "==> Building GUI with wails (frontend + bindings + binary)"
$GuiLdflags = "-X github.com/svpchain/svpchain-agent/internal/desktop.Version=$Version"
Push-Location (Join-Path $Root "cmd\svpchain-gui")
Write-Host "Build started, please wait..."
& $Wails build -clean -trimpath -ldflags $GuiLdflags
Pop-Location

Write-Host "==> Assembling release folder"
New-Item -ItemType Directory -Force -Path $AppDir | Out-Null
Copy-Item (Join-Path $Root "cmd\svpchain-gui\build\bin\svpchain-gui.exe") $AppDir
Copy-Item (Join-Path $BuildDir "svpchain-mcp.exe") $AppDir
Copy-Item (Join-Path $Root "packaging\windows\READ-BEFORE-RUN.txt") $AppDir
Copy-Item (Join-Path $Root "packaging\windows\运行前先阅读.txt") $AppDir

Write-Host "==> Creating zip archive"
if (Test-Path $ZipPath) {
	Remove-Item -Force $ZipPath
}
Compress-Archive -Path $AppDir -DestinationPath $ZipPath

Write-Host ""
Write-Host "Done."
Write-Host "  App: $AppDir"
Write-Host "  Zip: $ZipPath"
Write-Host ""
Write-Host "APP_ZIP=$ZipPath"
Write-Host "APP_WINDOWS_DIR=$AppDir"
