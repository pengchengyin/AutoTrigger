<#
.SYNOPSIS
    OCR 引擎可用性自检：多引擎探测 + 能力自检 + 端到端识别验证。

.DESCRIPTION
    对应《自动安装工具》PRD 的 F16.3（前置依赖探测与能力自检）、F16.4（--check-prereq）
    以及交付包 scripts\verify-ocr.ps1（PRD 8.2 / D10）。用途是在**部署前 / 镜像构建阶段**
    验证目标机上 OCR 引擎是否**真的可用**，而不是只检查文件是否存在。

    流程：
      [0] 主机环境与 CPU 能力（AVX / AVX2 —— 决定 PaddleOCR-json 能否运行）
      [1] 引擎发现（优先级：-EnginePath 显式路径 → config.json 的 ocr.engine_path / ocr.engine
          → PATH → 常见安装路径）
      [2] 逐引擎能力自检 + 端到端识别（生成中英混排测试图，实际识别一次并输出
          文本 + 四角像素坐标 + score；PaddleOCR 系额外探测 stdio 管道模式）
      [3] 汇总：可用引擎、推荐 config.json 片段、退出码

    支持引擎：paddleocr_json（默认）/ rapidocr_json / umi_ocr / tesseract /
              windows（Windows 内置 OCR，可选回退后端，仅 PS 5.1）
    引擎选型依据见 PRD 7.6 / 7.7；v2.3 内置 OCR 实测结论见 PRD F14.6。

.PARAMETER EnginePath
    直接指定引擎可执行文件路径，跳过自动发现。
.PARAMETER Engine
    指定要验证的引擎：auto（默认，按上面顺序发现）/ paddleocr_json / rapidocr_json /
    umi_ocr / tesseract / windows / none。
.PARAMETER ConfigPath
    指定 config.json 路径，用于读取 ocr.engine 与 ocr.engine_path。缺省时自动在当前目录、
    脚本同级目录与其上级目录查找。
.PARAMETER TestImage
    使用已有图片做端到端识别；缺省时自动生成一张中英混排测试图。
.PARAMETER TimeoutSec
    单个引擎单次自检的超时秒数，默认 25。
.PARAMETER UmiApiUrl
    Umi-OCR 的 HTTP 识别接口，默认 http://127.0.0.1:1224/api/ocr。
.PARAMETER Json
    把结构化自检结果以 JSON 输出到 stdout（人类可读报告改走 stderr），
    便于镜像构建流水线做机器可判定的卡点（对齐 PRD F10.1 三通道约定）。
.PARAMETER KeepTestImage
    保留生成的测试图（默认识别完成后删除）。
