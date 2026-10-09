<#
.SYNOPSIS
    《自动安装工具》离线前置一键部署：安装 7-Zip、释放 OCR 引擎、写好 config.json。

.DESCRIPTION
    把交付包里的第三方离线物料变成"开箱即用"，对应 PRD 的 F16（前置依赖自检与用户提示）
    与 DEPLOY.md「镜像准备」章节。四步：

      [1] 7-Zip   —— 先找系统上**已有的**可用 7z.exe（-SevenZipExe → config.json → 注册表 →
                     PATH → 常见安装目录 → 交付包内便携目录）；确实没有，才用
                     third_party\7zip\7z*-x64.exe 静默安装（/S /D=<目录>）。
                     注意：使用该安装包时，7-Zip 安装器会把自身安装路径写进注册表
                     （HKLM/HKCU 的 SOFTWARE\7-Zip 与卸载项），这是安装 7-Zip 的正常副作用。
      [2] OCR     —— 用上一步得到的 7z.exe 把 third_party\ocr\*.7z 释放到
                     <OCRRoot>\PaddleOCR-json 与 <OCRRoot>\RapidOCR-json，保留 models 与 DLL。
                     默认两个引擎都装：工具在 CPU 无 AVX 时会自动从 PaddleOCR 回落到 RapidOCR。
      [3] config  —— 在 AutoTrigger.exe 同目录写**最小增量** config.json（显式 sevenzip_path；
                     ocr.engine_path 保持 auto，这样上一条的自动回落才依然有效）。覆盖前自动备份。
      [4] 自检    —— 调 `AutoTrigger.exe --check-prereq --json` 复核，报告与工具本体同源。

    退出码：0 全部就绪 / 4 缺 7-Zip / 5 缺 OCR（要求部署但未就绪）/ 1 其他错误。
    比工具本体默认更严格：工具在 ocr.require_ocr=false 时 OCR 缺失也返回 0，而本脚本要求
    "要装就必须装好"，便于镜像构建阶段做卡点。

    合规性：只读探测用 `-Check`；只打印不落盘用 `-DryRun`。脚本不联网，全部物料来自交付包。

.PARAMETER Root
    交付根目录（含 AutoTrigger.exe）。缺省按"脚本上级目录 → 脚本目录 → 上级目录\dist"顺序探测。

.PARAMETER SevenZipExe
    直接指定已有的 7z.exe，跳过发现与安装。

.PARAMETER SevenZipDir
    安装 7-Zip 的目标目录。缺省 `<Root>\third_party\7zip\7-Zip`。

.PARAMETER OCRRoot
    OCR 引擎释放根目录，缺省 `<Root>\third_party\ocr`；最终目录为 `<OCRRoot>\PaddleOCR-json` 等。

.PARAMETER Engine
    要释放的 OCR 引擎：auto/both（缺省，两个都装）、paddleocr_json、rapidocr_json、none。

.PARAMETER ConfigPath
    写出的配置文件路径，缺省 `<Root>\config.json`。

.PARAMETER MinSevenZipVersion
    最低 7-Zip 版本；缺省从 `AutoTrigger.exe --dump-config` 的 archive.min_version 读取。

.PARAMETER SkipSevenZip
    跳过 7-Zip 的发现与安装。

.PARAMETER SkipOCR
    跳过 OCR 释放。

.PARAMETER NoConfig
    不写 config.json（只装前置）。

.PARAMETER Force
    目标已存在也重新释放 / 重新写配置（OCR 目标目录会先清空）。

.PARAMETER Check
    只探测并报告当前前置状态，不做任何改动。

.PARAMETER DryRun
    打印将要执行的动作，但不安装、不解压、不写文件。

