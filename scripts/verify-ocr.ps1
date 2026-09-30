<#
.SYNOPSIS
    验证 Windows 系统内置 OCR（Windows.Media.Ocr）的可用性与语言包支持情况。

.DESCRIPTION
    依次执行三项检查：
      [1] 查询系统实际可识别的 OCR 语言（引擎级探测，最权威判据）
      [2] 列出 C:\Windows\OCR 下已安装的语言资源目录
      [3] 生成中英混排测试图，实际调用 OCR 识别，输出文本与像素包围盒

    第 [3] 步通过即说明内置 OCR 可直接用于 GUI 自动化辅助定位（按文本包围盒
    推算可点击像素坐标）。

.NOTES
    必须使用 Windows PowerShell 5.1（powershell.exe）运行。
    不支持 PowerShell 7+（其缺少 WinRT 类型投影所需的那套语法）。
    纯只读检查，不修改系统状态，仅向 %TEMP% 写入一张测试图与探测日志。
#>

$ErrorActionPreference = 'Stop'

function Write-Section([string]$text) {
    Write-Host ''
    Write-Host $text -ForegroundColor Cyan
}

Write-Host '=== Windows 内置 OCR 自检 ===' -ForegroundColor Cyan
Write-Host ("时间: {0}" -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'))

# ---------- [1] 引擎级语言探测 ----------
Write-Section '[1] 可用 OCR 识别语言（引擎级探测）'

$langs = $null
try {
    [Windows.Media.Ocr.OcrEngine, Windows.Media.Ocr, ContentType = WindowsRuntime] | Out-Null
    [Windows.Globalization.Language, Windows.Globalization, ContentType = WindowsRuntime] | Out-Null
    $langs = [Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages
} catch {
    Write-Host ("  探测失败: {0}" -f $_.Exception.Message) -ForegroundColor Red
}

if ($null -ne $langs) {
    if ($langs.Count -eq 0) {
        Write-Host '  未检测到任何 OCR 语言包 —— 内置 OCR 不可用' -ForegroundColor Red
    } else {
        foreach ($l in $langs) {
            Write-Host ("  - {0}   {1}" -f $l.LanguageTag, $l.DisplayName) -ForegroundColor Green
        }
        Write-Host ("  合计 {0} 种语言。注意：Windows OCR 为单语言引擎，一个引擎实例只按一种语言识别。" -f $langs.Count)
    }
    Write-Host ("  MaxImageDimension = {0}" -f [Windows.Media.Ocr.OcrEngine]::MaxImageDimension)

    foreach ($tag in @('zh-Hans-CN', 'en-US')) {
        $ok = [Windows.Media.Ocr.OcrEngine]::IsLanguageSupported([Windows.Globalization.Language]::new($tag))
        $color = if ($ok) { 'Green' } else { 'Yellow' }
        Write-Host ("  IsLanguageSupported({0}) = {1}" -f $tag, $ok) -ForegroundColor $color
    }
}

# ---------- [2] 资源目录 ----------
Write-Section '[2] 系统 OCR 资源目录 C:\Windows\OCR'
$ocrDirs = Get-ChildItem 'C:\Windows\OCR' -Directory -ErrorAction SilentlyContinue
if ($ocrDirs) {
    $ocrDirs | ForEach-Object { Write-Host ("  - {0}" -f $_.Name) }
} else {
    Write-Host '  目录不存在或不可读（可能使用了非标准路径，以 [1] 的结果为准）' -ForegroundColor Yellow
}

# ---------- [3] 端到端识别 ----------
Write-Section '[3] 端到端识别测试'

if ($null -eq $langs -or $langs.Count -eq 0) {
    Write-Host '  跳过：无可用 OCR 语言。' -ForegroundColor Yellow
    return
}

Add-Type -AssemblyName System.Runtime.WindowsRuntime
Add-Type -AssemblyName System.Drawing

$asTaskGeneric = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
        $_.Name -eq 'AsTask' -and
        $_.GetParameters().Count -eq 1 -and
        $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
    })[0]

function Await($winRtOperation, $resultType) {
    $task = $asTaskGeneric.MakeGenericMethod($resultType).Invoke($null, @($winRtOperation))
    $task.Wait(-1) | Out-Null
    $task.Result
}

$imgPath = Join-Path $env:TEMP 'ocr_selftest.png'
$bmp = New-Object System.Drawing.Bitmap 760, 280
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.Clear([System.Drawing.Color]::White)
$g.TextRenderingHint = [System.Drawing.Text.TextRenderingHint]::AntiAliasGridFit
$font = New-Object System.Drawing.Font('Microsoft YaHei', 24)
$g.DrawString('下一步', $font, [System.Drawing.Brushes]::Black, 40, 40)
$g.DrawString('安装', $font, [System.Drawing.Brushes]::Black, 40, 130)
$g.DrawString('Install', $font, [System.Drawing.Brushes]::Black, 340, 130)
$g.Dispose()
$bmp.Save($imgPath, [System.Drawing.Imaging.ImageFormat]::Png)
$bmp.Dispose()
Write-Host ("  测试图: {0}" -f $imgPath)

[Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime] | Out-Null
[Windows.Storage.Streams.IRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime] | Out-Null
[Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType = WindowsRuntime] | Out-Null
[Windows.Graphics.Imaging.SoftwareBitmap, Windows.Graphics.Imaging, ContentType = WindowsRuntime] | Out-Null
[Windows.Media.Ocr.OcrResult, Windows.Media.Ocr, ContentType = WindowsRuntime] | Out-Null

$engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
if ($null -eq $engine) { $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($langs[0]) }
if ($null -eq $engine) {
    Write-Host '  无法创建 OCR 引擎：' -ForegroundColor Red
    return
}
Write-Host ("  引擎语言: {0}" -f $engine.RecognizerLanguage.LanguageTag)

$file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($imgPath)) ([Windows.Storage.StorageFile])
$stream = Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
$decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
$softwareBitmap = Await ($decoder.GetSoftwareBitmapAsync(
        [Windows.Graphics.Imaging.BitmapPixelFormat]::Bgra8,
        [Windows.Graphics.Imaging.BitmapAlphaMode]::Premultiplied)) ([Windows.Graphics.Imaging.SoftwareBitmap])
$result = Await ($engine.RecognizeAsync($softwareBitmap)) ([Windows.Media.Ocr.OcrResult])

Write-Host ''
Write-Host '  识别文本:'
Write-Host ("    {0}" -f ($result.Text -replace "`r?`n", ' | '))
Write-Host ''
Write-Host '  词 + 像素包围盒（可直接作为点击坐标来源）:'
foreach ($line in $result.Lines) {
    foreach ($w in $line.Words) {
        $r = $w.BoundingRect
        Write-Host ("    {0,-10} 包围盒=({1},{2} {3}x{4})   中心=({5},{6})" -f `
                $w.Text, [int]$r.X, [int]$r.Y, [int]$r.Width, [int]$r.Height, `
            [int]($r.X + $r.Width / 2), [int]($r.Y + $r.Height / 2))
    }
}

Write-Section '自检完成'
Write-Host '判读要点：' -ForegroundColor Cyan
Write-Host '  - [1] 列出的语言才是真正可用于识别的语言；未列出的语言即使镜像里有目录也不生效。'
Write-Host '  - [3] 若中文（下一步/安装）能识别并给出包围盒，说明内置 OCR 可用于 GUI 辅助定位。'
Write-Host '  - [3] 若 Install 未被识别，说明当前引擎对拉丁字母支持有限，英文安装器界面应优先走 UIA + 模板。'
