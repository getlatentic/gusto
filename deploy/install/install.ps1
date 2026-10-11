# Installs haas and haas-helper on Windows from a release: downloads each package for this system, checks it against the
# release's checksums, and checks the checksums' signature when cosign is installed. It downloads with the GitHub command line
# when that is signed in, which a private release needs, and otherwise straight from the release's public address. It puts the
# programs in %LOCALAPPDATA%\haas and adds that folder to your PATH, which is the only thing it changes about your settings.
#
#   powershell -ExecutionPolicy ByPass -c "irm https://github.com/getlatentic/haas/releases/latest/download/install.ps1 | iex"
#   .\install.ps1                      # the newest release
#   .\install.ps1 -Tag v0.0.1-alpha.5
#   .\install.ps1 -Programs haas-helper
#   .\install.ps1 -NoStart              # install, and do not start haas for this person
#   $env:HAAS_RELEASE_URL = "http://10.0.2.2:8000"; .\install.ps1 -Tag v0.0.0-test   # a release's files from another address
#
# Piped to iex it takes no arguments, so HAAS_TAG, HAAS_PROGRAMS (names separated by spaces) and HAAS_NO_START say the same.
#
# Once haas is installed it is started for the person running this, at logon and now, and opened in their browser, signed in.
param(
  [string]$Tag = $(if ($env:HAAS_TAG) { $env:HAAS_TAG } else { "latest" }),
  [string]$Repo = $(if ($env:HAAS_REPO) { $env:HAAS_REPO } else { "getlatentic/haas" }),
  [string]$Dir = $(Join-Path $env:LOCALAPPDATA "haas"),
  [string[]]$Programs = $(if ($env:HAAS_PROGRAMS) { $env:HAAS_PROGRAMS -split '\s+' } else { @("haas", "haas-helper") }),
  [switch]$NoStart = ($env:HAAS_NO_START -eq "1")
)
$ErrorActionPreference = "Stop"

# Fail throws rather than exits, since a script piped to iex runs in the person's own PowerShell, and exit would close it.
function Fail($message) { throw "install: $message" }
$arch = switch ($env:PROCESSOR_ARCHITECTURE) { "AMD64" { "amd64" } "ARM64" { "arm64" } default { Fail "$($env:PROCESSOR_ARCHITECTURE) is a processor no package is built for" } }

# The GitHub command line reads a private release; a public one is read with no account. HAAS_RELEASE_URL is a folder with a
# release's files in it, such as a test build served to a machine that cannot reach GitHub.
$from = $env:HAAS_RELEASE_URL
$viaGh = $false
if (-not $from -and (Get-Command gh -ErrorAction SilentlyContinue)) {
  gh auth status 2>$null | Out-Null
  $viaGh = ($LASTEXITCODE -eq 0)
}

function Fetch($name, $into) {
  if ($from) {
    Invoke-WebRequest -UseBasicParsing -Uri ($from.TrimEnd("/") + "/$name") -OutFile (Join-Path $into $name)
  } elseif ($viaGh) {
    gh release download $Tag -R $Repo -D $into -p $name
    if ($LASTEXITCODE -ne 0) { throw "gh could not download $name" }
  } else {
    Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/$Repo/releases/download/$Tag/$name" -OutFile (Join-Path $into $name)
  }
}

if ($Tag -eq "latest") {
  if ($from) { Fail "HAAS_RELEASE_URL needs the release's tag, as -Tag or HAAS_TAG" }
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
  try { Fetch "SHA256SUMS" $work } catch { Fail "the release $Tag cannot be read from here" }

  if (Get-Command cosign -ErrorAction SilentlyContinue) {
    try { Fetch "SHA256SUMS.sigstore.json" $work } catch { Fail "the release $Tag has no signature for its checksums" }
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
    $target = Join-Path $Dir "$program.exe"
    Copy-Item (Join-Path $work "$program.exe") "$target.new" -Force
    # Windows does not let a program that is running be replaced, and does let it be renamed, so the one running is moved aside
    # and removed once nothing runs it.
    Get-ChildItem $Dir -Filter "$program.exe.old-*" -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue
    if (Test-Path $target) { Move-Item $target "$target.old-$([guid]::NewGuid().ToString('N'))" -Force }
    Move-Item "$target.new" $target -Force
    Write-Host ("Installed " + (& (Join-Path $Dir "$program.exe") version) + " in $Dir")
  }

  $path = [Environment]::GetEnvironmentVariable("Path", "User")
  if (($path -split ';') -notcontains $Dir) {
    [Environment]::SetEnvironmentVariable("Path", ($path.TrimEnd(';') + ";" + $Dir), "User")
    Write-Host "$Dir is now on your PATH. Open a new terminal to use it."
  }
  if (($Programs -contains "haas") -and -not $NoStart) {
    & (Join-Path $Dir "haas.exe") local install
    if ($LASTEXITCODE -ne 0) { Fail "haas is installed, and could not be started: run haas local install to see why" }
  }
} finally {
  Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