.PARAMETER Json
    结构化结果以 JSON 输出到 stdout，人类可读报告改走 stderr（对齐 PRD F10.1 三通道约定）。

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\install-windows.ps1

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\install-windows.ps1 -Check -Json

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\install-windows.ps1 `
        -OCRRoot D:\tools -SevenZipExe 'D:\software\7-Zip\7z.exe' -SkipSevenZip

.NOTES
    仅 Windows。建议 Windows PowerShell 5.1 运行；已按 PowerShell 2.0 语法编写，Win7 SP1
    自带的 PowerShell 2.0 也能跑（不依赖 ConvertTo-Json / Get-FileHash / Expand-Archive）。
#>

[CmdletBinding()]
param(
    [string]$Root,
    [string]$SevenZipExe,
    [string]$SevenZipDir,
    [string]$OCRRoot,
    [ValidateSet('auto', 'both', 'paddleocr_json', 'rapidocr_json', 'none')]
    [string]$Engine = 'auto',
    [string]$ConfigPath,
    [string]$MinSevenZipVersion,
    [switch]$SkipSevenZip,
    [switch]$SkipOCR,
    [switch]$NoConfig,
    [switch]$Force,
    [switch]$Check,
    [switch]$DryRun,
    [switch]$Json
)

$ErrorActionPreference = 'Continue'
$script:JsonMode = [bool]$Json
$script:Issues = New-Object System.Collections.ArrayList
$script:Steps = New-Object System.Collections.ArrayList
$script:EnginePaths = @{}
$script:SevenZipPath = $null
$script:SevenZipVersion = $null
$script:SevenZipInstalledTo = $null
$script:ConfigWritten = $false
$script:ConfigBackup = $null
$script:NoModify = [bool]($Check -or $DryRun)
$script:ScriptPath = $MyInvocation.MyCommand.Definition
$script:ScriptDir = Split-Path -Parent $script:ScriptPath

if ($Json) {
    try { [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false) } catch { }
}

# ---------------------------------------------------------------- 输出与工具函数

function Out-Log([string]$Text) {
    if ($script:JsonMode) { [Console]::Error.WriteLine($Text) } else { Write-Host $Text }
}
function Out-Section([string]$Title) { Out-Log ''; Out-Log ('== ' + $Title + ' ==') }
function Out-Ok([string]$Text) { Out-Log ('  [ok]   ' + $Text) }
function Out-Skip([string]$Text) { Out-Log ('  [skip] ' + $Text) }
function Out-Do([string]$Text) { Out-Log ('  [执行] ' + $Text) }
function Out-Warn([string]$Text) {
    [void]$script:Issues.Add($Text)
    Out-Log ('  [警告] ' + $Text)
}
function Out-Fail([string]$Text) {
    [void]$script:Issues.Add($Text)
    Out-Log ('  [失败] ' + $Text)
}
function Add-Step([string]$Name, [string]$Status, [string]$Detail) {
    [void]$script:Steps.Add(@{ name = $Name; status = $Status; detail = $Detail })
}

function Esc-Json([string]$Value) {
    if ($null -eq $Value) { return 'null' }
    $s = [string]$Value
    $s = $s.Replace('\', '\\').Replace('"', '\"').Replace("`r", '\r').Replace("`n", '\n').Replace("`t", '\t')
    return '"' + $s + '"'
}
function Unescape-JsonPath([string]$Value) {
    if ($null -eq $Value) { return $null }
    return $Value.Replace('\\', '\').Replace('\/', '/')
}
function Test-Admin {
    try {
        $id = [Security.Principal.WindowsIdentity]::GetCurrent()
        $p = New-Object Security.Principal.WindowsPrincipal($id)
        return $p.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    } catch { return $false }
}

# 把 "24.09" 与 "26.0" 这类版本号做逐段数值比较
function Test-VersionAtLeast([string]$Have, [string]$Need) {
    if (-not $Have) { return $false }
    if (-not $Need) { return $true }
    $a = @($Have -split '\.')
    $b = @($Need -split '\.')
    $n = $a.Count
    if ($b.Count -gt $n) { $n = $b.Count }
    for ($i = 0; $i -lt $n; $i++) {
        $x = 0; $y = 0
        if ($i -lt $a.Count) { [void][int]::TryParse($a[$i], [ref]$x) }
        if ($i -lt $b.Count) { [void][int]::TryParse($b[$i], [ref]$y) }
        if ($x -ne $y) { return ($x -gt $y) }
    }
    return $true
}

# 探测交付根目录：优先含 AutoTrigger.exe 的目录
function Find-DeliveryRoot([string]$Given) {
    $candidates = New-Object System.Collections.ArrayList
    if ($Given) { [void]$candidates.Add($Given) }
    else {
        if ($script:ScriptDir) {
            [void]$candidates.Add((Split-Path -Parent $script:ScriptDir))
            [void]$candidates.Add($script:ScriptDir)
            [void]$candidates.Add((Join-Path (Split-Path -Parent $script:ScriptDir) 'dist'))
        }
        [void]$candidates.Add((Get-Location).Path)
    }
    foreach ($c in $candidates) {
        if (-not $c) { continue }
        if (-not (Test-Path $c)) { continue }
        if (Test-Path (Join-Path $c 'AutoTrigger.exe')) { return (Resolve-Path $c).Path }
    }
    return $null
}

function Get-ConfigSevenZipPath([string]$RootPath) {
    $cfg = Join-Path $RootPath 'config.json'
    if (-not (Test-Path $cfg)) { return $null }
    try {
        $text = [System.IO.File]::ReadAllText($cfg)
        $m = [regex]::Match($text, '"sevenzip_path"\s*:\s*"([^"]*)"')
        if ($m.Success) {
            $v = Unescape-JsonPath $m.Groups[1].Value
            if ($v -and $v.ToLower() -ne 'auto') { return $v }
        }
    } catch { }
    return $null
}

function Get-RegistrySevenZipCandidates {
    $out = New-Object System.Collections.ArrayList
    foreach ($key in @('HKLM:\SOFTWARE\7-Zip', 'HKLM:\SOFTWARE\WOW6432Node\7-Zip', 'HKCU:\SOFTWARE\7-Zip')) {
        try {
            if (Test-Path $key) {
                $v = Get-ItemProperty -Path $key -ErrorAction SilentlyContinue
                if ($v) {
                    if ($v.Path) { [void]$out.Add((Join-Path $v.Path '7z.exe')) }
                    if ($v.Path64) { [void]$out.Add((Join-Path $v.Path64 '7z.exe')) }
                }
            }
        } catch { }
    }
    return $out
}

# 跑一次 `7z.exe i`，返回版本号（失败返回 $null）
function Get-SevenZipVersion([string]$Path) {
    if (-not (Test-Path $Path)) { return $null }
    try {
        $text = (& $Path 'i' 2>&1 | Out-String)
    } catch { return $null }
    $m = [regex]::Match($text, '7-Zip(?: \([^)]*\))?\s+([0-9]+\.[0-9]+)')
    if ($m.Success) { return $m.Groups[1].Value }
    return $null
}

# 依次探测候选，返回第一个**真能跑**的 7z.exe
function Resolve-SevenZip([string]$RootPath) {
    $cands = New-Object System.Collections.ArrayList
    if ($SevenZipExe) { [void]$cands.Add($SevenZipExe) }
    $fromCfg = Get-ConfigSevenZipPath $RootPath
    if ($fromCfg) { [void]$cands.Add($fromCfg) }
    foreach ($c in (Get-RegistrySevenZipCandidates)) { [void]$cands.Add($c) }
    $cmd = Get-Command '7z.exe' -ErrorAction SilentlyContinue
    if ($cmd) { [void]$cands.Add($cmd.Definition) }
    [void]$cands.Add('C:\Program Files\7-Zip\7z.exe')
    [void]$cands.Add('C:\Program Files (x86)\7-Zip\7z.exe')
    [void]$cands.Add((Join-Path $RootPath 'third_party\7zip\7-Zip\7z.exe'))
    foreach ($c in $cands) {
        if (-not $c) { continue }
        $v = Get-SevenZipVersion $c
        if ($v) { return @{ path = $c; version = $v } }
    }
    return $null
}

function Get-ArchivePath([string]$RootPath, [string]$Keyword) {
    $dir = Join-Path $RootPath 'third_party\ocr'
    if (-not (Test-Path $dir)) { return $null }
    $hit = Get-ChildItem -Path $dir -Filter '*.7z' -ErrorAction SilentlyContinue |
        Where-Object { $_.Name.ToLower().Contains($Keyword.ToLower()) } |
        Sort-Object Name | Select-Object -First 1
    if ($hit) { return $hit.FullName }
    return $null
}

function Get-ThreeZipPackage([string]$RootPath) {
    $dir = Join-Path $RootPath 'third_party\7zip'
    if (-not (Test-Path $dir)) { return $null }
    $hit = Get-ChildItem -Path $dir -Filter '*x64.exe' -ErrorAction SilentlyContinue |
        Sort-Object Name | Select-Object -First 1
    if ($hit) { return $hit.FullName }
    return $null
}

function Get-ToolMinVersion([string]$RootPath, [string]$ExePath) {
    if ($MinSevenZipVersion) { return $MinSevenZipVersion }
    try {
        $text = (& $ExePath '--dump-config' 2>$null | Out-String)
        $m = [regex]::Match($text, '"min_version"\s*:\s*"([^"]+)"')
        if ($m.Success) { return $m.Groups[1].Value }
    } catch { }
    return '24.09'
}

# 释放一个 OCR 压缩包到 <OCRRoot>\<Stem>，并压平单层根目录
function Expand-OCRArchive([string]$SevenZip, [string]$Archive, [string]$TargetDir, [string]$ExeName, [bool]$Overwrite) {
    $staging = Join-Path $env:TEMP ('at-ocr-stage-' + [Guid]::NewGuid().ToString('N'))
    try {
        if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
        New-Item -ItemType Directory -Force -Path $staging | Out-Null
        # 把 7z 的"当前目录"设成 staging，就不用打 -o<目录>，也就没有路径带空格的引号问题
        $rc = 1
        Push-Location $staging
        try {
            $null = & $SevenZip 'x' $Archive '-y' '-bso0' '-bsp0' 2>&1
            $rc = $LASTEXITCODE
        } finally { Pop-Location }
        if ($rc -ne 0) { throw ('7z 解压返回 ' + $rc) }
        $top = @(Get-ChildItem -Path $staging -Force)
        $src = $staging
        if ($top.Count -eq 1 -and $top[0].PSIsContainer) { $src = $top[0].FullName }
        if (Test-Path $TargetDir) {
            if (-not $Overwrite) { return $false }
            Remove-Item -Recurse -Force $TargetDir
        }
        New-Item -ItemType Directory -Force -Path $TargetDir | Out-Null
        Get-ChildItem -Path $src -Force | ForEach-Object {
            Move-Item -LiteralPath $_.FullName -Destination $TargetDir -Force
        }
        return (Test-Path (Join-Path $TargetDir $ExeName))
    } finally {
        if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
    }
}

# ---------------------------------------------------------------- [1] 7-Zip

function Step-SevenZip([string]$RootPath, [string]$MinVersion) {
    Out-Section '第 1 步 / 共 4 步：7-Zip'
    if ($SkipSevenZip) {
        # 即使跳过安装，也照样探测一次：OCR 步骤要用同一个 7z.exe 解压
        $found = Resolve-SevenZip $RootPath
        if ($found) {
            $script:SevenZipPath = $found.path
            $script:SevenZipVersion = $found.version
            Out-Skip ('按 -SkipSevenZip 跳过安装；复用已有 7z.exe：' + $found.path + '（' + $found.version + '）')
            Add-Step '7-Zip' 'skipped' ($found.path + '（复用，未安装）')
            return $found.path
        }
        Out-Skip '按 -SkipSevenZip 跳过（且系统上未探测到可用 7z.exe）'
        Add-Step '7-Zip' 'skipped' '用户跳过'
        return $null
    }
    $found = Resolve-SevenZip $RootPath
    if ($found) {
        $script:SevenZipPath = $found.path
        $script:SevenZipVersion = $found.version
        $ok = Test-VersionAtLeast $found.version $MinVersion
        if ($ok) {
            Out-Ok ('复用已有 7-Zip：' + $found.path + '（版本 ' + $found.version + '，要求 >= ' + $MinVersion + '）')
            Add-Step '7-Zip' 'ok' ($found.path + ' @ ' + $found.version + '（复用）')
        } else {
            Out-Warn ('已有 7-Zip ' + $found.version + ' 低于要求 ' + $MinVersion + '：仍会使用，但压缩包处理可能异常')
            Add-Step '7-Zip' 'warn' ($found.path + ' @ ' + $found.version + '（低于 ' + $MinVersion + '）')
        }
        return $found.path
    }

    # 没有任何可用 7z.exe：用交付包内的安装包静默安装
    $pkg = Get-ThreeZipPackage $RootPath
    if (-not $pkg) {
        Out-Fail ('未找到 7-Zip：系统无可用 7z.exe，交付包也缺 third_party\7zip\7z*-x64.exe')
        Add-Step '7-Zip' 'missing' '无可用 7z.exe 且无离线安装包'
        return $null
    }
    $admin = Test-Admin
    $target = $SevenZipDir
    if (-not $target) {
        $target = Join-Path $RootPath 'third_party\7zip\7-Zip'
    }
    $commandLine = '/S /D=' + $target
    Out-Do ('静默安装 7-Zip：' + $pkg)
    Out-Log  ('         命令行: "' + $pkg + '" ' + $commandLine)
    Out-Log  ('         目标目录: ' + $target + '（当前' + $(if ($admin) { '为管理员' } else { '非管理员，故装到交付包内以免权限不足' }) + '）')
    Out-Log  '         提示: 7-Zip 安装器会把安装路径写入注册表 SOFTWARE\7-Zip 与卸载项，属正常副作用'
    if ($script:NoModify) {
        Out-Skip $(if ($Check) { '按 -Check 只探测，不安装' } else { '按 -DryRun 只演示，不安装' })
        Add-Step '7-Zip' 'missing' ('未安装（' + $(if ($Check) { '-Check' } else { '-DryRun' }) + '）；计划装到 ' + $target)
        return $null
    }
    try {
        # 这里必须用 Start-Process（ShellExecute）而不是 &：安装器声明了 requireAdministrator，
        # 走 ShellExecute 才能触发 UAC 提权；用 & 会直接报"需要提升"而失败。
        $proc = Start-Process -FilePath $pkg -ArgumentList $commandLine -Wait -PassThru
        if ($proc.ExitCode -ne 0) { Out-Warn ('7-Zip 安装器返回退出码 ' + $proc.ExitCode) }
    } catch {
        Out-Fail ('7-Zip 安装器启动失败：' + $_.Exception.Message + '（如弹了 UAC 请点"是"，或改用管理员身份重跑）')
        Add-Step '7-Zip' 'failed' $_.Exception.Message
        return $null
    }
    $installed = Join-Path $target '7z.exe'
    $v = Get-SevenZipVersion $installed
    if (-not $v) {
        Out-Fail ('安装后仍未找到 ' + $installed + '：请确认安装器是否被 UAC/AV 拦截')
        Add-Step '7-Zip' 'failed' ('未生成 ' + $installed)
        return $null
    }
    $script:SevenZipPath = $installed
    $script:SevenZipVersion = $v
    $script:SevenZipInstalledTo = $target
    Out-Ok ('已安装 7-Zip ' + $v + ' → ' + $installed)
    if (-not (Test-VersionAtLeast $v $MinVersion)) {
        Out-Warn ('安装版本 ' + $v + ' 低于要求 ' + $MinVersion)
        Add-Step '7-Zip' 'warn' ($installed + ' @ ' + $v + '（低于 ' + $MinVersion + '）')
    } else {
        Add-Step '7-Zip' 'installed' ($installed + ' @ ' + $v)
    }
    return $installed
}

# ---------------------------------------------------------------- [2] OCR

function Step-OCR([string]$RootPath, [string]$SevenZip) {
    Out-Section '第 2 步 / 共 4 步：OCR 引擎'
    if ($SkipOCR -or $Engine -eq 'none') {
        Out-Skip $(if ($SkipOCR) { '按 -SkipOCR 跳过' } else { '按 -Engine none 跳过' })
        Add-Step 'OCR' 'skipped' '用户跳过'
        return
    }
    $want = New-Object System.Collections.ArrayList
    $paddle = @{ key = 'paddleocr_json'; stem = 'PaddleOCR-json'; exe = 'PaddleOCR-json.exe'; keyword = 'PaddleOCR' }
    $rapid = @{ key = 'rapidocr_json'; stem = 'RapidOCR-json'; exe = 'RapidOCR-json.exe'; keyword = 'RapidOCR' }
    $autoDir = Test-AutoOCRDir $OCRRoot
    if ($Engine -eq 'auto' -or $Engine -eq 'both') {
        if ($autoDir) {
            # 释放到工具自带搜索目录：两个都装，CPU 无 AVX 时工具会自动从 PaddleOCR 回落到 RapidOCR
            [void]$want.Add($paddle)
            [void]$want.Add($rapid)
        } else {
            # 自定义目录：engine_path 只能写死一个，回落无从谈起，故只装选定的那个
            [void]$want.Add($paddle)
            Out-Warn ('OCR 目标目录 ' + $OCRRoot + ' 不在工具自带搜索路径内：只安装 PaddleOCR-json；' +
                      'CPU 若未启用 AVX，请使用默认工具同级目录或 -Engine rapidocr_json')
        }
    } elseif ($Engine -eq 'paddleocr_json') {
        [void]$want.Add($paddle)
    } else {
        [void]$want.Add($rapid)
    }
    if (-not $SevenZip) { $SevenZip = $script:SevenZipPath }
    if (-not $SevenZip) {
        $again = Resolve-SevenZip $RootPath
        if ($again) { $SevenZip = $again.path }
    }
    if (-not $SevenZip) {
        Out-Fail '没有可用的 7z.exe，无法释放 OCR 压缩包（先解决 7-Zip，或用 --no-ocr 运行工具）'
        Add-Step 'OCR' 'missing' '缺 7z.exe'
        return
    }
    $allOK = $true
    foreach ($e in $want) {
        $targetDir = Join-Path $OCRRoot $e.stem
        $targetExe = Join-Path $targetDir $e.exe
        $script:EnginePaths[$e.key] = $targetExe
        if ((Test-Path $targetExe) -and (-not $Force)) {
            Out-Ok ('已存在，跳过：' + $targetExe)
            continue
        }
        $archive = Get-ArchivePath $RootPath $e.keyword
        if (-not $archive) {
            Out-Fail ('交付包缺 ' + $e.keyword + ' 离线压缩包（third_party\ocr\*' + $e.keyword + '*.7z）')
            $allOK = $false
            continue
        }
        Out-Do ('释放 ' + $e.keyword + ' → ' + $targetDir)
        if ($script:NoModify) {
            Out-Skip $(if ($Check) { '按 -Check 只探测，不解压' } else { '按 -DryRun 只演示，不解压' })
            continue
        }
        try {
            New-Item -ItemType Directory -Force -Path $OCRRoot | Out-Null
            $okFile = Expand-OCRArchive $SevenZip $archive $targetDir $e.exe ([bool]$Force)
            if ($okFile) { Out-Ok ('已就绪：' + $targetExe) }
            else { Out-Fail ('解压后未找到 ' + $targetExe); $allOK = $false }
        } catch {
            Out-Fail ('解压失败：' + $_.Exception.Message)
            $allOK = $false
        }
    }
    # 存在的引擎计入就绪集合
    foreach ($k in $want) {
        $p = Join-Path (Join-Path $OCRRoot $k.stem) $k.exe
        if (Test-Path $p) { $script:EnginePaths[$k.key] = $p }
    }
    if ($script:NoModify) {
        Add-Step 'OCR' 'planned' ('计划释放 ' + $want.Count + ' 个引擎到 ' + $OCRRoot)
    } elseif ($allOK -and $script:EnginePaths.Count -gt 0) {
        Add-Step 'OCR' 'ok' ((@($script:EnginePaths.Values) -join '; '))
    } else {
        Add-Step 'OCR' 'failed' '部分或全部引擎未就绪'
    }
}

# ---------------------------------------------------------------- [3] config.json

# 工具同级目录及常见路径可保持 engine_path="auto"。
function Test-AutoOCRDir([string]$Dir) {
    if (-not $Dir) { return $false }
    $d = $Dir.TrimEnd('\')
    foreach ($a in @('C:\tools', 'D:\tools', (Join-Path $root 'third_party\ocr'))) { if ($d -ieq $a) { return $true } }
    return $false
}

function Step-Config([string]$RootPath, [string]$EngineName) {
    Out-Section '第 3 步 / 共 4 步：写 config.json'
    if ($NoConfig) {
        Out-Skip '按 -NoConfig 跳过'
        Add-Step 'config.json' 'skipped' '用户跳过'
        return
    }
    $path = $ConfigPath
    if (-not $path) { $path = Join-Path $RootPath 'config.json' }
    # 最小增量覆盖：未列出的字段沿用工具内置默认值（config.Load 递归合并）
    $lines = New-Object System.Collections.ArrayList
    [void]$lines.Add('{')
    $block = New-Object System.Collections.ArrayList
    if ($script:SevenZipPath) {
        [void]$block.Add('  "archive": { "sevenzip_path": ' + (Esc-Json $script:SevenZipPath) + ' }')
    }
    if ($EngineName) {
        $enginePathValue = 'auto'
        if (-not (Test-AutoOCRDir $OCRRoot)) {
            # 自定义目录需要写入显式路径。
            if ($script:EnginePaths.ContainsKey($EngineName)) { $enginePathValue = $script:EnginePaths[$EngineName] }
            Out-Warn ('OCR 释放到自定义目录 ' + $OCRRoot + '：将在 config.json 里写死 engine_path；' +
                      '副作用是 CPU 无 AVX 时不能再自动回落到 RapidOCR，请自行把 ocr.engine 改成 rapidocr_json')
        }
        [void]$block.Add('  "ocr": { "enabled": true, "engine": ' + (Esc-Json $EngineName) + ', "engine_path": ' + (Esc-Json $enginePathValue) + ', "require_ocr": false }')
    }
    for ($i = 0; $i -lt $block.Count; $i++) {
        $suffix = ','
        if ($i -eq $block.Count - 1) { $suffix = '' }
        [void]$lines.Add($block[$i] + $suffix)
    }
    [void]$lines.Add('}')
    $text = ($lines -join "`r`n") + "`r`n"

    Out-Do ('写入 ' + $path)
    Out-Log ('         内容：' + ($block -join '  '))
    if ($block.Count -eq 0) {
        Out-Skip '没有需要覆盖的字段，跳过写配置（保持内置默认值即可）'
        Add-Step 'config.json' 'skipped' '无需覆盖'
        return
    }
    if ($script:NoModify) {
        Out-Skip $(if ($Check) { '按 -Check 只探测，不写文件' } else { '按 -DryRun 只演示，不写文件' })
        Add-Step 'config.json' 'planned' $path
        return
    }
    try {
        if (Test-Path $path) {
            $stamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
            $backup = $path + '.bak-' + $stamp
            Copy-Item -LiteralPath $path -Destination $backup -Force
            $script:ConfigBackup = $backup
            Out-Ok ('已备份原配置：' + $backup)
        }
        $utf8 = New-Object System.Text.UTF8Encoding($false)
        [System.IO.File]::WriteAllText($path, $text, $utf8)
        $script:ConfigWritten = $true
        Out-Ok ('已写入：' + $path)
        Add-Step 'config.json' 'ok' $path
    } catch {
        Out-Fail ('写配置失败：' + $_.Exception.Message)
        Add-Step 'config.json' 'failed' $_.Exception.Message
    }
}

# ---------------------------------------------------------------- [4] 自检

function Step-Verify([string]$RootPath) {
    Out-Section '第 4 步 / 共 4 步：前置自检（AutoTrigger --check-prereq）'
    $exe = Join-Path $RootPath 'AutoTrigger.exe'
    $raw = ''
    $code = $null
    try {
        $scratch = Join-Path $env:TEMP ('at-prereq-' + [Guid]::NewGuid().ToString('N'))
        if (Test-Path $scratch) { Remove-Item -Recurse -Force $scratch }
        New-Item -ItemType Directory -Force -Path $scratch | Out-Null
        $outFile = Join-Path $scratch 'out.json'
        $errFile = Join-Path $scratch 'err.log'
        # 用 & + 重定向而不是 Start-Process：Start-Process 的 -RedirectStandard* 在部分环境下
        # 拿不到退出码/输出，而 & 的 $LASTEXITCODE 稳定可靠（AutoTrigger 不需要提权启动）。
        Push-Location $scratch
        try {
            & $exe '--check-prereq' '--no-deploy-deps' '--json' > $outFile 2> $errFile
            $code = $LASTEXITCODE
        } finally { Pop-Location }
        if (Test-Path $outFile) { $raw = [System.IO.File]::ReadAllText($outFile) }
        Remove-Item -Recurse -Force $scratch -ErrorAction SilentlyContinue
    } catch {
        Out-Fail ('自检未能执行：' + $_.Exception.Message)
        Add-Step '自检' 'failed' $_.Exception.Message
        return @{ code = $null; json = ''; sevenzip = $null; sevenzipVersion = $null; ocr = $null; ocrPath = $null }
    }
    $get = {
        param($pattern)
        $m = [regex]::Match($raw, $pattern)
        if ($m.Success) { return $m.Groups[1].Value }
        return $null
    }
    $zip = & $get '"archive_backend"\s*:\s*(null|"[^"]*")'
    $zipV = & $get '"sevenzip_version"\s*:\s*(null|"[^"]*")'
    $ocrStatus = & $get '"ocr_engine_status"\s*:\s*"([^"]*)"'
    $ocrPath = & $get '"ocr_engine_path"\s*:\s*(null|"[^"]*")'
    if ($zip -and $zip -ne 'null') { Out-Ok ('7-Zip    : ' + (Unescape-JsonPath ($zip.Trim('"')))) }
    else { Out-Fail '7-Zip    : 工具没探测到可用的 7z.exe' }
    if ($zipV -and $zipV -ne 'null') { Out-Log ('         版本 ' + $zipV.Trim('"')) }
    if ($ocrStatus -eq 'ok') { Out-Ok ('OCR 引擎 : ' + (Unescape-JsonPath ($ocrPath.Trim('"')))) }
    else { Out-Warn ('OCR 引擎 : 状态 ' + $ocrStatus + '（工具会退回 UIA/Win32 + 模板；可用 --no-ocr 显式跳过）') }
    Out-Log ('         工具 --check-prereq 退出码：' + $code)
    Add-Step '自检' $(if ($code -eq 0) { 'ok' } else { 'warn' }) ('退出码 ' + $code)
    return @{
        code = $code
        json = $raw
        sevenzip = $(if ($zip -and $zip -ne 'null') { Unescape-JsonPath ($zip.Trim('"')) } else { $null })
        sevenzipVersion = $(if ($zipV -and $zipV -ne 'null') { $zipV.Trim('"') } else { $null })
        ocr = $ocrStatus
        ocrPath = $(if ($ocrPath -and $ocrPath -ne 'null') { Unescape-JsonPath ($ocrPath.Trim('"')) } else { $null })
    }
}

# ---------------------------------------------------------------- 主流程

$exit = 0
$root = Find-DeliveryRoot $Root
Out-Log '《自动安装工具》离线前置安装脚本'
Out-Log ('PowerShell ' + $PSVersionTable.PSVersion.ToString() + '；管理员=' + (Test-Admin) + '；' + $(if ($script:NoModify) { $(if ($Check) { '模式=只探测' } else { '模式=演示' }) } else { '模式=实际执行' }))

if (-not $root) {
    Out-Fail '没找到 AutoTrigger.exe：请用 -Root 指定交付根目录（或把脚本放在交付包的 scripts\ 下运行）'
    $exit = 1
} else {
    if (-not $OCRRoot) { $OCRRoot = Join-Path $root 'third_party\ocr' }
    $exe = Join-Path $root 'AutoTrigger.exe'
    Out-Log ('交付根目录：' + $root)
    $minVersion = Get-ToolMinVersion $root $exe
    $sevenZip = Step-SevenZip $root $minVersion
    Step-OCR $root $sevenZip

    $engineName = $null
    if (-not ($SkipOCR -or $Engine -eq 'none')) {
        # 主选引擎：装了两个或只装 PaddleOCR 时用 paddleocr_json —— 工具在 CPU 无 AVX 时
        # 会自动回落到 rapidocr_json，而 engine_path 保持 auto 才能让回落落到 RapidOCR 上。
        if ($Engine -eq 'rapidocr_json') { $engineName = 'rapidocr_json' } else { $engineName = 'paddleocr_json' }
    }
    Step-Config $root $engineName

    $verify = Step-Verify $root

    # 结论：退出码与工具本体 --check-prereq 的 4/5 语义对齐（但更严格：要装就必须装好）
    $sevenZipReady = [bool]($verify.sevenzip -or $script:SevenZipPath)
    $ocrReady = ($verify.ocr -eq 'ok')
    $ocrRequested = -not ($SkipOCR -or $Engine -eq 'none')
    if (-not $sevenZipReady) { $exit = 4 }
    elseif (-not $script:NoModify -and $ocrRequested -and -not $ocrReady) { $exit = 5 }
    elseif ($null -ne $verify.code -and $verify.code -ne 0 -and $verify.code -ne 5) { $exit = 1 }
    else { $exit = 0 }

    Out-Section '结论'
    $zipText = '未就绪'
    if ($script:SevenZipPath) { $zipText = $script:SevenZipPath + '（' + $script:SevenZipVersion + '）' }
    Out-Log ('  7-Zip    : ' + $zipText)
    if ($script:EnginePaths.Count -gt 0) {
        foreach ($k in $script:EnginePaths.Keys) {
            $mark = '未就绪'
            if (Test-Path $script:EnginePaths[$k]) { $mark = '就绪' }
            Out-Log ('  OCR      : ' + $k + ' → ' + $script:EnginePaths[$k] + ' [' + $mark + ']')
        }
    } else {
        Out-Log '  OCR      : 未部署'
    }
    if ($script:ConfigWritten) { Out-Log ('  配置文件 : ' + (Join-Path $root 'config.json') + '（已写入）') }
    elseif ($NoConfig) { Out-Log '  配置文件 : 按 -NoConfig 未写' }
    else { Out-Log '  配置文件 : 未写入' }

    if ($script:Issues.Count -gt 0) {
        Out-Log ''
        Out-Log ('  注意 ' + $script:Issues.Count + ' 条：')
        foreach ($i in $script:Issues) { Out-Log ('    - ' + $i) }
    }
    Out-Log ''
    if ($script:NoModify) {
        Out-Log '  （' + $(if ($Check) { '-Check' } else { '-DryRun' }) + ' 模式未做任何改动；去掉该开关即可真正安装）'
    } elseif ($exit -eq 0) {
        Out-Log '  下一步：双击 AutoTrigger.exe 打开界面，或执行'
        Out-Log ('    AutoTrigger.exe --input <样本路径> --output <结果.json>')
    } else {
        Out-Log '  请先解决上面的问题；安装动作建议以管理员身份重跑本脚本。'
    }
    Out-Log ('  退出码：' + $exit + $(if ($exit -eq 0) { '（前置就绪）' } elseif ($exit -eq 4) { '（缺 7-Zip）' } elseif ($exit -eq 5) { '（缺 OCR）' } else { '（出错）' }))
}

if ($Json) {
    $sb = New-Object System.Text.StringBuilder
    [void]$sb.AppendLine('{')
    [void]$sb.AppendLine('  "root": ' + (Esc-Json $root) + ',')
    [void]$sb.AppendLine('  "mode": ' + (Esc-Json $(if ($script:NoModify) { $(if ($Check) { 'check' } else { 'dry-run' }) } else { 'apply' })) + ',')
    [void]$sb.AppendLine('  "sevenzip": {')
    [void]$sb.AppendLine('    "path": ' + (Esc-Json $script:SevenZipPath) + ',')
    [void]$sb.AppendLine('    "version": ' + (Esc-Json $script:SevenZipVersion) + ',')
    [void]$sb.AppendLine('    "installed_to": ' + (Esc-Json $script:SevenZipInstalledTo))
    [void]$sb.AppendLine('  },')
    [void]$sb.AppendLine('  "ocr": {')
    [void]$sb.AppendLine('    "root": ' + (Esc-Json $OCRRoot) + ',')
    if ($script:EnginePaths.Count -gt 0) {
        $pairs = @()
        foreach ($k in $script:EnginePaths.Keys) {
            $st = 'missing'
            if (Test-Path $script:EnginePaths[$k]) { $st = 'ok' }
            $pairs += ((Esc-Json $k) + ': { "path": ' + (Esc-Json $script:EnginePaths[$k]) + ', "status": ' + (Esc-Json $st) + ' }')
        }
        [void]$sb.AppendLine('    "engines": { ' + ($pairs -join ', ') + ' }')
    } else {
        [void]$sb.AppendLine('    "engines": {}')
    }
    [void]$sb.AppendLine('  },')
    [void]$sb.AppendLine('  "config": {')
    [void]$sb.AppendLine('    "path": ' + (Esc-Json $(if ($NoConfig) { $null } else { $(if ($ConfigPath) { $ConfigPath } else { Join-Path $root 'config.json' }) })) + ',')
    [void]$sb.AppendLine('    "written": ' + $(if ($script:ConfigWritten) { 'true' } else { 'false' }) + ',')
    [void]$sb.AppendLine('    "backup": ' + (Esc-Json $script:ConfigBackup))
    [void]$sb.AppendLine('  },')
    if ($verify -and $verify.json) {
        [void]$sb.AppendLine('  "prereq": ' + $verify.json.Trim() + ',')
    } else {
        [void]$sb.AppendLine('  "prereq": null,')
    }
    $issues = @()
    foreach ($i in $script:Issues) { $issues += (Esc-Json $i) }
    [void]$sb.AppendLine('  "issues": [' + ($issues -join ', ') + '],')
    $steps = @()
    foreach ($s in $script:Steps) {
        $steps += ('{ "name": ' + (Esc-Json $s.name) + ', "status": ' + (Esc-Json $s.status) + ', "detail": ' + (Esc-Json $s.detail) + ' }')
    }
    [void]$sb.AppendLine('  "steps": [' + ($steps -join ', ') + '],')
    [void]$sb.AppendLine('  "exit_code": ' + $exit)
    [void]$sb.AppendLine('}')
    Write-Output $sb.ToString()
}

exit $exit