.PARAMETER IncludeWindowsBuiltin
    在 auto 模式下也额外探测 Windows 内置 OCR（默认不探测，与 PRD DC8 一致：默认关闭）。

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File .\verify-ocr.ps1

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File .\verify-ocr.ps1 `
        -EnginePath "C:\tools\PaddleOCR-json\PaddleOCR-json.exe" -Json

.NOTES
    - 只读自检：不安装任何软件、不修改系统状态，仅在 %TEMP% 下写入一张测试图。
    - 退出码：0 = 至少一个引擎可用（或配置为不使用 OCR）；5 = 无可用引擎
      （与工具本体 --check-prereq 在 require_ocr:true 时的退出码 5 语义对齐）。
    - 建议用 Windows PowerShell 5.1（powershell.exe）运行；PowerShell 7+ 亦可，
      但 -Engine windows（内置 OCR 回退后端）依赖 WinRT 类型投影，仅 5.1 支持。
#>

[CmdletBinding()]
param(
    [string]$EnginePath,
    [ValidateSet('auto', 'paddleocr_json', 'rapidocr_json', 'umi_ocr', 'tesseract', 'windows', 'none')]
    [string]$Engine = 'auto',
    [string]$ConfigPath,
    [string]$TestImage,
    [int]$TimeoutSec = 25,
    [string]$UmiApiUrl = 'http://127.0.0.1:1224/api/ocr',
    [switch]$Json,
    [switch]$KeepTestImage,
    [switch]$IncludeWindowsBuiltin
)

$ErrorActionPreference = 'Continue'
$script:JsonMode = [bool]$Json
$script:Issues = New-Object System.Collections.ArrayList

if ($Json) {
    try { [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false) } catch { }
}

# ============================================================================
# 基础工具
# ============================================================================

function Write-Report {
    param([string]$Text, [string]$Color = 'Gray')
    if ($script:JsonMode) { [Console]::Error.WriteLine($Text) }
    elseif ($Text -eq '') { Write-Host '' }
    else { Write-Host $Text -ForegroundColor $Color }
}

function Write-Section {
    param([string]$Text)
    Write-Report ''
    Write-Report ("=== {0} ===" -f $Text) 'Cyan'
}

function Expand-PathVars {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { return $null }
    try {
        $p = [System.Environment]::ExpandEnvironmentVariables($Path)
        if ($p -match '^~(\\|/)') { $p = Join-Path $HOME $p.Substring(2) }
        return $p
    } catch { return $Path }
}

function Add-Issue {
    param([string]$Text, [string]$Level = 'WARN')
    [void]$script:Issues.Add([pscustomobject]@{ level = $Level; text = $Text })
}

# 单个参数转命令行文本（含空格/引号时加引号）
function Quote-Arg {
    param([string]$Value)
    if ($null -eq $Value) { return '""' }
    if ($Value -match '[\s"]') { return '"' + ($Value -replace '"', '\"') + '"' }
    return $Value
}

<#
    运行外部进程并采集输出。
    返回：Ok / TimedOut / ExitCode / StdOut / StdErr / ElapsedMs / Error
#>
function Invoke-Proc {
    param(
        [Parameter(Mandatory)][string]$Exe,
        [string]$ArgText = '',
        [string[]]$StdinLines,
        [int]$TimeoutSec = 25
    )
    $res = [pscustomobject]@{
        Ok = $false; TimedOut = $false; ExitCode = $null
        StdOut = ''; StdErr = ''; ElapsedMs = 0; Error = $null
    }
    if (-not (Test-Path -LiteralPath $Exe)) { $res.Error = '可执行文件不存在'; return $res }

    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $Exe
    $psi.Arguments = $ArgText
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.RedirectStandardInput = ($null -ne $StdinLines)
    $psi.StandardOutputEncoding = [System.Text.Encoding]::UTF8
    $psi.StandardErrorEncoding = [System.Text.Encoding]::UTF8
    try { $psi.WorkingDirectory = (Split-Path -Parent $Exe) } catch { }

    $proc = New-Object System.Diagnostics.Process
    $proc.StartInfo = $psi
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    try {
        [void]$proc.Start()
        $outTask = $proc.StandardOutput.ReadToEndAsync()
        $errTask = $proc.StandardError.ReadToEndAsync()
        if ($null -ne $StdinLines) {
            foreach ($line in $StdinLines) { $proc.StandardInput.WriteLine($line) }
            $proc.StandardInput.Flush()
            # 关闭 stdin 以发出 EOF：引擎处理完当前请求后会正常退出
            $proc.StandardInput.Close()
        }
        if (-not $proc.WaitForExit($TimeoutSec * 1000)) {
            $res.TimedOut = $true
            try { $proc.Kill() } catch { }
            Start-Sleep -Milliseconds 200
        }
        try { [void]$outTask.Wait(3000); $res.StdOut = [string]$outTask.Result } catch { $res.StdOut = '' }
        try { [void]$errTask.Wait(3000); $res.StdErr = [string]$errTask.Result } catch { $res.StdErr = '' }
        if (-not $res.TimedOut) { $res.ExitCode = $proc.ExitCode }
        $res.Ok = $true
    } catch {
        $res.Error = $_.Exception.Message
    } finally {
        $sw.Stop()
        $res.ElapsedMs = [int]$sw.ElapsedMilliseconds
        try { $proc.Dispose() } catch { }
    }
    return $res
}

# 从可能混有启动横幅的输出中解析 PaddleOCR-json 系的结果 JSON
function Parse-PaddleJson {
    param([string]$Text)
    if ([string]::IsNullOrWhiteSpace($Text)) { return $null }
    foreach ($line in ($Text -split "`r?`n")) {
        $t = $line.Trim()
        if ($t.StartsWith('{')) {
            try { return ($t | ConvertFrom-Json) } catch { }
        }
    }
    $m = [regex]::Match($Text, '(?s)\{.*"code"\s*:.*\}')
    if ($m.Success) { try { return ($m.Value | ConvertFrom-Json) } catch { } }
    return $null
}

# 从引擎输出里抓版本号（形如 v1.3.1 / 1.3.1 / 5.3.3.20231005）
function Get-VersionHint {
    param([string]$Text)
    if ([string]::IsNullOrWhiteSpace($Text)) { return $null }
    $m = [regex]::Match($Text, '(?i)\bv?(\d+\.\d+(?:\.\d+){0,3})\b')
    if ($m.Success) { return $m.Groups[1].Value }
    return $null
}

function Test-TcpPort {
    param([string]$Host = '127.0.0.1', [int]$Port = 1224, [int]$TimeoutMs = 800)
    $client = New-Object System.Net.Sockets.TcpClient
    try {
        $iar = $client.BeginConnect($Host, $Port, $null, $null)
        if ($iar.AsyncWaitHandle.WaitOne($TimeoutMs)) { $client.EndConnect($iar); return $true }
        return $false
    } catch { return $false } finally { try { $client.Close() } catch { } }
}

# ============================================================================
# [0] 主机环境与 CPU 能力
# ============================================================================

function Get-HostInfo {
    $os = [System.Environment]::OSVersion
    $osName = "$($os.VersionString)"

    # 架构探测：环境变量优先，缺失时用 .NET 运行时信息兜底
    $arch = $env:PROCESSOR_ARCHITECTURE
    if (-not $arch) { $arch = $env:PROCESSOR_ARCHITEW6432 }
    if (-not $arch) {
        try { $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString() } catch { }
    }
    if (-not $arch) {
        $arch = $(if ([System.Environment]::Is64BitOperatingSystem) { 'X64' } else { 'X86' })
    }

    try {
        $cv = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion' -ErrorAction SilentlyContinue
        if ($cv) { $osName = "$($cv.ProductName) $($cv.DisplayVersion) (Build $($cv.CurrentBuild).$($cv.UBR))" }
    } catch { }
    return [pscustomobject]@{
        os = $osName
        version = $os.Version.ToString()
        arch = $arch
        ps_version = $PSVersionTable.PSVersion.ToString()
        ps_edition = $PSVersionTable.PSEdition
        is_win7 = ($os.Version.Major -eq 6 -and $os.Version.Minor -eq 1)
    }
}

function Get-CpuInfo {
    $info = [pscustomobject]@{ name = $null; avx = $null; avx2 = $null; note = $null }
    try {
        $cpu = Get-CimInstance Win32_Processor -ErrorAction Stop | Select-Object -First 1
        if ($cpu) { $info.name = $cpu.Name.Trim() }
    } catch {
        $info.note = '无法通过 WMI 读取 CPU 名称（不影响后续自检）'
    }

    # IsProcessorFeaturePresent：PF_AVX_INSTRUCTIONS_AVAILABLE = 39，PF_AVX2 = 40
    try {
        if (-not ('Win32.NativeMethods' -as [type])) {
            Add-Type -Namespace Win32 -Name NativeMethods -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("kernel32.dll", SetLastError = true)]
public static extern bool IsProcessorFeaturePresent(uint processorFeature);
'@ -ErrorAction Stop
        }
        $info.avx = [Win32.NativeMethods]::IsProcessorFeaturePresent(39)
        $info.avx2 = [Win32.NativeMethods]::IsProcessorFeaturePresent(40)
    } catch {
        $info.note = 'AVX 探测不可用（Add-Type 被策略拦截或 API 缺失），将以引擎实际启动结果为准'
    }
    return $info
}

# ============================================================================
# [1] 引擎发现
# ============================================================================

$EngineCatalog = @(
    [pscustomobject]@{
        Id = 'paddleocr_json'; Name = 'PaddleOCR-json'; ExeName = 'PaddleOCR-json.exe'; NeedAvx = $true
        StandalonePaths = @(
            'C:\tools\PaddleOCR-json\PaddleOCR-json.exe',
            'D:\tools\PaddleOCR-json\PaddleOCR-json.exe',
            'C:\PaddleOCR-json\PaddleOCR-json.exe'
        )
        SearchRoots = @('%LOCALAPPDATA%\Programs', 'C:\tools', 'D:\tools', '%USERPROFILE%\Downloads')
        Hint = '默认引擎；中英日韩识别率高，返回四角坐标 + 原生 score；需 CPU 支持 AVX'
    },
    [pscustomobject]@{
        Id = 'rapidocr_json'; Name = 'RapidOCR-json'; ExeName = 'RapidOCR-json.exe'; NeedAvx = $false
        StandalonePaths = @(
            'C:\tools\RapidOCR-json\RapidOCR-json.exe',
            'D:\tools\RapidOCR-json\RapidOCR-json.exe',
            'C:\RapidOCR-json\RapidOCR-json.exe'
        )
        SearchRoots = @('%LOCALAPPDATA%\Programs', 'C:\tools', 'D:\tools', '%USERPROFILE%\Downloads')
        Hint = '不需要 AVX，老 CPU / 虚拟化环境适用；接口与 PaddleOCR-json 一致'
    },
    [pscustomobject]@{
        Id = 'umi_ocr'; Name = 'Umi-OCR'; ExeName = 'Umi-OCR.exe'; NeedAvx = $false
        StandalonePaths = @(
            'C:\tools\Umi-OCR\Umi-OCR.exe',
            'D:\tools\Umi-OCR\Umi-OCR.exe',
            'C:\Program Files\Umi-OCR\Umi-OCR.exe'
        )
        SearchRoots = @('%LOCALAPPDATA%\Programs', 'C:\tools', 'D:\tools', '%USERPROFILE%\Downloads')
        Hint = '完整 GUI 软件（体积大），提供 HTTP 接口；本脚本通过 HTTP 接口验证'
    },
    [pscustomobject]@{
        Id = 'tesseract'; Name = 'Tesseract-OCR'; ExeName = 'tesseract.exe'; NeedAvx = $false
        StandalonePaths = @('C:\Program Files\Tesseract-OCR\tesseract.exe')
        SearchRoots = @('C:\Program Files', '%LOCALAPPDATA%\Programs')
        Hint = '兼容性最广但中文识别率弱于 PaddleOCR；界面小字号文本需图像预处理'
    }
)

function Read-ConfigOcr {
    param([string]$Path)

    if (-not $Path) {
        $cands = New-Object System.Collections.ArrayList
        try { if ($PSScriptRoot) { [void]$cands.Add((Join-Path $PSScriptRoot 'config.json')) } } catch { }
        try { if ($PSScriptRoot) { [void]$cands.Add((Join-Path (Split-Path -Parent $PSScriptRoot) 'config.json')) } } catch { }
        try { [void]$cands.Add((Join-Path (Get-Location).Path 'config.json')) } catch { }
        foreach ($c in $cands) {
            if ($c -and (Test-Path -LiteralPath $c)) { $Path = $c; break }
        }
    }
    if (-not $Path -or -not (Test-Path -LiteralPath $Path)) { return $null }

    try {
        $raw = Get-Content -LiteralPath $Path -Raw -Encoding UTF8
        $cfg = $raw | ConvertFrom-Json
        $ocr = $cfg.ocr
        return [pscustomobject]@{
            path = (Resolve-Path -LiteralPath $Path).Path
            engine = $(if ($ocr) { $ocr.engine } else { $null })
            engine_path = $(if ($ocr) { $ocr.engine_path } else { $null })
            min_score = $(if ($ocr) { $ocr.min_score } else { $null })
            transport = $(if ($ocr) { $ocr.transport } else { $null })
        }
    } catch {
        Add-Issue ("config.json 解析失败（已忽略）：{0} —— {1}" -f $Path, $_.Exception.Message)
        return $null
    }
}

function Find-EngineExe {
    param($Spec, [string]$ExplicitPath, [string]$ConfigEnginePath)

    $cands = New-Object System.Collections.ArrayList
    foreach ($p in @($ExplicitPath, $ConfigEnginePath)) {
        if ($p) { [void]$cands.Add((Expand-PathVars $p)) }
    }
    foreach ($p in $Spec.StandalonePaths) { [void]$cands.Add((Expand-PathVars $p)) }

    $cmd = Get-Command $Spec.ExeName -ErrorAction SilentlyContinue
    if ($cmd) {
        $src = $null
        if ($cmd.PSObject.Properties['Path'] -and $cmd.Path) { $src = $cmd.Path }
        elseif ($cmd.PSObject.Properties['Source'] -and $cmd.Source) { $src = $cmd.Source }
        if ($src) { [void]$cands.Add($src) }
    }

    foreach ($root in $Spec.SearchRoots) {
        $r = Expand-PathVars $root
        if ($r -and (Test-Path -LiteralPath $r)) {
            $hit = Get-ChildItem -LiteralPath $r -Filter $Spec.ExeName -Recurse -Depth 2 -File -ErrorAction SilentlyContinue |
                Select-Object -First 1
            if ($hit) { [void]$cands.Add($hit.FullName) }
        }
    }

    foreach ($c in $cands) {
        if ($c -and (Test-Path -LiteralPath $c -PathType Leaf)) {
            try { return (Resolve-Path -LiteralPath $c).Path } catch { return $c }
        }
    }
    return $null
}

# ============================================================================
# 端到端测试图
# ============================================================================

function New-TestImage {
    param([string]$EngineId)

    $dir = Join-Path ([System.IO.Path]::GetTempPath()) 'autotrigger-ocr-selftest'
    if (-not (Test-Path -LiteralPath $dir)) { [void](New-Item -ItemType Directory -Path $dir -Force) }
    $imgPath = Join-Path $dir ("ocr_selftest_{0}.png" -f $EngineId)

    Add-Type -AssemblyName System.Drawing -ErrorAction Stop
    $bmp = New-Object System.Drawing.Bitmap 960, 360
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.Clear([System.Drawing.Color]::White)
    $g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit

    $cnFont = New-Object System.Drawing.Font('Microsoft YaHei', 26)
    $enFont = New-Object System.Drawing.Font('Segoe UI', 26)
    $black = [System.Drawing.Brushes]::Black
    $texts = @(
        @{ t = '下一步'; f = $cnFont; x = 40; y = 30 },
        @{ t = '上一步'; f = $cnFont; x = 300; y = 30 },
        @{ t = '取消'; f = $cnFont; x = 560; y = 30 },
        @{ t = '安装'; f = $cnFont; x = 40; y = 130 },
        @{ t = '浏览'; f = $cnFont; x = 300; y = 130 },
        @{ t = '我接受'; f = $cnFont; x = 560; y = 130 },
        @{ t = 'Next'; f = $enFont; x = 40; y = 240 },
        @{ t = 'Install'; f = $enFont; x = 300; y = 240 },
        @{ t = 'I Agree'; f = $enFont; x = 560; y = 240 }
    )
    foreach ($item in $texts) { $g.DrawString($item.t, $item.f, $black, $item.x, $item.y) }

    # 画一个"按钮"外框，模拟真实安装向导中的点击目标
    $pen = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(120, 120, 120), 2)
    $g.DrawRectangle($pen, 285, 115, 150, 60)

    $g.Dispose(); $cnFont.Dispose(); $enFont.Dispose(); $pen.Dispose()
    $bmp.Save($imgPath, [System.Drawing.Imaging.ImageFormat]::Png)
    $bmp.Dispose()
    return $imgPath
}

# ============================================================================
# [2] 各引擎验证
# ============================================================================

function New-EngineResult {
    param([string]$Id, [string]$Name, [string]$Path, [bool]$Found)
    return [pscustomobject]@{
        engine = $Id; name = $Name; path = $Path; found = $Found; available = $false
        version = $null; transports = (New-Object System.Collections.ArrayList); init_ms = $null; elapsed_ms = $null
        languages = @(); coords = $null; native_score = $null
        texts = (New-Object System.Collections.ArrayList); words = (New-Object System.Collections.ArrayList); raw_code = $null; summary = ''; issues = @()
    }
}

# PaddleOCR-json / RapidOCR-json：CLI 一次性调用 + stdio 管道模式
function Test-PaddleLikeEngine {
    param($Spec, [string]$Exe, [string]$ImagePath, [int]$TimeoutSec)

    $r = New-EngineResult -Id $Spec.Id -Name $Spec.Name -Path $Exe -Found ([bool]$Exe)
    if (-not $Exe) { $r.summary = '未发现'; return $r }

    $issues = New-Object System.Collections.ArrayList
    $imageOption = if ($Spec.Id -eq 'rapidocr_json') { '--image_path=' } else { '-image_path=' }
    $argText = $imageOption + (Quote-Arg $ImagePath)

    Write-Report ("  [CLI 模式] {0} {1}" -f $Spec.ExeName, $argText) 'DarkGray'
    $cli = Invoke-Proc -Exe $Exe -ArgText $argText -TimeoutSec $TimeoutSec
    $r.elapsed_ms = $cli.ElapsedMs
    $r.version = Get-VersionHint ($cli.StdErr + "`n" + $cli.StdOut)

    if ($cli.TimedOut) {
        [void]$issues.Add(("CLI 模式超时（>{0}s）未返回结果" -f $TimeoutSec))
    } elseif (-not $cli.Ok) {
        [void]$issues.Add(("CLI 模式启动失败：{0}" -f $cli.Error))
    } elseif ($cli.ExitCode -ne 0) {
        [void]$issues.Add(("CLI 模式退出码非 0：{0}" -f $cli.ExitCode))
    }

    $json = Parse-PaddleJson $cli.StdOut
    if ($json) {
        $r.raw_code = $json.code
        if ($json.code -eq 100 -or $json.code -eq 101) {
            [void]$r.transports.Add('cli')
            $r.coords = 'quad'
            $r.native_score = $true
            $r.languages = @('ch', 'en')
            if ($json.code -eq 100 -and $json.data) {
                foreach ($d in $json.data) {
                    if ($null -eq $d) { continue }
                    if ($d.text) { [void]$r.texts.Add([string]$d.text) }
                    $box = $d.box
                    if ($box -and $box.Count -ge 4) {
                        $x = [int](([double]$box[0][0] + [double]$box[2][0]) / 2)
                        $y = [int](([double]$box[0][1] + [double]$box[2][1]) / 2)
                        [void]$r.words.Add([pscustomobject]@{
                                text = [string]$d.text; center_x = $x; center_y = $y; score = [double]$d.score
                            })
                    }
                }
            } else {
                [void]$issues.Add('引擎可用，但未识别出任何文本（code=101）')
            }
            $r.available = $true
        } else {
            [void]$issues.Add(("引擎返回错误码 code={0}（100=成功 / 101=未识别到文字）" -f $json.code))
        }
    } else {
        $snippet = ($cli.StdOut + ' | ' + $cli.StdErr).Trim()
        if ($snippet.Length -gt 160) { $snippet = $snippet.Substring(0, 160) + '...' }
        [void]$issues.Add(("未解析到合法 JSON 结果；原始输出：{0}" -f $snippet))
        if ($Spec.NeedAvx -and (Test-CpuAvxSupported) -eq $false) {
            [void]$issues.Add('该引擎需要 CPU 支持 AVX：本机 AVX 探测为不支持，请改用 rapidocr_json')
        }
    }

    # stdio 管道模式（F14.6 的默认调用方式）
    if ($r.available) {
        $req = (@{ image_path = $ImagePath } | ConvertTo-Json -Compress)
        Write-Report '  [stdio 模式] 无参启动 + 写入一行 JSON 请求' 'DarkGray'
        $pipe = Invoke-Proc -Exe $Exe -StdinLines @($req) -TimeoutSec $TimeoutSec
        $pjson = Parse-PaddleJson $pipe.StdOut
        if ($pjson -and ($pjson.code -eq 100 -or $pjson.code -eq 101)) {
            [void]$r.transports.Add('stdio')
        } else {
            [void]$issues.Add('stdio 管道模式未返回合法结果（该版本可能仅支持 -image_path / -port 方式；工具侧可改用 TCP 模式）')
        }
    }

    $r.issues = $issues.ToArray()
    $r.summary = $(if ($r.available) { '可用' } else { '不可用' })
    return $r
}

