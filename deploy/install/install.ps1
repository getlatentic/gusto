# Installs haas and haas-helper on Windows from a release: downloads each package for this system, checks it against the
# release's checksums, and checks the checksums' signature when cosign is installed. It downloads with the GitHub command line
# when that is signed in, which a private release needs, and otherwise straight from the release's public address. It puts the
# programs in %LOCALAPPDATA%\haas and adds that folder to your PATH, which is the only thing it changes about your settings.
#
#   .\install.ps1                      # the newest release
#   .\install.ps1 -Tag v0.0.1-alpha.5
#   .\install.ps1 -Programs haas-helper
#   .\install.ps1 -NoStart              # install, and do not start haas for this person
#
# Once haas is installed it is started for the person running this, at logon and now, and opened in their browser, signed in.
param(
  [string]$Tag = "latest",
  [string]$Repo = $(if ($env:HAAS_REPO) { $env:HAAS_REPO } else { "getlatentic/haas" }),
  [string]$Dir = $(Join-Path $env:LOCALAPPDATA "haas"),
  [string[]]$Programs = @("haas", "haas-helper"),
  [switch]$NoStart = ($env:HAAS_NO_START -eq "1")
)
$ErrorActionPreference = "Stop"

function Fail($message) { Write-Error "install: $message"; exit 1 }
$arch = switch ($env:PROCESSOR_ARCHITECTURE) { "AMD64" { "amd64" } "ARM64" { "arm64" } default { Fail "$($env:PROCESSOR_ARCHITECTURE) is a processor no package is built for" } }

# The GitHub command line reads a private release; a public one is read with no account.
$viaGh = $false
if (Get-Command gh -ErrorAction SilentlyContinue) {
  gh auth status 2>$null | Out-Null
  $viaGh = ($LASTEXITCODE -eq 0)
}

function Fetch($name, $into) {
  if ($viaGh) {
    gh release download $Tag -R $Repo -D $into -p $name
    if ($LASTEXITCODE -ne 0) { throw "gh could not download $name" }
  } else {
    Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/$Repo/releases/download/$Tag/$name" -OutFile (Join-Path $into $name)
  }
}

if ($Tag -eq "latest") {
  if ($viaGh) {
    $Tag = (gh release list -R $Repo --limit 1 --json tagName --jq ".[0].tagName")
  } else {
    $Tag = (Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=1")[0].tag_name
  }
  if (-not $Tag) { Fail "$Repo has no release this machine can see: a private one needs the GitHub command line, signed in (https://cli.github.com)" }
}

$work = Join-Path ([System.IO.Path]::GetTempPath()) ("haas-install-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $work | Out-Null
try {
  try { Fetch "SHA256SUMS" $work; Fetch "SHA256SUMS.sigstore.json" $work } catch { Fail "the release $Tag cannot be read from here" }

  if (Get-Command cosign -ErrorAction SilentlyContinue) {
    cosign verify-blob (Join-Path $work "SHA256SUMS") --bundle (Join-Path $work "SHA256SUMS.sigstore.json") `
      --certificate-identity "https://github.com/$Repo/.github/workflows/release.yml@refs/tags/$Tag" `
      --certificate-oidc-issuer https://token.actions.githubusercontent.com 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) { Fail "the checksums' signature does not hold: do not use these packages" }
    Write-Host "The checksums are signed by $Repo's release workflow."
  } elseif ($env:HAAS_REQUIRE_COSIGN -eq "1") {
    Fail "cosign is not installed, and HAAS_REQUIRE_COSIGN=1 says not to go on without it"
  } else {
    Write-Host "cosign is not installed, so the packages are checked against the checksums, and not who made them."
  }

  New-Item -ItemType Directory -Path $Dir -Force | Out-Null
  foreach ($program in $Programs) {
    $archive = "${program}_${Tag}_windows_${arch}.zip"
    Write-Host "Downloading $archive from $Repo $Tag"
    try { Fetch $archive $work } catch { Fail "the release $Tag has no $program for windows $arch" }
    $line = Get-Content (Join-Path $work "SHA256SUMS") | Where-Object { $_ -match [regex]::Escape($archive) + '$' } | Select-Object -First 1
    if (-not $line) { Fail "$archive is not in the release's checksums" }
    $wanted = ($line -split '\s+')[0]
    $got = (Get-FileHash (Join-Path $work $archive) -Algorithm SHA256).Hash.ToLower()
    if ($got -ne $wanted) { Fail "$archive does not match its checksum: do not use it" }
    Expand-Archive -Path (Join-Path $work $archive) -DestinationPath $work -Force
    Copy-Item (Join-Path $work "$program.exe") (Join-Path $Dir "$program.exe.new") -Force
    Move-Item (Join-Path $Dir "$program.exe.new") (Join-Path $Dir "$program.exe") -Force
    Write-Host ("Installed " + (& (Join-Path $Dir "$program.exe") version) + " in $Dir")
  }

  $path = [Environment]::GetEnvironmentVariable("Path", "User")
  if (($path -split ';') -notcontains $Dir) {
    [Environment]::SetEnvironmentVariable("Path", ($path.TrimEnd(';') + ";" + $Dir), "User")
    Write-Host "$Dir is now on your PATH. Open a new terminal to use it."
  }
  if (($Programs -contains "haas") -and -not $NoStart) {
    & (Join-Path $Dir "haas.exe") local install
  }
} finally {
  Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
