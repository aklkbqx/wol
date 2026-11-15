# Installs the latest WOL release on 64-bit Windows.
#
#   irm https://raw.githubusercontent.com/aklkbqx/wol/main/install.ps1 | iex
#
# $env:WOL_VERSION picks a release; $env:WOL_INSTALL_DIR changes the target
# directory (default %LOCALAPPDATA%\Programs\wol). The archive is checked
# against the release's SHA256SUMS before anything is installed.
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'
    $repo = 'https://github.com/aklkbqx/wol'

    $version = $env:WOL_VERSION
    if (-not $version) {
        $version = (Invoke-RestMethod 'https://api.github.com/repos/aklkbqx/wol/releases/latest').tag_name
    }
    $version = $version.TrimStart('v')
    $file = "wol_${version}_windows_amd64.zip"
    $base = "$repo/releases/download/v$version"
    $dir = if ($env:WOL_INSTALL_DIR) { $env:WOL_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\wol' }

    $tmp = Join-Path ([IO.Path]::GetTempPath()) ('wol-' + [Guid]::NewGuid())
    New-Item -ItemType Directory -Path $tmp | Out-Null
    try {
        Write-Host "Downloading wol $version for windows/amd64"
        $zip = Join-Path $tmp $file
        $sums = Join-Path $tmp 'SHA256SUMS'
        Invoke-WebRequest "$base/$file" -OutFile $zip
        Invoke-WebRequest "$base/SHA256SUMS" -OutFile $sums

        $row = Get-Content $sums | Where-Object { ($_ -split '\s+')[1] -eq $file }
        if (-not $row) { throw "$file is not listed in SHA256SUMS" }
        $want = ($row -split '\s+')[0]
        if ((Get-FileHash $zip -Algorithm SHA256).Hash -ne $want) { throw "Checksum mismatch for $file" }

        Expand-Archive $zip -DestinationPath (Join-Path $tmp 'x')
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        Copy-Item (Join-Path $tmp 'x\wol.exe') -Destination $dir -Force
        & (Join-Path $dir 'wol.exe') version | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'The installed wol.exe does not run' }

        $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
        $entries = @($userPath -split ';' | Where-Object { $_ })
        if ($entries -notcontains $dir) {
            [Environment]::SetEnvironmentVariable('Path', (($entries + $dir) -join ';'), 'User')
            Write-Host "Added $dir to your PATH. Open a new terminal, then run wol doctor"
        } else {
            Write-Host 'Next: run wol doctor'
        }
        Write-Host "Installed wol $version to $dir\wol.exe"
    } finally {
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}