function Test-CpuAvxSupported {
    try {
        if (-not ('Win32.NativeMethods' -as [type])) { return $null }
        return [Win32.NativeMethods]::IsProcessorFeaturePresent(39)
    } catch { return $null }
}

# Tesseract：--version / --list-langs / TSV 输出
function Test-TesseractEngine {
    param($Spec, [string]$Exe, [string]$ImagePath, [int]$TimeoutSec)

    $r = New-EngineResult -Id $Spec.Id -Name $Spec.Name -Path $Exe -Found ([bool]$Exe)
    if (-not $Exe) { $r.summary = '未发现'; return $r }
    $issues = New-Object System.Collections.ArrayList

    $ver = Invoke-Proc -Exe $Exe -ArgText '--version' -TimeoutSec $TimeoutSec
    $r.version = Get-VersionHint ($ver.StdErr + "`n" + $ver.StdOut)

    $langs = Invoke-Proc -Exe $Exe -ArgText '--list-langs' -TimeoutSec $TimeoutSec
    $langOut = ($langs.StdOut + "`n" + $langs.StdErr)
    $available = New-Object System.Collections.ArrayList
    foreach ($line in ($langOut -split "`r?`n")) {
        $t = $line.Trim()
        if ($t -and $t -notmatch '^(List of available|Only the first)') { [void]$available.Add($t) }
    }
    $r.languages = $available.ToArray()

    if (-not $r.languages -or $r.languages.Count -eq 0) {
        [void]$issues.Add('未能列出语言包（tessdata 缺失？）')
    }

    $useLang = 'eng'
    if ($r.languages -contains 'chi_sim') { $useLang = 'chi_sim+eng' }

    $argText = (Quote-Arg $ImagePath) + ' stdout -l ' + $useLang + ' tsv'
    Write-Report ("  [TSV 模式] tesseract {0}" -f $argText) 'DarkGray'
    $run = Invoke-Proc -Exe $Exe -ArgText $argText -TimeoutSec $TimeoutSec
    $r.elapsed_ms = $run.ElapsedMs

    if ($run.TimedOut) { [void]$issues.Add(("识别超时（>{0}s）" -f $TimeoutSec)) }

    $words = New-Object System.Collections.ArrayList
    $texts = New-Object System.Collections.ArrayList
    foreach ($line in ($run.StdOut -split "`r?`n")) {
        $f = $line -split "`t"
        if ($f.Count -ge 12 -and $f[0] -eq '5') {
            $left = 0; $top = 0; $w = 0; $h = 0; $conf = 0.0
            [void][int]::TryParse($f[6], [ref]$left)
            [void][int]::TryParse($f[7], [ref]$top)
            [void][int]::TryParse($f[8], [ref]$w)
            [void][int]::TryParse($f[9], [ref]$h)
            [void][double]::TryParse($f[10], [ref]$conf)
            $txt = $f[11]
            if ($conf -gt 0 -and $txt) {
                [void]$texts.Add($txt)
                [void]$words.Add([pscustomobject]@{
                        text = $txt; center_x = [int]($left + $w / 2); center_y = [int]($top + $h / 2)
                        score = [math]::Round($conf / 100, 4)
                    })
            }
        }
    }
    $r.texts = $texts.ToArray()
    $r.words = $words.ToArray()

    if ($texts.Count -gt 0) {
        $r.available = $true
        [void]$r.transports.Add('cli')
        $r.coords = 'bbox'
        $r.native_score = $true
    } else {
        [void]$issues.Add('TSV 未解析出任何词（可能是识别失败或输出格式异常）')
    }

    $r.issues = $issues.ToArray()
    $r.summary = $(if ($r.available) { '可用' } else { '不可用' })
    return $r
}

