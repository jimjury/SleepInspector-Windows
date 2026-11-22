# SleepInspector.Actions.ps1
# Safe targeting helpers. No process is closed during import or lookup.
# This file is packaged as UTF-8 BOM for Windows PowerShell 5.1.

function Test-ExactExecutablePath {
    param([string]$Expected, [string]$Actual)
    if ([string]::IsNullOrWhiteSpace($Expected) -or [string]::IsNullOrWhiteSpace($Actual)) { return $false }
    try {
        $a = [System.IO.Path]::GetFullPath($Expected.Trim().Trim('"'))
        $b = [System.IO.Path]::GetFullPath($Actual.Trim().Trim('"'))
        if (-not [System.IO.Path]::IsPathRooted($a) -or -not [System.IO.Path]::IsPathRooted($b)) { return $false }
        return [string]::Equals($a, $b, [System.StringComparison]::OrdinalIgnoreCase)
    } catch { return $false }
}

function Get-ProtectedProcessReason {
    param([string]$ExecutablePath)
    if ([string]::IsNullOrWhiteSpace($ExecutablePath)) { return '未获得完整可执行文件路径，禁止自动关闭。' }
    try { $path = [System.IO.Path]::GetFullPath($ExecutablePath.Trim().Trim('"')) }
    catch { return '程序路径无效，禁止自动关闭。' }
    $filename = [System.IO.Path]::GetFileName($path).ToLowerInvariant()
    $protected = @(
        'system','system idle process','registry','idle',
        'csrss.exe','smss.exe','wininit.exe','winlogon.exe','lsass.exe',
        'services.exe','svchost.exe','fontdrvhost.exe','dwm.exe',
        'taskhostw.exe','sihost.exe','explorer.exe','conhost.exe',
        'powershell.exe','pwsh.exe','cmd.exe','taskmgr.exe',
        'audiodg.exe','spoolsv.exe','searchindexer.exe','wmiprvse.exe',
        'runtimebroker.exe','shellexperiencehost.exe','startmenuexperiencehost.exe'
    )
    if ($filename -in $protected) { return '这是 Windows 系统组件或诊断器相关进程，不提供结束功能。' }
    if ($filename -notmatch '\.exe$') { return '目标不是可确认的 EXE 程序。' }
    if (-not [string]::IsNullOrWhiteSpace($env:WINDIR)) {
        $systemDir = $env:WINDIR.TrimEnd('\') + '\'
        if ($path.StartsWith($systemDir,[System.StringComparison]::OrdinalIgnoreCase)) {
            return 'Windows 系统目录中的进程受到保护，请使用任务管理器或系统设置排查。'
        }
    }
    return ''
}

function Convert-DevicePathToDosPath {
    param([string]$DevicePath)
    if ([string]::IsNullOrWhiteSpace($DevicePath)) { return $null }
    $raw = $DevicePath.Trim().Trim('"')
    if ($raw -match '^[A-Za-z]:\\.+\.exe$') { return $raw }
    if ($raw -match '^\\\\\?\\[A-Za-z]:\\.+\.exe$') { return $raw.Substring(4) }
    # Only translate the disk device namespace. Other kernel device objects are not processes.
    if ($raw -notmatch '^\\Device\\HarddiskVolume[^\\]+\\.+\.exe$') { return $null }
    if (-not ('SleepInspectorNativePaths' -as [type])) {
        $nativeSource = @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public static class SleepInspectorNativePaths {
    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    public static extern uint QueryDosDevice(string deviceName, StringBuilder targetPath, int maxChars);
}
'@
        Add-Type -TypeDefinition $nativeSource -ErrorAction Stop
    }
    foreach ($drive in [System.IO.DriveInfo]::GetDrives()) {
        if ($drive.Name -notmatch '^[A-Za-z]:\\$') { continue }
        $letter = $drive.Name.Substring(0,2)
        $buffer = [System.Text.StringBuilder]::new(2048)
        $count = [SleepInspectorNativePaths]::QueryDosDevice($letter, $buffer, $buffer.Capacity)
        if ($count -eq 0) { continue }
        $device = $buffer.ToString().Split([char]0)[0].TrimEnd('\')
        if ($raw.StartsWith(($device + '\'), [System.StringComparison]::OrdinalIgnoreCase)) {
            return $letter + $raw.Substring($device.Length)
        }
    }
    return $null
}

function Find-RequestProcessCandidates {
    param([string]$RequestName, [object[]]$ProcessSnapshot = $null)
    $exePath = Convert-DevicePathToDosPath $RequestName
    if ([string]::IsNullOrWhiteSpace($exePath)) {
        return [pscustomobject]@{ Path=''; Reason='该请求没有可验证的完整 EXE 路径，不能安全对应到特定进程。'; Candidates=@() }
    }
    $reason = Get-ProtectedProcessReason $exePath
    if ($reason) { return [pscustomobject]@{ Path=$exePath; Reason=$reason; Candidates=@() } }
    $matches = New-Object 'System.Collections.Generic.List[object]'
    try {
        $procs = if ($null -ne $ProcessSnapshot) { @($ProcessSnapshot) } else { @(Get-CimInstance -ClassName Win32_Process -ErrorAction Stop) }
        foreach ($p in $procs) {
            if (-not (Test-ExactExecutablePath $exePath ([string]$p.ExecutablePath))) { continue }
            if (-not $p.ProcessId -or -not $p.CreationDate) { continue }
            if ([int]$p.ProcessId -le 4 -or [int]$p.ProcessId -eq $PID) { continue }
            $matches.Add([pscustomobject]@{
                ProcessId=[int]$p.ProcessId
                Name=[string]$p.Name
                Path=[string]$p.ExecutablePath
                StartTicks=([datetime]$p.CreationDate).ToUniversalTime().Ticks
            })
        }
    } catch {
        return [pscustomobject]@{ Path=$exePath; Reason=('读取进程信息失败：' + $_.Exception.Message); Candidates=@() }
    }
    if ($matches.Count -eq 0) { $reason = '未找到与请求路径完全一致的运行中进程，可能已经退出。' }
    return [pscustomobject]@{ Path=$exePath; Reason=$reason; Candidates=$matches.ToArray() }
}

function Test-SamePowerRequest {
    param($Detected, $Original)
    if ($null -eq $Detected -or $null -eq $Original) { return $false }
    return ($Detected.Kind -eq 'PROCESS' -and $Original.Kind -eq 'PROCESS' -and
        $Detected.Section -eq $Original.Section -and
        [string]::Equals([string]$Detected.Name, [string]$Original.Name, [System.StringComparison]::OrdinalIgnoreCase))
}

function Test-SelectedProcessIdentity {
    param($Candidate)
    if ($null -eq $Candidate -or [int]$Candidate.ProcessId -le 4 -or [int]$Candidate.ProcessId -eq $PID) { return $false }
    if (Get-ProtectedProcessReason ([string]$Candidate.Path)) { return $false }
    try {
        $current = @(Get-CimInstance -ClassName Win32_Process -Filter ('ProcessId = ' + [int]$Candidate.ProcessId) -ErrorAction Stop)
        if ($current.Count -ne 1 -or -not $current[0].CreationDate) { return $false }
        if (-not (Test-ExactExecutablePath $Candidate.Path ([string]$current[0].ExecutablePath))) { return $false }
        return ([long]$Candidate.StartTicks -eq ([datetime]$current[0].CreationDate).ToUniversalTime().Ticks)
    } catch { return $false }
}
