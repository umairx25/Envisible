# Envis installer for Windows (PowerShell).
#
# Usage:
#   irm https://raw.githubusercontent.com/umairx25/Envisible/main/scripts/install.ps1 | iex
#
# Environment overrides:
#   $env:ENVIS_VERSION      install a specific tag (e.g. v1.0.0); default: latest
#   $env:ENVIS_INSTALL_DIR  target dir; default: $HOME\.envis\bin

$ErrorActionPreference = "Stop"
$Repo = "umairx25/Envisible"
$InstallDir = if ($env:ENVIS_INSTALL_DIR) { $env:ENVIS_INSTALL_DIR } else { Join-Path $HOME ".envis\bin" }

function Fail($msg) { Write-Error $msg; exit 1 }

# --- detect architecture ---------------------------------------------------
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
  "AMD64" { "amd64" }
  "ARM64" { "arm64" }
  default { Fail "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

# --- resolve version -------------------------------------------------------
$version = $env:ENVIS_VERSION
if (-not $version) {
  Write-Host "Resolving latest release..."
  $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
  $version = $rel.tag_name
  if (-not $version) { Fail "could not determine latest version (set `$env:ENVIS_VERSION)" }
}

$name = "envis-windows-$arch"
$archive = "$name.zip"
$base = "https://github.com/$Repo/releases/download/$version"

Write-Host "Installing envis $version for windows/$arch..."

$tmp = Join-Path $env:TEMP ("envis-" + [System.Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp -Force | Out-Null
try {
  $zipPath = Join-Path $tmp $archive
  Invoke-WebRequest -Uri "$base/$archive" -OutFile $zipPath

  # --- verify checksum (best-effort) --------------------------------------
  try {
    $sumsPath = Join-Path $tmp "SHA256SUMS"
    Invoke-WebRequest -Uri "$base/SHA256SUMS" -OutFile $sumsPath
    $expected = (Select-String -Path $sumsPath -Pattern ([regex]::Escape($archive)) `
      | Select-Object -First 1).Line.Split(" ")[0]
    if ($expected) {
      $actual = (Get-FileHash -Algorithm SHA256 $zipPath).Hash.ToLower()
      if ($actual -ne $expected.ToLower()) {
        Fail "checksum mismatch for $archive (expected $expected, got $actual)"
      }
      Write-Host "Checksum verified."
    }
  } catch {
    Write-Host "Warning: could not verify checksum; continuing."
  }

  # --- extract and install ------------------------------------------------
  Expand-Archive -Path $zipPath -DestinationPath $tmp -Force
  $exe = Join-Path $tmp "envis.exe"
  if (-not (Test-Path $exe)) { Fail "archive did not contain envis.exe" }

  New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
  Copy-Item $exe (Join-Path $InstallDir "envis.exe") -Force
  Write-Host "Installed envis to $InstallDir\envis.exe"

  # --- add to user PATH if missing ----------------------------------------
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if ($userPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to your user PATH. Open a new terminal to pick it up."
  }
}
finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Run 'envis version' to confirm, then 'envis init' in a project."
