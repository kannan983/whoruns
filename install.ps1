# Install whoruns on Windows (PowerShell):
#   irm https://raw.githubusercontent.com/kannan983/whoruns/main/install.ps1 | iex
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue' # makes Invoke-WebRequest much faster

$repo = 'kannan983/whoruns'
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$file = "whoruns_windows_$arch.zip"
$base = "https://github.com/$repo/releases/latest/download"
$dir = Join-Path $env:LOCALAPPDATA 'whoruns'
$tmp = Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid())

New-Item -ItemType Directory -Force -Path $tmp, $dir | Out-Null
try {
    Write-Host "Downloading $file..."
    Invoke-WebRequest "$base/$file" -OutFile "$tmp\$file" -UseBasicParsing
    Invoke-WebRequest "$base/checksums.txt" -OutFile "$tmp\checksums.txt" -UseBasicParsing

    $line = Get-Content "$tmp\checksums.txt" | Where-Object { $_ -match "  $([regex]::Escape($file))$" }
    $expected = if ($line) { ($line -split '\s+')[0] } else { '' }
    $actual = (Get-FileHash "$tmp\$file" -Algorithm SHA256).Hash.ToLower()
    if (-not $expected -or $expected -ne $actual) { throw "checksum mismatch for $file" }

    Expand-Archive "$tmp\$file" -DestinationPath $tmp -Force
    Copy-Item "$tmp\whoruns.exe" $dir -Force
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $dir) {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$dir", 'User')
    $env:Path += ";$dir"
    Write-Host "Added $dir to your PATH (new terminals pick it up automatically)"
}
Write-Host "Installed $(& "$dir\whoruns.exe" --version) to $dir\whoruns.exe"
