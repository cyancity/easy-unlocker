# easyGet Windows 安装脚本（PowerShell）
# 用法：
#   .\install.ps1                        # 只装二进制，配置走 easyGet pair
#   .\install.ps1 -Broker <url> -Code <8位配对码>   # 装完直接配对
# 可选环境变量：EASYGET_INSTALL_DIR（默认 $env:LOCALAPPDATA\Programs\easy-unlocker）

param(
    [string]$Broker = $env:EASY_UNLOCKER_BROKER_URL,
    [string]$Code,
    [string]$BinaryPath = $env:EASYGET_BINARY,
    [switch]$SessionOnly,
    [string]$InstallDir = $(if ($env:EASYGET_INSTALL_DIR) { $env:EASYGET_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\easy-unlocker" })
)

$ErrorActionPreference = 'Stop'
if ($Code -and -not $Broker) { throw '-Code 需要同时指定 -Broker' }

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$binary = "easyGet-windows-$arch.exe"
$url = "https://github.com/cyancity/easy-unlocker/releases/latest/download/$binary"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$target = Join-Path $InstallDir 'easyGet.exe'
$tmp = Join-Path $InstallDir ('.easyGet-' + [guid]::NewGuid().ToString('N') + '.exe')
try {
    if ($BinaryPath) {
        Copy-Item -LiteralPath $BinaryPath -Destination $tmp
    } else {
        Write-Host "下载 $url"
        Invoke-WebRequest -Uri $url -OutFile $tmp -UseBasicParsing
    }
    $installedVersion = & $tmp --version
    if ($LASTEXITCODE -ne 0) { throw '下载的 easyGet 无法运行，请检查版本与系统架构' }
    Move-Item -LiteralPath $tmp -Destination $target -Force
} finally {
    if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Force }
}

# 放到 PATH 前面，避免继续调用旧安装目录里的二进制。
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$remainingPath = @($userPath -split ';' | Where-Object { $_ -and $_.TrimEnd('\') -ne $InstallDir.TrimEnd('\') })
$newUserPath = (@($InstallDir) + $remainingPath) -join ';'
if (-not $SessionOnly -and $newUserPath -ne $userPath) {
    [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
}
$processPath = @($env:Path -split ';' | Where-Object { $_ -and $_.TrimEnd('\') -ne $InstallDir.TrimEnd('\') })
$env:Path = (@($InstallDir) + $processPath) -join ';'

if ($Broker -and $Code) {
    & $target pair --broker $Broker --code $Code
    if ($LASTEXITCODE -ne 0) { throw "easyGet pair 失败（$LASTEXITCODE）" }
    & $target ping
    if ($LASTEXITCODE -ne 0) { throw "easyGet ping 失败（$LASTEXITCODE）" }
}

Write-Host "easyGet $installedVersion 已安装到 $target"
if (-not $SessionOnly) { Write-Host '新开的终端使用更新后的 PATH。' }
Write-Host '尚未配对时运行 easyGet pair --broker <url> --code <配对码>'
