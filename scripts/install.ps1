# easyGet Windows 安装脚本（PowerShell）
# 用法：
#   .\install.ps1                        # 只装二进制，配置走 easyGet pair
#   .\install.ps1 -Broker <url> -Code <8位配对码>   # 装完直接配对
# 可选环境变量：EASYGET_INSTALL_DIR（默认 $env:LOCALAPPDATA\Programs\easy-unlocker）

param(
    [string]$Broker = $env:EASY_UNLOCKER_BROKER_URL,
    [string]$Code,
    [string]$InstallDir = $(if ($env:EASYGET_INSTALL_DIR) { $env:EASYGET_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\easy-unlocker" })
)

$ErrorActionPreference = 'Stop'

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$binary = "easyGet-windows-$arch.exe"
$url = "https://github.com/cyancity/easy-unlocker/releases/latest/download/$binary"

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$target = Join-Path $InstallDir 'easyGet.exe'
$tmp = Join-Path $env:TEMP "easyGet-$PID.exe"

Write-Host "下载 $url"
Invoke-WebRequest -Uri $url -OutFile $tmp -UseBasicParsing
Move-Item -Force $tmp $target

# 装到用户级 PATH（已存在则不重复写）
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not ($userPath -split ';' | Where-Object { $_ -eq $InstallDir })) {
    [Environment]::SetEnvironmentVariable('Path', "$userPath;$InstallDir", 'User')
    $env:Path = "$env:Path;$InstallDir"
    Write-Host "已加入用户 PATH：$InstallDir（新开的终端生效）"
}

if ($Broker -and $Code) {
    & $target pair --broker $Broker --code $Code
    if ($LASTEXITCODE -ne 0) { throw "easyGet pair 失败（$LASTEXITCODE）" }
}

& $target ping
Write-Host "easyGet 已安装到 $target"