# Umi-OCR：HTTP 接口
function Test-UmiOcrEngine {
    param($Spec, [string]$Exe, [string]$ImagePath, [string]$ApiUrl, [int]$TimeoutSec)

    $r = New-EngineResult -Id $Spec.Id -Name $Spec.Name -Path $Exe -Found ([bool]$Exe)
    $issues = New-Object System.Collections.ArrayList
    $uri = $null
    try { $uri = [System.Uri]$ApiUrl } catch { }
    $port = 1224
    if ($uri) { $port = $uri.Port }

    Write-Report ("  [HTTP 模式] {0}" -f $ApiUrl) 'DarkGray'
    if (-not (Test-TcpPort -Host '127.0.0.1' -Port $port)) {
        [void]$issues.Add(("HTTP 接口不可达（127.0.0.1:{0} 未监听）——Umi-OCR 需先启动并开启 HTTP 服务" -f $port))
        $r.issues = $issues.ToArray()
        $r.summary = '接口不可达'
        return $r
    }

    try {
        $b64 = [Convert]::ToBase64String([System.IO.File]::ReadAllBytes($ImagePath))
        $body = (@{ base64 = $b64 } | ConvertTo-Json -Compress)
        $sw = [System.Diagnostics.Stopwatch]::StartNew()
        $resp = Invoke-RestMethod -Uri $ApiUrl -Method Post -ContentType 'application/json' -Body $body -TimeoutSec $TimeoutSec
        $sw.Stop()
        $r.elapsed_ms = [int]$sw.ElapsedMilliseconds
        if ($resp.code -eq 100) {
            $r.available = $true
            $r.raw_code = 100
            $r.coords = 'quad'
            $r.native_score = $true
            [void]$r.transports.Add('http')
            $texts = New-Object System.Collections.ArrayList
            $words = New-Object System.Collections.ArrayList
            foreach ($d in $resp.data) {
                if ($null -eq $d) { continue }
                if ($d.text) { [void]$texts.Add([string]$d.text) }
                $box = $d.box
                if ($box -and $box.Count -ge 4) {
                    $x = [int](([double]$box[0][0] + [double]$box[2][0]) / 2)
                    $y = [int](([double]$box[0][1] + [double]$box[2][1]) / 2)
                    [void]$words.Add([pscustomobject]@{
                            text = [string]$d.text; center_x = $x; center_y = $y; score = [double]$d.score
                        })
                }
            }
            $r.texts = $texts.ToArray()
            $r.words = $words.ToArray()
        } else {
            [void]$issues.Add(("HTTP 接口返回 code={0}" -f $resp.code))
        }
    } catch {
        [void]$issues.Add(("HTTP 调用失败：{0}" -f $_.Exception.Message))
    }

    $r.issues = $issues.ToArray()
    $r.summary = $(if ($r.available) { '可用' } else { '不可用' })
    return $r
}

