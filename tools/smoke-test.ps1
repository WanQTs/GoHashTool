# Smoke test: launch the built exe, measure time-to-window, capture a screenshot, close.
param(
    [string]$ExePath = "bin\gohash.exe",
    [string]$ShotPath = "docs\screenshot-main.png"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location (Split-Path -Parent $root)  # repo root

# 0. exe 功能自检：--selftest 不开 GUI 跑核心校验（已知值/SUM 闭环/清单解析），退出码非 0 即失败。
# 注：GUI 子系统进程经 ShellExecute 启动时 ExitCode 偶发为空；这里显式 UseShellExecute=$false
# （CreateProcess 路径）并将路径解析为绝对路径，保证退出码可读（v3 构建下原写法会拿到空值）。
$exeFull = Join-Path (Get-Location) $ExePath
if (-not (Test-Path $exeFull)) { Write-Output "EXE_NOT_FOUND: $exeFull"; exit 1 }
Write-Output "SELFTEST_EXE=$exeFull"
$psi = [System.Diagnostics.ProcessStartInfo]::new()
$psi.FileName = $exeFull
$psi.Arguments = "--selftest"
$psi.UseShellExecute = $false
$selftest = [System.Diagnostics.Process]::Start($psi)
$selftest.WaitForExit()
Write-Output "SELFTEST_EXIT=$($selftest.ExitCode)"
if ($selftest.ExitCode -ne 0) { Write-Output "SELFTEST_FAILED"; exit 1 }

$sw = [System.Diagnostics.Stopwatch]::StartNew()
$p = Start-Process -FilePath $exeFull -PassThru
$handle = [IntPtr]::Zero
while ($sw.ElapsedMilliseconds -lt 20000) {
    $p.Refresh()
    if ($p.MainWindowHandle -ne [IntPtr]::Zero) { $handle = $p.MainWindowHandle; break }
    if ($p.HasExited) { Write-Output "PROCESS_EXITED_EARLY code=$($p.ExitCode)"; exit 1 }
    Start-Sleep -Milliseconds 50
}
$startupMs = $sw.ElapsedMilliseconds
if ($handle -eq [IntPtr]::Zero) { Write-Output "NO_WINDOW"; $p.Kill(); exit 1 }

# 立即置顶（NOACTIVATE 不抢焦点），等待渲染稳定后屏幕区域截图，再取消置顶。
[void][System.Reflection.Assembly]::LoadWithPartialName("System.Drawing")
$sig = @"
using System;
using System.Runtime.InteropServices;
public class Win32 {
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT rect);
    [DllImport("user32.dll")] public static extern bool SetWindowPos(IntPtr hWnd, IntPtr hWndInsertAfter, int X, int Y, int cx, int cy, uint uFlags);
    [DllImport("user32.dll")] public static extern IntPtr GetForegroundWindow();
    [DllImport("shcore.dll")] public static extern int SetProcessDpiAwareness(int awareness);
    public struct RECT { public int Left; public int Top; public int Right; public int Bottom; }
}
"@
Add-Type -TypeDefinition $sig -ReferencedAssemblies System.Drawing
# 关键：声明为 Per-Monitor DPI 感知（2），使 GetWindowRect 与 CopyFromScreen
# 统一使用物理像素——否则 >100% 缩放的显示器上截图会被放大/裁切、带入邻窗内容。
# 进程已声明过时返回 0x80070005，忽略即可。
try { [void][Win32]::SetProcessDpiAwareness(2) } catch { }
$SWP_NOMOVE = 0x0002; $SWP_NOSIZE = 0x0001; $SWP_NOACTIVATE = 0x0010
$HWND_TOPMOST = New-Object IntPtr(-1)
$HWND_NOTOPMOST = New-Object IntPtr(-2)
$setOk = [Win32]::SetWindowPos($handle, $HWND_TOPMOST, 0, 0, 0, 0, $SWP_NOMOVE -bor $SWP_NOSIZE -bor $SWP_NOACTIVATE)
Write-Output "SET_TOPMOST=$setOk"
Start-Sleep -Milliseconds 3000
$rect = New-Object Win32+RECT
[Win32]::GetWindowRect($handle, [ref]$rect) | Out-Null
# 注：已声明 Per-Monitor DPI 感知（见上），物理像素坐标一致；
# 若窗口矩形被其他置顶窗口遮挡仍会带入遮挡内容，属截图环境限制。

$w = $rect.Right - $rect.Left
$h = $rect.Bottom - $rect.Top
$bmp = New-Object System.Drawing.Bitmap $w, $h
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($rect.Left, $rect.Top, 0, 0, $bmp.Size)
# 截图完成，取消置顶
[Win32]::SetWindowPos($handle, $HWND_NOTOPMOST, 0, 0, 0, 0, $SWP_NOMOVE -bor $SWP_NOSIZE -bor $SWP_NOACTIVATE) | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path $ShotPath) | Out-Null
$bmp.Save((Join-Path (Get-Location) $ShotPath), [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()

$title = $p.MainWindowTitle
$proc = Get-Process -Id $p.Id
$rssMB = [math]::Round($proc.WorkingSet64 / 1MB, 1)
$p.Kill()
Write-Output "STARTUP_MS=$startupMs"
Write-Output "WINDOW_TITLE=$title"
Write-Output "WINDOW_SIZE=${w}x${h}"
Write-Output "RSS_MB_AFTER_LAUNCH=$rssMB"
Write-Output "SCREENSHOT=$ShotPath"
