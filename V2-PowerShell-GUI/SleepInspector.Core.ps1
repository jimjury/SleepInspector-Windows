# SleepInspector.Core.ps1 - Windows PowerShell 5.1 compatible, read-only parser.
Set-StrictMode -Version 2.0

function Get-RequestSection {
    param([string]$Header)
    $h = $Header.Trim().ToUpperInvariant()
    switch ($h) {
        { $_ -in @('DISPLAY','显示','显示器') } { return 'DISPLAY' }
        { $_ -in @('SYSTEM','系统') } { return 'SYSTEM' }
        { $_ -in @('AWAYMODE','离开模式') } { return 'AWAYMODE' }
        { $_ -in @('EXECUTION','执行') } { return 'EXECUTION' }
        { $_ -in @('PERFBOOST','性能提升') } { return 'PERFBOOST' }
        { $_ -in @('ACTIVELOCKSCREEN','活动锁定屏幕','锁定屏幕') } { return 'ACTIVELOCKSCREEN' }
        default { return $null }
    }
}

function Get-RequestKind {
    param([string]$Kind)
    switch ($Kind.Trim().ToUpperInvariant()) {
        { $_ -in @('PROCESS','进程') } { return 'PROCESS' }
        { $_ -in @('DRIVER','驱动','驱动程序') } { return 'DRIVER' }
        { $_ -in @('SERVICE','服务') } { return 'SERVICE' }
        default { return 'UNKNOWN' }
    }
}

function Get-RequestKindText {
    param([string]$Kind)
    switch ($Kind) {
        'PROCESS' { return '程序/进程' }
        'DRIVER' { return '驱动程序' }
        'SERVICE' { return '系统服务' }
        default { return '其他来源' }
    }
}

function Get-RequestImpact {
    param([string]$Section)
    switch ($Section) {
        'SYSTEM' { return '阻止自动休眠' }
        'DISPLAY' { return '阻止屏幕自动关闭' }
        'AWAYMODE' { return '请求离开模式' }
        'EXECUTION' { return '执行请求（可能影响休眠）' }
        'PERFBOOST' { return '性能提升请求' }
        'ACTIVELOCKSCREEN' { return '活动锁屏请求' }
        default { return '其他请求' }
    }
}

function Add-RequestItem {
    param(
        [System.Collections.Generic.List[object]]$Items,
        [string]$Section,
        [string]$Kind,
        [string]$Name,
        [System.Collections.Generic.List[string]]$Details
    )
    $nameValue = $Name.Trim()
    if ([string]::IsNullOrWhiteSpace($nameValue)) { $nameValue = '（未提供对象名称）' }
    $desc = ($Details.ToArray() -join "`n").Trim()
    $Items.Add([pscustomobject]@{
        Section = $Section
        Impact = Get-RequestImpact $Section
        Kind = $Kind
        KindText = Get-RequestKindText $Kind
        Name = $nameValue
        Detail = $desc
    })
}

function Parse-PowerRequests {
    [CmdletBinding()]
    param([AllowEmptyString()][string]$Text)
    $items = New-Object 'System.Collections.Generic.List[object]'
    $unknown = New-Object 'System.Collections.Generic.List[string]'
    $sections = New-Object 'System.Collections.Generic.List[string]'
    $currentSection = $null
    $currentKind = $null
    $currentName = $null
    $currentDetails = New-Object 'System.Collections.Generic.List[string]'
    $lines = $Text -split '\r?\n'
    foreach ($line in $lines) {
        # English headers are used by powercfg on many localized Windows installations.
        if ($line -match '^\s*([A-Za-z]+|[\p{IsCJKUnifiedIdeographs}]+)\s*[:：]\s*$') {
            $found = Get-RequestSection $Matches[1]
            if ($null -ne $found) {
                if ($null -ne $currentKind) {
                    Add-RequestItem $items $currentSection $currentKind $currentName $currentDetails
                    $currentKind = $null
                    $currentDetails.Clear()
                }
                $currentSection = $found
                if (-not $sections.Contains($found)) { $sections.Add($found) }
                continue
            }
        }
        if ($null -eq $currentSection) {
            if (-not [string]::IsNullOrWhiteSpace($line)) { $unknown.Add($line.Trim()) }
            continue
        }
        if ($line -match '^\s*\[(PROCESS|DRIVER|SERVICE|进程|驱动|驱动程序|服务)\]\s*(.*?)\s*$') {
            if ($null -ne $currentKind) {
                Add-RequestItem $items $currentSection $currentKind $currentName $currentDetails
                $currentDetails.Clear()
            }
            $currentKind = Get-RequestKind $Matches[1]
            $currentName = $Matches[2]
            continue
        }
        $trimmed = $line.Trim()
        if ($trimmed.Length -eq 0) { continue }
        if ($trimmed -match '^(None\.?|None。|无[。.]?|没有|无请求|无活动请求|无任何请求)$') { continue }
        if ($null -ne $currentKind) {
            $currentDetails.Add($trimmed)
        } else {
            $unknown.Add("$($currentSection): $trimmed")
        }
    }
    if ($null -ne $currentKind) {
        Add-RequestItem $items $currentSection $currentKind $currentName $currentDetails
    }
    return [pscustomobject]@{
        Items = $items.ToArray()
        Sections = $sections.ToArray()
        Unparsed = $unknown.ToArray()
    }
}