# Windows 内置 OCR（可选回退后端，仅 PS 5.1，默认关闭）
function Test-WindowsBuiltinOcr {
    param([string]$ImagePath)

    $r = New-EngineResult -Id 'windows' -Name 'Windows 内置 OCR' -Path 'Windows.Media.Ocr' -Found $true
    $issues = New-Object System.Collections.ArrayList

    if ($PSVersionTable.PSEdition -eq 'Core') {
        [void]$issues.Add('内置 OCR 需 Windows PowerShell 5.1（WinRT 类型投影），当前为 PowerShell 7+')
        $r.issues = $issues.ToArray(); $r.summary = '不适用'
        return $r
    }

    try {
        [Windows.Media.Ocr.OcrEngine, Windows.Media.Ocr, ContentType = WindowsRuntime] | Out-Null
        [Windows.Globalization.Language, Windows.Globalization, ContentType = WindowsRuntime] | Out-Null
        $langs = [Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages
        $tags = New-Object System.Collections.ArrayList
        foreach ($l in $langs) { [void]$tags.Add([string]$l.LanguageTag) }
        $r.languages = $tags.ToArray()
        if ($tags.Count -eq 0) {
            [void]$issues.Add('未安装任何 OCR 语言包，内置 OCR 不可用')
        } else {
            Write-Report ("  可用语言: {0}" -f ($tags -join ', ')) 'DarkGray'
            foreach ($tag in @('zh-Hans-CN', 'en-US')) {
                $ok = [Windows.Media.Ocr.OcrEngine]::IsLanguageSupported([Windows.Globalization.Language]::new($tag))
                Write-Report ("  IsLanguageSupported({0}) = {1}" -f $tag, $ok) $(if ($ok) { 'Green' } else { 'Yellow' })
            }
        }
    } catch {
        [void]$issues.Add(("内置 OCR 引擎探测失败：{0}" -f $_.Exception.Message))
    }

    if ($r.languages.Count -gt 0) {
        try {
            Add-Type -AssemblyName System.Runtime.WindowsRuntime -ErrorAction Stop
            $asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
                    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and
                    $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
                })[0]
            function Await($op, $type) {
                $t = $asTaskGeneric.MakeGenericMethod($type).Invoke($null, @($op))
                $t.Wait(-1) | Out-Null
                $t.Result
            }
            [Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime] | Out-Null
            [Windows.Storage.Streams.IRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime] | Out-Null
            [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType = WindowsRuntime] | Out-Null
            [Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics.Imaging, ContentType = WindowsRuntime] | Out-Null

            $eng = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
            if ($null -eq $eng) {
                $langObj = [Windows.Globalization.Language]::new([string]$r.languages[0])
                $eng = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($langObj)
            }
            if ($null -eq $eng) {
                [void]$issues.Add('无法创建 OCR 引擎实例')
            } else {
                $file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($ImagePath)) ([Windows.Storage.StorageFile])
                $stream = Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
                $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
                $sbitmap = Await ($decoder.GetSoftwareBitmapAsync(
                        [Windows.Graphics.Imaging.BitmapPixelFormat]::Bgra8,
                        [Windows.Graphics.Imaging.BitmapAlphaMode]::Premultiplied)) ([Windows.Graphics.Imaging.SoftwareBitmap])
                $result = Await ($eng.RecognizeAsync($sbitmap)) ([Windows.Media.Ocr.OcrResult])

                $texts = New-Object System.Collections.ArrayList
                $words = New-Object System.Collections.ArrayList
                foreach ($line in $result.Lines) {
                    foreach ($w in $line.Words) {
                        $rect = $w.BoundingRect
                        [void]$texts.Add([string]$w.Text)
                        [void]$words.Add([pscustomobject]@{
                                text = [string]$w.Text
                                center_x = [int]($rect.X + $rect.Width / 2)
                                center_y = [int]($rect.Y + $rect.Height / 2)
                                score = $null
                            })
                    }
                }
                $r.texts = $texts.ToArray()
                $r.words = $words.ToArray()
                $r.coords = 'bbox'
                $r.native_score = $false
                $r.available = ($texts.Count -gt 0)
                if ($r.available) { [void]$r.transports.Add('builtin') }
                else { [void]$issues.Add('识别结果为空') }
            }
        } catch {
            [void]$issues.Add(("端到端识别失败：{0}" -f $_.Exception.Message))
        }
    }

    $r.issues = $issues.ToArray()
    $r.summary = $(if ($r.available) { '可用（可选回退）' } else { '不可用' })
    return $r
}

