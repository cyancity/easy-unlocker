# easy-unlocker 桌面客户端 Windows 安装脚本（PowerShell）
#
# 解决的问题：此前桌面端只能手工构建 exe 直接运行，开始菜单/搜索里找不到入口。
# 本脚本把客户端装成「正规的 Windows 用户级应用」：
#   exe   -> %LOCALAPPDATA%\Programs\easy-unlocker\easy-unlocker.exe
#   开始菜单快捷方式（可被搜索命中）
#   App Paths 注册（Win+R 输 easy-unlocker 可启动）
#   「应用列表/设置-应用」里的卸载项（卸载走本脚本 -Uninstall）
#   可选 -Startup：登录自启动（托盘常驻场景推荐）
#
# 用法：
#   .\install-desktop.ps1                                  # 下载 latest release 安装
#   .\install-desktop.ps1 -BinaryPath .\easy-unlocker.exe  # 安装本地构建
#   .\install-desktop.ps1 -Startup                         # 同时加登录自启动
#   .\install-desktop.ps1 -Uninstall                       # 卸载（删 exe/快捷方式/注册项）

param(
    [string]$BinaryPath = $env:EASYUNLOCKER_BINARY,
    [switch]$Startup,
    [switch]$Uninstall,
    [switch]$NoStartMenu,
    [string]$InstallDir = $(if ($env:EASYUNLOCKER_INSTALL_DIR) { $env:EASYUNLOCKER_INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\easy-unlocker" })
)

$ErrorActionPreference = 'Stop'
$appName = 'easy-unlocker'
$exeName = 'easy-unlocker.exe'
$target = Join-Path $InstallDir $exeName
$startMenuLnk = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\easy-unlocker.lnk"
$startupLnk = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup\easy-unlocker.lnk"
$appPathsKey = "HKCU:\Software\Microsoft\Windows\CurrentVersion\App Paths\$exeName"
$uninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\easy-unlocker'

function Remove-Install {
    foreach ($p in @($target)) {
        if (Test-Path -LiteralPath $p) {
            # 常驻托盘：exe 可能正在运行，先劝退再删。
            Get-Process -Name $appName -ErrorAction SilentlyContinue | Stop-Process -Force
            Remove-Item -LiteralPath $p -Force
        }
    }
    Remove-Item -LiteralPath $startMenuLnk, $startupLnk -Force -ErrorAction SilentlyContinue
    Remove-Item $appPathsKey, $uninstallKey -Recurse -Force -ErrorAction SilentlyContinue
    # 脚本副本与目录：目录里还有 easyGet.exe（CLI 安装器共用）时保留。
    Remove-Item -LiteralPath (Join-Path $InstallDir 'uninstall-desktop.ps1') -Force -ErrorAction SilentlyContinue
    if ($InstallDir -and (Test-Path -LiteralPath $InstallDir) -and -not (Get-ChildItem -LiteralPath $InstallDir -Force)) {
        Remove-Item -LiteralPath $InstallDir -Force
    }
    Write-Host 'easy-unlocker 桌面端已卸载（配置与本地库缓存保留，在 %APPDATA%\easy-unlocker）。'
}

if ($Uninstall) { Remove-Install; exit 0 }

$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
# 覆盖安装：旧实例占着 exe 会 Move 失败，先劝退（配置都在文件里，无内存态丢失风险）。
Get-Process -Name $appName -ErrorAction SilentlyContinue | Stop-Process -Force
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$tmp = Join-Path $InstallDir ('.' + $exeName + '.' + [guid]::NewGuid().ToString('N') + '.tmp')
try {
    if ($BinaryPath) {
        Copy-Item -LiteralPath $BinaryPath -Destination $tmp
    } else {
        $url = "https://github.com/cyancity/easy-unlocker/releases/latest/download/easy-unlocker-desktop-windows-$arch.exe"
        Write-Host "下载 $url"
        Invoke-WebRequest -Uri $url -OutFile $tmp -UseBasicParsing
        # SHA256 与发布清单对照：防下载链路被替换。清单拉不到只警告（首版 release 可能还没有桌面产物）。
        $sums = $null
        try { $sums = (Invoke-WebRequest -Uri 'https://github.com/cyancity/easy-unlocker/releases/latest/download/SHA256SUMS' -UseBasicParsing).Content } catch {}
        if ($sums) {
            $want = (($sums -split "`n") | Where-Object { $_ -match "easy-unlocker-desktop-windows-$arch\.exe" } | Select-Object -First 1)
            if ($want) {
                $want = ($want -replace '^([0-9a-fA-F]{64}).*$', '$1').ToLower()
                $got = (Get-FileHash -LiteralPath $tmp -Algorithm SHA256).Hash.ToLower()
                if ($want -ne $got) {
                    Remove-Item -LiteralPath $tmp -Force
                    throw "SHA256 校验失败（清单 $want，实际 $got），已丢弃下载文件"
                }
            }
        } else {
            Write-Warning 'SHA256SUMS 拉不到，跳过校验继续安装。'
        }
    }
    Move-Item -LiteralPath $tmp -Destination $target -Force
} finally {
    if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Force }
}

# 开始菜单快捷方式：开始菜单搜索「easy-unlocker」即可命中。
if (-not $NoStartMenu) {
    $shell = New-Object -ComObject WScript.Shell
    $lnk = $shell.CreateShortcut($startMenuLnk)
    $lnk.TargetPath = $target
    $lnk.WorkingDirectory = $InstallDir
    $lnk.Description = 'easy-unlocker 桌面端'
    $lnk.Save()
}

# App Paths：Win+R 与部分启动器按可执行名解析。
New-Item -Path $appPathsKey -Force | Out-Null
Set-ItemProperty -Path $appPathsKey -Name '(Default)' -Value $target

# 卸载项：出现在「设置-应用-已安装的应用」。
$ver = 'dev'
try { $ver = (& $target --version 2>$null | Select-Object -First 1) } catch {}
if (-not $ver) { $ver = 'dev' }
New-Item -Path $uninstallKey -Force | Out-Null
Set-ItemProperty -Path $uninstallKey -Name 'DisplayName' -Value 'easy-unlocker'
Set-ItemProperty -Path $uninstallKey -Name 'DisplayVersion' -Value "$ver"
Set-ItemProperty -Path $uninstallKey -Name 'Publisher' -Value 'cyancity'
Set-ItemProperty -Path $uninstallKey -Name 'InstallLocation' -Value $InstallDir
Set-ItemProperty -Path $uninstallKey -Name 'DisplayIcon' -Value $target
Set-ItemProperty -Path $uninstallKey -Name 'NoModify' -Value 1
Set-ItemProperty -Path $uninstallKey -Name 'NoRepair' -Value 1
# 卸载脚本落到安装目录——UninstallString 不能指向仓库路径（仓库会挪）。
$localScript = Join-Path $InstallDir 'uninstall-desktop.ps1'
Copy-Item -LiteralPath $PSCommandPath -Destination $localScript -Force
Set-ItemProperty -Path $uninstallKey -Name 'UninstallString' -Value "powershell.exe -NoProfile -ExecutionPolicy Bypass -File `"$localScript`" -Uninstall"

# 登录自启动（可选）：托盘常驻、锁屏收通知都靠进程活着。
if ($Startup) {
    $shell = New-Object -ComObject WScript.Shell
    $lnk = $shell.CreateShortcut($startupLnk)
    $lnk.TargetPath = $target
    $lnk.WorkingDirectory = $InstallDir
    $lnk.Save()
    Write-Host '已加入登录自启动。'
}

Write-Host "已安装到 $target"
Write-Host '开始菜单搜索 easy-unlocker 打开；Win+R 输 easy-unlocker 也可以。'
