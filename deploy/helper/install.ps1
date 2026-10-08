# Installs haas-helper on Windows from a release: downloads the package for this system, checks it against the release's
# checksums, and checks the checksums' signature when cosign is installed. The releases are private, so it uses the GitHub
# command line (https://cli.github.com), signed in as somebody who may read them. It puts the program in
# %LOCALAPPDATA%\haas-helper and adds that folder to your PATH, which is the only thing it changes about your settings.
#
#   .\install.ps1                      # the newest release
#   .\install.ps1 -Tag v0.0.1-alpha.5
param(
  [string]$Tag = "latest",
  [string]$Repo = $(if ($env:HAAS_REPO) { $env:HAAS_REPO } else { "getlatentic/haas" }),
  [string]$Dir = $(Join-Path $env:LOCALAPPDATA "haas-helper")
)
$ErrorActionPreference = "Stop"

function Fail($message) { Write-Error "install: $message"; exit 1 }
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
  Fail "this needs the GitHub command line, signed in to an account that can read $Repo's releases: https://cli.github.com"
}
$arch = switch ($env:PROCESSOR_ARCHITECTURE) { "AMD64" { "amd64" } "ARM64" { "arm64" } default { Fail "$($env:PROCESSOR_ARCHITECTURE) is a processor no package is built for" } }

if ($Tag -eq "latest") {
  $Tag = (gh release list -R $Repo --limit 1 --json tagName --jq ".[0].tagName")
  if (-not $Tag) { Fail "$Repo has no releases this account can see" }
}
$archive = "haas-helper_${Tag}_windows_${arch}.zip"
$work = Join-Path ([System.IO.Path]::GetTempPath()) ("haas-helper-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $work | Out-Null
try {
  Write-Host "Downloading $archive from $Repo $Tag"
  gh release download $Tag -R $Repo -D $work -p $archive -p SHA256SUMS -p SHA256SUMS.sigstore.json
  if ($LASTEXITCODE -ne 0) { Fail "the release $Tag has no package for windows $arch, or this account cannot read it" }

  $line = Get-Content (Join-Path $work "SHA256SUMS") | Where-Object { $_ -match [regex]::Escape($archive) + '$' } | Select-Object -First 1
  if (-not $line) { Fail "$archive is not in the release's checksums" }
  $wanted = ($line -split '\s+')[0]
  $got = (Get-FileHash (Join-Path $work $archive) -Algorithm SHA256).Hash.ToLower()
  if ($got -ne $wanted) { Fail "the package does not match its checksum: do not use it" }
  Write-Host "The package matches the release's checksum."

  if (Get-Command cosign -ErrorAction SilentlyContinue) {
    cosign verify-blob (Join-Path $work "SHA256SUMS") --bundle (Join-Path $work "SHA256SUMS.sigstore.json") `
      --certificate-identity "https://github.com/$Repo/.github/workflows/release.yml@refs/tags/$Tag" `
      --certificate-oidc-issuer https://token.actions.githubusercontent.com 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { Fail "the checksums' signature does not hold: do not use this package" }
    Write-Host "The checksums are signed by $Repo's release workflow."
  } elseif ($env:HAAS_REQUIRE_COSIGN -eq "1") {
    Fail "cosign is not installed, and HAAS_REQUIRE_COSIGN=1 says not to go on without it"
  } else {
    Write-Host "cosign is not installed, so only the checksum was checked, not who made it."
  }

  Expand-Archive -Path (Join-Path $work $archive) -DestinationPath $work -Force
  New-Item -ItemType Directory -Path $Dir -Force | Out-Null
  Copy-Item (Join-Path $work "haas-helper.exe") (Join-Path $Dir "haas-helper.exe.new") -Force
  Move-Item (Join-Path $Dir "haas-helper.exe.new") (Join-Path $Dir "haas-helper.exe") -Force
  Write-Host ("Installed " + (& (Join-Path $Dir "haas-helper.exe") version) + " in $Dir")

  $path = [Environment]::GetEnvironmentVariable("Path", "User")
  if (($path -split ';') -notcontains $Dir) {
    [Environment]::SetEnvironmentVariable("Path", ($path.TrimEnd(';') + ";" + $Dir), "User")
    Write-Host "$Dir is now on your PATH. Open a new terminal to use it."
  }
  Write-Host "Next: haas-helper login -server https://<your organisation's server>"
} finally {
  Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