# ============================================================================
# 主流程
# ============================================================================

$host_info = Get-HostInfo
$cpu_info = Get-CpuInfo

Write-Report '=================================================='
Write-Report ' OCR 引擎可用性自检（AutoTrigger）'
Write-Report '=================================================='
Write-Report ("时间      : {0}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))
Write-Report ("主机      : {0}  [{1}]" -f $host_info.os, $host_info.arch)
Write-Report ("PowerShell: {0} ({1})" -f $host_info.ps_version, $host_info.ps_edition)
Write-Report ("CPU       : {0}" -f $(if ($cpu_info.name) { $cpu_info.name } else { '未知' }))
$avxText = '未知'
if ($cpu_info.avx -eq $true) { $avxText = '支持' } elseif ($cpu_info.avx -eq $false) { $avxText = '不支持' }
$avx2Text = '未知'
if ($cpu_info.avx2 -eq $true) { $avx2Text = '支持' } elseif ($cpu_info.avx2 -eq $false) { $avx2Text = '不支持' }
Write-Report ("AVX / AVX2: {0} / {1}" -f $avxText, $avx2Text)
if ($cpu_info.note) { Write-Report ("            {0}" -f $cpu_info.note) 'Yellow' }
if ($cpu_info.avx -eq $false) {
    Write-Report '  注意：本机不支持 AVX，PaddleOCR-json 无法运行，请使用 rapidocr_json。' 'Yellow'
    Add-Issue 'CPU 不支持 AVX：默认引擎 paddleocr_json 不可用，建议 ocr.engine = "rapidocr_json"' 'WARN'
}

# ---------- 配置 ----------
Write-Section '[1] 配置与引擎发现'
$cfg = Read-ConfigOcr -Path $ConfigPath
if ($cfg) {
    Write-Report ("  config.json : {0}" -f $cfg.path)
    Write-Report ("  ocr.engine  : {0}" -f $(if ($cfg.engine) { $cfg.engine } else { '(未设置，用内嵌默认 paddleocr_json)' }))
    Write-Report ("  ocr.engine_path: {0}" -f $(if ($cfg.engine_path) { $cfg.engine_path } else { '(未设置，走 PATH / 自动发现)' }))
} else {
    Write-Report '  未找到 config.json（按内嵌默认：paddleocr_json + 自动发现）' 'DarkGray'
}

if ($Engine -eq 'none' -or ($cfg -and $cfg.engine -eq 'none' -and $Engine -eq 'auto')) {
    Write-Report ''
    Write-Report 'OCR 已配置为关闭（engine = "none"）：工具将仅使用 UIA + 图像模板。' 'Yellow'
    $result = [pscustomobject]@{
        ok = $true; checked_at = (Get-Date).ToString('o')
        host = $host_info; cpu = $cpu_info; engines = @()
        recommended = [pscustomobject]@{ engine = 'none'; engine_path = $null }
        issues = $script:Issues.ToArray()
    }
    if ($Json) { Write-Output ($result | ConvertTo-Json -Depth 8) }
    exit 0
}

# 目标引擎集合
$planned = New-Object System.Collections.ArrayList
if ($Engine -eq 'auto') {
    foreach ($spec in $EngineCatalog) { [void]$planned.Add($spec) }
    if ($IncludeWindowsBuiltin) {
        [void]$planned.Add([pscustomobject]@{
                Id = 'windows'; Name = 'Windows 内置 OCR'; ExeName = ''; NeedAvx = $false
                StandalonePaths = @(); SearchRoots = @(); Hint = '可选回退后端（默认关闭）'
            })
    }
} else {
    $spec = $EngineCatalog | Where-Object { $_.Id -eq $Engine } | Select-Object -First 1
    if ($spec) {
        [void]$planned.Add($spec)
    } else {
        [void]$planned.Add([pscustomobject]@{
                Id = 'windows'; Name = 'Windows 内置 OCR'; ExeName = ''; NeedAvx = $false
                StandalonePaths = @(); SearchRoots = @(); Hint = '可选回退后端（默认关闭）'
            })
    }
}

# ---------- 测试图 ----------
Write-Section '[2] 测试图'
$imgPath = $null
$tempImage = $false
if ($TestImage) {
    if (Test-Path -LiteralPath $TestImage -PathType Leaf) {
        $imgPath = (Resolve-Path -LiteralPath $TestImage).Path
        Write-Report ("  使用指定图片: {0}" -f $imgPath)
    } else {
        Write-Report ("  指定图片不存在: {0}" -f $TestImage) 'Red'
        Add-Issue ("-TestImage 指定的文件不存在：{0}" -f $TestImage) 'ERROR'
    }
}
if (-not $imgPath) {
    try {
        $imgPath = New-TestImage -EngineId 'shared'
        $tempImage = $true
        Write-Report ("  已生成中英混排测试图: {0}" -f $imgPath)
        Write-Report '  内容: 下一步 / 上一步 / 取消 / 安装 / 浏览 / 我接受 / Next / Install / I Agree' 'DarkGray'
    } catch {
        Write-Report ("  测试图生成失败: {0}" -f $_.Exception.Message) 'Red'
        Add-Issue ("测试图生成失败（无法完成端到端验证）：{0}" -f $_.Exception.Message) 'ERROR'
    }
}

# ---------- 逐引擎验证 ----------
Write-Section '[3] 引擎验证'
$results = New-Object System.Collections.ArrayList
foreach ($spec in $planned) {
    Write-Report ''
    Write-Report ("--- {0} ({1}) ---" -f $spec.Name, $spec.Id) 'White'

    if ($spec.Id -eq 'windows') {
        if ($imgPath) {
            $res = Test-WindowsBuiltinOcr -ImagePath $imgPath
        } else {
            $res = New-EngineResult -Id 'windows' -Name $spec.Name -Path 'Windows.Media.Ocr' -Found $true
            $res.summary = '跳过（无测试图）'
        }
        [void]$results.Add($res)
        Write-Report ("  结论: {0}" -f $res.summary) $(if ($res.available) { 'Green' } else { 'Yellow' })
        continue
    }

    $exe = Find-EngineExe -Spec $spec -ExplicitPath $(if ($EnginePath -and $Engine -ne 'auto') { $EnginePath } else { $EnginePath }) -ConfigEnginePath $(if ($cfg -and $cfg.engine -eq $spec.Id) { $cfg.engine_path } else { $null })

    if (-not $exe) {
        $res = New-EngineResult -Id $spec.Id -Name $spec.Name -Path $null -Found $false
        $res.summary = '未发现（未安装或不在 PATH / 常见路径中）'
        [void]$results.Add($res)
        Write-Report ("  结论: {0}" -f $res.summary) 'DarkGray'
        Write-Report ("  说明: {0}" -f $spec.Hint) 'DarkGray'
        continue
    }

    Write-Report ("  路径: {0}" -f $exe) 'Green'
    if ($spec.NeedAvx -and $cpu_info.avx -eq $false) {
        Write-Report '  警告: 该引擎需要 AVX，本机不支持，跳过实际启动。' 'Yellow'
        $res = New-EngineResult -Id $spec.Id -Name $spec.Name -Path $exe -Found $true
        $res.summary = '跳过（CPU 不支持 AVX）'
        $res.issues = @('CPU 不支持 AVX')
        [void]$results.Add($res)
        continue
    }

    if (-not $imgPath) {
        $res = New-EngineResult -Id $spec.Id -Name $spec.Name -Path $exe -Found $true
        $res.summary = '跳过（无测试图）'
        [void]$results.Add($res)
        continue
    }

    switch ($spec.Id) {
        'umi_ocr' { $res = Test-UmiOcrEngine -Spec $spec -Exe $exe -ImagePath $imgPath -ApiUrl $UmiApiUrl -TimeoutSec $TimeoutSec }
        'tesseract' { $res = Test-TesseractEngine -Spec $spec -Exe $exe -ImagePath $imgPath -TimeoutSec $TimeoutSec }
        default { $res = Test-PaddleLikeEngine -Spec $spec -Exe $exe -ImagePath $imgPath -TimeoutSec $TimeoutSec }
    }
    [void]$results.Add($res)

    if ($res.version) { Write-Report ("  版本: {0}" -f $res.version) 'DarkGray' }
    if ($res.elapsed_ms) { Write-Report ("  耗时: {0} ms" -f $res.elapsed_ms) 'DarkGray' }
    if ($res.languages -and $res.languages.Count -gt 0) { Write-Report ("  语言: {0}" -f (($res.languages | Select-Object -First 8) -join ', ')) 'DarkGray' }
    if ($res.transports -and $res.transports.Count -gt 0) { Write-Report ("  可用调用方式: {0}" -f ($res.transports -join ', ')) 'DarkGray' }

    if ($res.words -and $res.words.Count -gt 0) {
        Write-Report '  识别结果（文本 / 中心像素坐标 / score）:' 'DarkGray'
        foreach ($w in $res.words) {
            $sc = $(if ($null -ne $w.score) { ('{0:N4}' -f $w.score) } else { 'n/a' })
            Write-Report ("    {0,-10} 中心=({1,4},{2,4})  score={3}" -f $w.text, $w.center_x, $w.center_y, $sc) 'DarkGray'
        }
    }
    foreach ($iss in $res.issues) { Write-Report ("  ! {0}" -f $iss) 'Yellow' }
    Write-Report ("  结论: {0}" -f $res.summary) $(if ($res.available) { 'Green' } else { 'Yellow' })
}

# ---------- 汇总 ----------
Write-Section '[4] 汇总'
$usable = @($results | Where-Object { $_.available })

foreach ($r in $results) {
    $mark = $(if ($r.available) { '[OK]  ' } elseif ($r.found) { '[FAIL]' } else { '[MISS]' })
    $color = $(if ($r.available) { 'Green' } elseif ($r.found) { 'Yellow' } else { 'DarkGray' })
    Write-Report ("  {0} {1,-14} {2}" -f $mark, $r.engine, $r.summary) $color
}

$recommended = $null
foreach ($pref in @('paddleocr_json', 'rapidocr_json', 'umi_ocr', 'tesseract')) {
    $hit = $usable | Where-Object { $_.engine -eq $pref } | Select-Object -First 1
    if ($hit) { $recommended = $hit; break }
}
if (-not $recommended -and ($usable | Where-Object { $_.engine -eq 'windows' })) {
    $recommended = $usable | Where-Object { $_.engine -eq 'windows' } | Select-Object -First 1
}

$exitCode = 5
if ($usable.Count -gt 0) {
    $exitCode = 0
    Write-Report ''
    Write-Report ("  推荐引擎: {0}（{1}）" -f $recommended.engine, $recommended.path) 'Green'
    $frag = [pscustomobject]@{
        ocr = [pscustomobject]@{
            engine = $recommended.engine
            engine_path = $recommended.path
            transport = $(if ($recommended.transports -contains 'stdio') { 'stdio' } elseif ($recommended.transports -contains 'http') { 'http' } else { 'stdio' })
            min_score = $(if ($cfg -and $null -ne $cfg.min_score) { $cfg.min_score } else { 0.6 })
        }
    }
    Write-Report '  建议写入 config.json 的片段:' 'Cyan'
    foreach ($line in ($frag | ConvertTo-Json -Depth 4) -split "`r?`n") { Write-Report ("    {0}" -f $line) 'DarkGray' }
} else {
    Write-Report ''
    Write-Report '  未发现任何可用 OCR 引擎。' 'Red'
    Write-Report '  解决方式（离线环境，任选其一）:' 'Yellow'
    Write-Report '    0) 使用交付包内的引擎包（推荐）: third_party\ocr\PaddleOCR-json_v<版本>.7z' 'Yellow'
    Write-Report '       解压到任意目录（绿色免安装），再设置 config.json 的 ocr.engine_path' 'Yellow'
    Write-Report '    1) 将解压目录加入 PATH' 'Yellow'
    Write-Report '    2) 若 CPU 不支持 AVX: 改用 RapidOCR-json 并设置 ocr.engine = "rapidocr_json"' 'Yellow'
    Write-Report '    临时绕过（强制仅用 UIA + 模板）: AutoTrigger.exe --no-ocr ...' 'Yellow'
}

if (-not $KeepTestImage -and $tempImage -and $imgPath -and (Test-Path -LiteralPath $imgPath)) {
    try { Remove-Item -LiteralPath $imgPath -Force -ErrorAction SilentlyContinue } catch { }
}

Write-Report ''
Write-Report ("退出码: {0}（0=至少一个引擎可用；5=无可用引擎，与工具本体 --check-prereq 对齐）" -f $exitCode) 'Cyan'

if ($Json) {
    $out = [pscustomobject]@{
        ok = ($exitCode -eq 0)
        checked_at = (Get-Date).ToString('o')
        host = $host_info
        cpu = $cpu_info
        config = $cfg
        engines = $results
        recommended = $(
            if ($recommended) {
                [pscustomobject]@{
                    engine = $recommended.engine
                    engine_path = $recommended.path
                    transports = $recommended.transports
                    version = $recommended.version
                }
            } else { $null }
        )
        issues = $script:Issues.ToArray()
    }
    try { Write-Output ($out | ConvertTo-Json -Depth 8) } catch {
        Write-Output '{"ok":false,"issues":[{"level":"ERROR","text":"JSON serialization failed"}]}'
    }
}

exit $exitCode
