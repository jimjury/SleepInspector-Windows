# SleepInspector.ps1 - Windows sleep-request diagnostic GUI with opt-in application closing.
# Windows PowerShell 5.1 / .NET Framework / Windows 10 and 11.
# Launch with Start_Sleep_Inspector.cmd. UTF-8 with BOM is required for PS 5.1.

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()

function Show-UiError {
    param([string]$Message, [string]$Caption = '休眠阻止项检测器')
    [void][System.Windows.Forms.MessageBox]::Show(
        $Message, $Caption,
        [System.Windows.Forms.MessageBoxButtons]::OK,
        [System.Windows.Forms.MessageBoxIcon]::Warning
    )
}

function Test-IsAdministrator {
    $identity = [System.Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [System.Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([System.Security.Principal.WindowsBuiltInRole]::Administrator)
}

# Request elevation once, before creating the actual window.
if (-not (Test-IsAdministrator)) {
    try {
        $self = $MyInvocation.MyCommand.Path
        $exe = Join-Path $PSHOME 'powershell.exe'
        $arguments = '-NoLogo -NoProfile -STA -ExecutionPolicy Bypass -WindowStyle Hidden -File "' + $self + '"'
        Start-Process -FilePath $exe -ArgumentList $arguments -Verb RunAs -ErrorAction Stop | Out-Null
    } catch {
        Show-UiError ('无法获取管理员权限，检测器尚未启动。' + "`r`n`r`n" + $_.Exception.Message)
    }
    exit
}

try {
    . (Join-Path $PSScriptRoot 'SleepInspector.Core.ps1')
    . (Join-Path $PSScriptRoot 'SleepInspector.Actions.ps1')

    $script:ui = @{}
    $script:state = [pscustomobject]@{
        LastCheck = $null
        Requests = $null
        LastWake = $null
        WakeTimers = $null
        Parsed = $null
        Actions = (New-Object 'System.Collections.Generic.List[string]')
    }

    $navy = [System.Drawing.Color]::FromArgb(21, 40, 61)
    $ink = [System.Drawing.Color]::FromArgb(37, 55, 75)
    $muted = [System.Drawing.Color]::FromArgb(92, 108, 125)
    $blue = [System.Drawing.Color]::FromArgb(25, 112, 183)
    $red = [System.Drawing.Color]::FromArgb(186, 64, 65)
    $green = [System.Drawing.Color]::FromArgb(36, 129, 104)
    $surface = [System.Drawing.Color]::FromArgb(246, 249, 252)
    $border = [System.Drawing.Color]::FromArgb(223, 231, 238)
    $white = [System.Drawing.Color]::White
    $font = [System.Drawing.Font]::new('Microsoft YaHei UI', 9.0)
    $fontSmall = [System.Drawing.Font]::new('Microsoft YaHei UI', 8.5)
    $fontTitle = [System.Drawing.Font]::new('Microsoft YaHei UI', 19.0, [System.Drawing.FontStyle]::Bold)
    $fontBig = [System.Drawing.Font]::new('Segoe UI', 21.0, [System.Drawing.FontStyle]::Bold)
    $fontMono = [System.Drawing.Font]::new('Consolas', 9.0)

    function New-Label {
        param(
            [string]$Text, [int]$X, [int]$Y, [int]$Width, [int]$Height,
            [System.Drawing.Font]$Font = $script:font,
            [System.Drawing.Color]$ForeColor = $script:ink
        )
        $label = [System.Windows.Forms.Label]::new()
        $label.Text = $Text
        $label.Location = [System.Drawing.Point]::new($X, $Y)
        $label.Size = [System.Drawing.Size]::new($Width, $Height)
        $label.Font = $Font
        $label.ForeColor = $ForeColor
        $label.TextAlign = [System.Drawing.ContentAlignment]::MiddleLeft
        return $label
    }

    function New-Button {
        param([string]$Text, [int]$Width = 112, [bool]$Primary = $false)
        $button = [System.Windows.Forms.Button]::new()
        $button.Text = $Text
        $button.Width = $Width
        $button.Height = 32
        $button.Margin = [System.Windows.Forms.Padding]::new(4, 7, 4, 3)
        $button.FlatStyle = [System.Windows.Forms.FlatStyle]::Flat
        $button.FlatAppearance.BorderColor = $script:border
        $button.Font = $script:font
        $button.Cursor = [System.Windows.Forms.Cursors]::Hand
        if ($Primary) {
            $button.BackColor = $script:blue
            $button.ForeColor = $script:white
            $button.FlatAppearance.BorderSize = 0
        } else {
            $button.BackColor = $script:white
            $button.ForeColor = $script:ink
        }
        return $button
    }

    function New-TextArea {
        $box = [System.Windows.Forms.TextBox]::new()
        $box.Dock = [System.Windows.Forms.DockStyle]::Fill
        $box.Multiline = $true
        $box.ReadOnly = $true
        $box.ScrollBars = [System.Windows.Forms.ScrollBars]::Both
        $box.WordWrap = $false
        $box.Font = $script:fontMono
        $box.BackColor = $script:white
        $box.ForeColor = $script:ink
        return $box
    }

    function New-SummaryCard {
        param([string]$Caption, [string]$HelpText)
        $panel = [System.Windows.Forms.Panel]::new()
        $panel.Dock = [System.Windows.Forms.DockStyle]::Fill
        $panel.Margin = [System.Windows.Forms.Padding]::new(10, 8, 2, 8)
        $panel.BackColor = $script:white
        $panel.BorderStyle = [System.Windows.Forms.BorderStyle]::FixedSingle
        $num = New-Label '—' 13 3 65 44 $script:fontBig $script:blue
        $captionLabel = New-Label $Caption 75 5 300 23 $script:font $script:ink
        $helpLabel = New-Label $HelpText 75 26 350 21 $script:fontSmall $script:muted
        $captionLabel.Anchor = [System.Windows.Forms.AnchorStyles]'Top,Left,Right'
        $helpLabel.Anchor = [System.Windows.Forms.AnchorStyles]'Top,Left,Right'
        $panel.Controls.AddRange([System.Windows.Forms.Control[]]@($num, $captionLabel, $helpLabel))
        return [pscustomobject]@{ Panel=$panel; Number=$num; Caption=$captionLabel; Help=$helpLabel }
    }

    function Invoke-PowerCfgRead {
        param([string]$Arguments)
        $proc = $null
        try {
            $path = Join-Path $env:WINDIR 'System32\powercfg.exe'
            if (-not [System.IO.File]::Exists($path)) { $path = 'powercfg.exe' }
            $start = [System.Diagnostics.ProcessStartInfo]::new()
            $start.FileName = $path
            $start.Arguments = $Arguments
            $start.UseShellExecute = $false
            $start.CreateNoWindow = $true
            $start.RedirectStandardOutput = $true
            $start.RedirectStandardError = $true
            # powercfg is a native console executable: decode in the current OEM code page.
            $start.StandardOutputEncoding = [System.Text.Encoding]::GetEncoding(
                [System.Globalization.CultureInfo]::CurrentCulture.TextInfo.OEMCodePage
            )
            $start.StandardErrorEncoding = $start.StandardOutputEncoding
            $proc = [System.Diagnostics.Process]::new()
            $proc.StartInfo = $start
            if (-not $proc.Start()) { throw '无法启动 powercfg.exe。' }
            $stdoutTask = $proc.StandardOutput.ReadToEndAsync()
            $stderrTask = $proc.StandardError.ReadToEndAsync()
            if (-not $proc.WaitForExit(12000)) {
                try { $proc.Kill() } catch { }
                throw ('powercfg ' + $Arguments + ' 超过 12 秒，已终止等待。')
            }
            $stdout = $stdoutTask.Result
            $stderr = $stderrTask.Result
            $exitCode = $proc.ExitCode
            return [pscustomobject]@{
                Command='powercfg ' + $Arguments
                Success=($exitCode -eq 0)
                ExitCode=$exitCode
                Output=$stdout.TrimEnd()
                Error=$stderr.TrimEnd()
            }
        } catch {
            return [pscustomobject]@{
                Command='powercfg ' + $Arguments
                Success=$false
                ExitCode=-1
                Output=''
                Error=$_.Exception.Message
            }
        } finally {
            if ($null -ne $proc) { $proc.Dispose() }
        }
    }

    function Format-ResultText {
        param($Result)
        if ($null -eq $Result) { return '尚未运行检测。' }
        $body = $Result.Output
        if (-not $Result.Success) {
            $body = '[执行失败，退出码：' + $Result.ExitCode + "]`r`n" + $body
        }
        if (-not [string]::IsNullOrWhiteSpace($Result.Error)) {
            $body += "`r`n[错误信息]`r`n" + $Result.Error
        }
        if ([string]::IsNullOrWhiteSpace($body)) { return '（命令未返回文本）' }
        return $body
    }

    function Format-Report {
        $sb = [System.Text.StringBuilder]::new()
        [void]$sb.AppendLine('Windows 休眠阻止项检测报告')
        [void]$sb.AppendLine(('检测时间：' + $script:state.LastCheck))
        [void]$sb.AppendLine(('计算机：' + $env:COMPUTERNAME))
        [void]$sb.AppendLine('说明：DISPLAY 仅代表显示器关闭请求；SYSTEM 代表阻止自动休眠的请求。')
        [void]$sb.AppendLine('未检出请求不保证机器一定能够正常休眠。')
        foreach ($result in @($script:state.Requests, $script:state.LastWake, $script:state.WakeTimers)) {
            [void]$sb.AppendLine()
            [void]$sb.AppendLine(('====== ' + $result.Command + ' ======'))
            [void]$sb.AppendLine((Format-ResultText $result))
        }
        if ($script:state.Actions.Count -gt 0) {
            [void]$sb.AppendLine()
            [void]$sb.AppendLine('====== 本次运行的程序关闭记录 ======')
            foreach ($line in $script:state.Actions) { [void]$sb.AppendLine($line) }
        }
        if ($null -ne $script:state.Parsed -and $script:state.Parsed.Unparsed.Count -gt 0) {
            [void]$sb.AppendLine()
            [void]$sb.AppendLine('====== 未识别的输出行（请根据原始内容核对） ======')
            foreach ($unknown in $script:state.Parsed.Unparsed) { [void]$sb.AppendLine($unknown) }
        }
        return $sb.ToString()
    }

    function Update-Grid {
        $grid = $script:ui.Grid
        $selection = [string]$script:ui.Filter.SelectedItem
        $grid.SuspendLayout()
        $script:ui.CloseButton.Enabled = $false
        try {
            $grid.Rows.Clear()
            foreach ($entry in $script:state.Parsed.Items) {
                if ($selection -eq '仅 SYSTEM 请求' -and $entry.Section -ne 'SYSTEM') { continue }
                if ($selection -eq '仅阻止屏幕关闭' -and $entry.Section -ne 'DISPLAY') { continue }
                if ($selection -eq '其他电源请求' -and $entry.Section -in @('SYSTEM','DISPLAY')) { continue }
                $n = $grid.Rows.Add([object[]]@(
                    $entry.Impact, $entry.KindText, $entry.Name, $entry.Detail, $entry.Section
                ))
                $grid.Rows[$n].Tag = $entry
                if ($entry.Section -eq 'SYSTEM') {
                    $grid.Rows[$n].Cells[0].Style.ForeColor = $script:red
                } elseif ($entry.Section -eq 'DISPLAY') {
                    $grid.Rows[$n].Cells[0].Style.ForeColor = $script:blue
                }
            }
            $grid.ClearSelection()
            $script:ui.Details.Text = '点击表格中的一行，可查看完整对象名称与说明。'
            if ($grid.Rows.Count -eq 0) {
                $script:ui.Details.Text = if ($script:state.Requests.Success) {
                    '当前筛选条件下未检测到活动请求。可以切换“全部请求”或查看完整原始输出。'
                } else { '命令执行失败，请查看“原始输出”页中的错误信息。' }
            }
        } finally {
            $grid.ResumeLayout()
        }
    }

    function Refresh-Inspector {
        $script:ui.RefreshButton.Enabled = $false
        $script:ui.CloseButton.Enabled = $false
        $script:ui.Footer.Text = '正在检测，请稍候…'
        $script:ui.Form.Cursor = [System.Windows.Forms.Cursors]::WaitCursor
        $script:ui.Form.Refresh()
        try {
            $script:state.Requests = Invoke-PowerCfgRead '/requests'
            $script:state.LastWake = Invoke-PowerCfgRead '/lastwake'
            $script:state.WakeTimers = Invoke-PowerCfgRead '/waketimers'
            $script:state.LastCheck = (Get-Date).ToString('yyyy-MM-dd HH:mm:ss')
            if ($script:state.Requests.Success) {
                $script:state.Parsed = Parse-PowerRequests $script:state.Requests.Output
                $all = @($script:state.Parsed.Items)
                $sleep = @($all | Where-Object { $_.Section -eq 'SYSTEM' }).Count
                $display = @($all | Where-Object { $_.Section -eq 'DISPLAY' }).Count
                $other = @($all | Where-Object { $_.Section -notin @('SYSTEM','DISPLAY') }).Count
                $script:ui.SleepCard.Number.Text = [string]$sleep
                $script:ui.DisplayCard.Number.Text = [string]$display
                $script:ui.OtherCard.Number.Text = [string]$other
                $script:ui.SleepCard.Number.ForeColor = if ($sleep -gt 0) { $script:red } else { $script:green }
                $script:ui.SleepCard.Help.Text = if ($sleep -gt 0) { '发现活动 SYSTEM 请求' } else { '当前未发现 SYSTEM 请求' }
                $script:ui.DisplayCard.Help.Text = if ($display -gt 0) { '仅涉及屏幕自动关闭' } else { '当前未发现 DISPLAY 请求' }
                $script:ui.OtherCard.Help.Text = '离开模式 / 执行 / 性能等'
                $unparsedCount = @($script:state.Parsed.Unparsed).Count
                $unparsedHint = if ($unparsedCount -gt 0) { "；有 $unparsedCount 行未识别，请核对原始输出" } else { '' }
                $script:ui.Footer.Text = "检测完成：$($all.Count) 条活动请求$unparsedHint  |  $($script:state.LastCheck)"
            } else {
                $script:state.Parsed = Parse-PowerRequests ''
                $script:ui.SleepCard.Number.Text = '—'
                $script:ui.DisplayCard.Number.Text = '—'
                $script:ui.OtherCard.Number.Text = '—'
                $script:ui.Footer.Text = '读取 /requests 失败，请打开“原始输出”查看原因。'
            }
            $script:ui.Wake.Text = Format-ResultText $script:state.LastWake
            $script:ui.Timers.Text = Format-ResultText $script:state.WakeTimers
            $script:ui.Raw.Text = Format-Report
            Update-Grid
        } catch {
            $script:ui.Footer.Text = '检测过程发生错误。'
            Show-UiError $_.Exception.Message
        } finally {
            $script:ui.Form.Cursor = [System.Windows.Forms.Cursors]::Default
            $script:ui.RefreshButton.Enabled = $true
        }
    }

    function Copy-Report {
        try {
            [System.Windows.Forms.Clipboard]::SetText((Format-Report))
            $script:ui.Footer.Text = '完整报告已复制到剪贴板。'
        } catch { Show-UiError ('复制失败：' + $_.Exception.Message) }
    }

    function Save-Report {
        $dialog = [System.Windows.Forms.SaveFileDialog]::new()
        $dialog.Title = '保存休眠诊断日志'
        $dialog.Filter = '文本文件 (*.txt)|*.txt|所有文件 (*.*)|*.*'
        $dialog.FileName = 'SleepInspector_' + (Get-Date).ToString('yyyyMMdd_HHmmss') + '.txt'
        $dialog.OverwritePrompt = $true
        try {
            if ($dialog.ShowDialog($script:ui.Form) -eq [System.Windows.Forms.DialogResult]::OK) {
                # UTF-8 BOM: traditional Windows Notepad and Windows PowerShell read Chinese correctly.
                $enc = [System.Text.UTF8Encoding]::new($true)
                [System.IO.File]::WriteAllText($dialog.FileName, (Format-Report), $enc)
                $script:ui.Footer.Text = '日志已保存：' + $dialog.FileName
            }
        } catch { Show-UiError ('保存失败：' + $_.Exception.Message) } finally { $dialog.Dispose() }
    }


    function Confirm-Action {
        param([string]$Text, [string]$Title = '确认关闭程序', [bool]$Danger = $false)
        $symbol = if ($Danger) { [System.Windows.Forms.MessageBoxIcon]::Warning } else { [System.Windows.Forms.MessageBoxIcon]::Question }
        return ([System.Windows.Forms.MessageBox]::Show(
            $script:ui.Form, $Text, $Title,
            [System.Windows.Forms.MessageBoxButtons]::YesNo, $symbol,
            [System.Windows.Forms.MessageBoxDefaultButton]::Button2
        ) -eq [System.Windows.Forms.DialogResult]::Yes)
    }

    function Select-ProcessCandidate {
        param([object[]]$Candidates)
        # powercfg /requests does not include a PID. Never guess which of several instances owns a request.
        $dialog = [System.Windows.Forms.Form]::new()
        $dialog.Text = '选择要关闭的程序实例'
        $dialog.StartPosition = [System.Windows.Forms.FormStartPosition]::CenterParent
        $dialog.Size = [System.Drawing.Size]::new(800, 380)
        $dialog.MinimumSize = [System.Drawing.Size]::new(660, 330)
        $dialog.Font = $script:font
        $dialog.FormBorderStyle = [System.Windows.Forms.FormBorderStyle]::Sizable
        $dialog.MaximizeBox = $false
        $layout = [System.Windows.Forms.TableLayoutPanel]::new()
        $layout.Dock = [System.Windows.Forms.DockStyle]::Fill
        $layout.RowCount = 3
        [void]$layout.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Absolute, 75))
        [void]$layout.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Percent, 100))
        [void]$layout.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Absolute, 52))
        $dialog.Controls.Add($layout)
        $notice = [System.Windows.Forms.Label]::new()
        $notice.Dock = [System.Windows.Forms.DockStyle]::Fill
        $notice.Padding = [System.Windows.Forms.Padding]::new(12, 6, 12, 3)
        $notice.Text = '检测到多个同路径程序。powercfg 未提供 PID，无法确定哪一个真正持有电源请求。请核实进程编号后选择；也可以取消。'
        $layout.Controls.Add($notice, 0, 0)
        $list = [System.Windows.Forms.ListView]::new()
        $list.Dock = [System.Windows.Forms.DockStyle]::Fill
        $list.View = [System.Windows.Forms.View]::Details
        $list.FullRowSelect = $true
        $list.MultiSelect = $false
        $list.HideSelection = $false
        [void]$list.Columns.Add('PID', 75)
        [void]$list.Columns.Add('程序', 160)
        [void]$list.Columns.Add('可执行文件路径', 520)
        foreach ($candidate in $Candidates) {
            $item = [System.Windows.Forms.ListViewItem]::new([string]$candidate.ProcessId)
            [void]$item.SubItems.Add([string]$candidate.Name)
            [void]$item.SubItems.Add([string]$candidate.Path)
            $item.Tag = $candidate
            [void]$list.Items.Add($item)
        }
        $layout.Controls.Add($list, 0, 1)
        $buttons = [System.Windows.Forms.FlowLayoutPanel]::new()
        $buttons.Dock = [System.Windows.Forms.DockStyle]::Fill
        $buttons.FlowDirection = [System.Windows.Forms.FlowDirection]::RightToLeft
        $cancel = [System.Windows.Forms.Button]::new()
        $cancel.Text = '取消'
        $cancel.Size = [System.Drawing.Size]::new(95, 32)
        $cancel.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
        $accept = [System.Windows.Forms.Button]::new()
        $accept.Text = '选择此 PID'
        $accept.Size = [System.Drawing.Size]::new(110, 32)
        $accept.Enabled = $false
        $accept.DialogResult = [System.Windows.Forms.DialogResult]::OK
        $list.Tag = $accept
        $list.Add_SelectedIndexChanged({ param($sender,$eventArgs) $sender.Tag.Enabled = ($sender.SelectedItems.Count -eq 1) })
        $buttons.Controls.AddRange([System.Windows.Forms.Control[]]@($cancel,$accept))
        $layout.Controls.Add($buttons, 0, 2)
        $dialog.AcceptButton = $accept
        $dialog.CancelButton = $cancel
        try {
            if ($dialog.ShowDialog($script:ui.Form) -eq [System.Windows.Forms.DialogResult]::OK -and $list.SelectedItems.Count -eq 1) {
                return $list.SelectedItems[0].Tag
            }
            return $null
        } finally { $dialog.Dispose() }
    }

    function Test-RequestStillActive {
        param($Entry)
        $result = Invoke-PowerCfgRead '/requests'
        if (-not $result.Success) { return $false }
        $parsed = Parse-PowerRequests $result.Output
        foreach ($active in $parsed.Items) {
            if (Test-SamePowerRequest $active $Entry) { return $true }
        }
        return $false
    }

    function Add-ActionRecord {
        param([string]$Text)
        $script:state.Actions.Add(((Get-Date).ToString('yyyy-MM-dd HH:mm:ss') + '  ' + $Text))
    }

    function Close-SelectedApplication {
        $grid = $script:ui.Grid
        if ($grid.SelectedRows.Count -ne 1) { Show-UiError '请先在“当前阻止项”列表中选中一个程序。'; return }
        $entry = $grid.SelectedRows[0].Tag
        if ($null -eq $entry -or $entry.Kind -ne 'PROCESS') {
            Show-UiError '只能关闭 [PROCESS] 程序项。驱动、系统服务以及其他类型的请求不能直接结束。'
            return
        }
        $script:ui.CloseButton.Enabled = $false
        try {
            $found = Find-RequestProcessCandidates $entry.Name
            if ($found.Reason) { Show-UiError $found.Reason; return }
            $candidates = @($found.Candidates)
            if ($candidates.Count -lt 1) { Show-UiError '当前没有找到可安全识别的目标进程。'; return }
            $candidate = if ($candidates.Count -eq 1) { $candidates[0] } else { Select-ProcessCandidate $candidates }
            if ($null -eq $candidate) { return }
            $confirmText = '将尝试正常关闭以下程序：' + "`r`n`r`n" +
                '程序：' + $candidate.Name + "`r`n" +
                'PID：' + $candidate.ProcessId + "`r`n" +
                '路径：' + $candidate.Path + "`r`n" +
                '电源请求：' + $entry.Section + "`r`n`r`n" +
                '请先保存该程序中的文档或任务。关闭程序可能中断播放、下载或未保存的工作。' + "`r`n" +
                '是否继续？'
            if (-not (Confirm-Action $confirmText)) { return }
            # Revalidate both request and process identity before touching a process.
            if (-not (Test-RequestStillActive $entry)) {
                Show-UiError '该阻止项已经消失，或无法重新确认；未执行关闭。请刷新后再试。'
                return
            }
            if (-not (Test-SelectedProcessIdentity $candidate)) {
                Show-UiError '目标进程的 PID、启动时间或路径已发生变化；为避免误关，操作已取消。'
                return
            }
            $process = [System.Diagnostics.Process]::GetProcessById([int]$candidate.ProcessId)
            try {
                $process.Refresh()
                $closed = $false
                if ($process.CloseMainWindow()) {
                    $closed = $process.WaitForExit(3000)
                }
                if (-not $closed) {
                    $forceText = '程序未在 3 秒内正常退出，或没有可关闭的主窗口。' + "`r`n`r`n" +
                        '程序：' + $candidate.Name + ' (PID ' + $candidate.ProcessId + ')' + "`r`n`r`n" +
                        '是否强制结束该进程？未保存的数据可能永久丢失。' + "`r`n" +
                        '默认选择“否”，不会强制结束。'
                    if (Confirm-Action $forceText '强制结束进程（高风险）' $true) {
                        if (-not (Test-RequestStillActive $entry) -or -not (Test-SelectedProcessIdentity $candidate)) {
                            Show-UiError '再次验证未通过，未强制结束进程。'
                            return
                        }
                        $process.Refresh()
                        if (-not $process.HasExited) {
                            $process.Kill()
                            $closed = $process.WaitForExit(3000)
                        } else { $closed = $true }
                    } else {
                        Add-ActionRecord ('用户取消强制结束：' + $candidate.Name + ' PID=' + $candidate.ProcessId)
                        Refresh-Inspector
                        return
                    }
                }
                if ($closed) {
                    Add-ActionRecord ('已结束进程：' + $candidate.Name + ' PID=' + $candidate.ProcessId)
                } else {
                    Add-ActionRecord ('关闭未确认成功：' + $candidate.Name + ' PID=' + $candidate.ProcessId)
                    Show-UiError '关闭命令已发出，但尚未确认进程退出。请核对新检测结果。'
                }
            } finally { $process.Dispose() }
            Refresh-Inspector
            if ($closed) { $script:ui.Footer.Text = '程序已结束，已重新检测阻止项。请核对上方列表。' }
        } catch {
            Add-ActionRecord ('关闭操作发生错误：' + $_.Exception.Message)
            Show-UiError ('无法关闭程序：' + $_.Exception.Message)
            Refresh-Inspector
        } finally {
            if ($script:ui.Grid.SelectedRows.Count -eq 1) {
                $selected = $script:ui.Grid.SelectedRows[0].Tag
                $script:ui.CloseButton.Enabled = ($null -ne $selected -and $selected.Kind -eq 'PROCESS')
            }
        }
    }

    $form = [System.Windows.Forms.Form]::new()
    $form.Text = 'Windows 休眠阻止项检测器 V1.1'
    $form.Size = [System.Drawing.Size]::new(1060, 740)
    $form.MinimumSize = [System.Drawing.Size]::new(910, 610)
    $form.StartPosition = [System.Windows.Forms.FormStartPosition]::CenterScreen
    $form.Font = $font
    $form.BackColor = $surface
    $form.AutoScaleMode = [System.Windows.Forms.AutoScaleMode]::Dpi
    $form.KeyPreview = $true
    $script:ui.Form = $form

    $root = [System.Windows.Forms.TableLayoutPanel]::new()
    $root.Dock = [System.Windows.Forms.DockStyle]::Fill
    $root.ColumnCount = 1
    $root.RowCount = 5
    $root.Margin = [System.Windows.Forms.Padding]::new(0)
    [void]$root.ColumnStyles.Add([System.Windows.Forms.ColumnStyle]::new([System.Windows.Forms.SizeType]::Percent, 100))
    foreach ($h in @(86, 78, 49)) {
        [void]$root.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Absolute, $h))
    }
    [void]$root.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Percent, 100))
    [void]$root.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Absolute, 31))
    $form.Controls.Add($root)

    $header = [System.Windows.Forms.Panel]::new()
    $header.Dock = [System.Windows.Forms.DockStyle]::Fill
    $header.BackColor = $navy
    $header.Margin = [System.Windows.Forms.Padding]::new(0)
    $title = New-Label '休眠阻止项检测器' 23 10 720 43 $fontTitle $white
    $subtitle = New-Label '实时读取 Windows 电源请求 · 手动关闭普通程序 · 不修改电源配置' 26 53 750 24 $fontSmall ([System.Drawing.Color]::FromArgb(194, 216, 233))
    $admin = New-Label '管理员模式' 863 25 160 32 $fontSmall ([System.Drawing.Color]::FromArgb(161, 221, 198))
    $admin.Anchor = [System.Windows.Forms.AnchorStyles]'Top,Right'
    $admin.TextAlign = [System.Drawing.ContentAlignment]::MiddleRight
    $header.Controls.AddRange([System.Windows.Forms.Control[]]@($title,$subtitle,$admin))
    $root.Controls.Add($header, 0, 0)

    $cards = [System.Windows.Forms.TableLayoutPanel]::new()
    $cards.Dock = [System.Windows.Forms.DockStyle]::Fill
    $cards.Margin = [System.Windows.Forms.Padding]::new(0)
    $cards.ColumnCount = 3
    $cards.RowCount = 1
    foreach ($p in @(33.34, 33.33, 33.33)) {
        [void]$cards.ColumnStyles.Add([System.Windows.Forms.ColumnStyle]::new([System.Windows.Forms.SizeType]::Percent, [single]$p))
    }
    $sleepCard = New-SummaryCard 'SYSTEM 休眠请求' '等待检测 SYSTEM 请求'
    $displayCard = New-SummaryCard '阻止显示器关闭' '等待检测 DISPLAY 请求'
    $otherCard = New-SummaryCard '其他活动请求' '等待检测其他类型'
    $script:ui.SleepCard = $sleepCard
    $script:ui.DisplayCard = $displayCard
    $script:ui.OtherCard = $otherCard
    $cards.Controls.Add($sleepCard.Panel, 0, 0)
    $cards.Controls.Add($displayCard.Panel, 1, 0)
    $cards.Controls.Add($otherCard.Panel, 2, 0)
    $root.Controls.Add($cards, 0, 1)

    $tools = [System.Windows.Forms.FlowLayoutPanel]::new()
    $tools.Dock = [System.Windows.Forms.DockStyle]::Fill
    $tools.FlowDirection = [System.Windows.Forms.FlowDirection]::LeftToRight
    $tools.WrapContents = $false
    $tools.Margin = [System.Windows.Forms.Padding]::new(12, 0, 8, 0)
    $tools.AutoScroll = $true
    $btnRefresh = New-Button '刷新检测 (F5)' 143 $true
    $btnCopy = New-Button '复制完整报告' 142
    $btnSave = New-Button '保存日志' 104
    $btnClose = New-Button '关闭所选程序' 138
    $btnClose.Enabled = $false
    $btnClose.ForeColor = $red
    $btnHelp = New-Button '功能说明' 104
    $script:ui.RefreshButton = $btnRefresh
    $script:ui.CloseButton = $btnClose
    $tools.Controls.AddRange([System.Windows.Forms.Control[]]@($btnRefresh,$btnClose,$btnCopy,$btnSave,$btnHelp))
    $root.Controls.Add($tools, 0, 2)

    $tabs = [System.Windows.Forms.TabControl]::new()
    $tabs.Dock = [System.Windows.Forms.DockStyle]::Fill
    $tabs.Margin = [System.Windows.Forms.Padding]::new(12, 0, 12, 5)
    $tabs.Font = $font
    $tabRequests = [System.Windows.Forms.TabPage]::new('当前阻止项')
    $tabWake = [System.Windows.Forms.TabPage]::new('唤醒诊断')
    $tabRaw = [System.Windows.Forms.TabPage]::new('原始输出与日志')
    $tabs.TabPages.AddRange([System.Windows.Forms.TabPage[]]@($tabRequests,$tabWake,$tabRaw))
    $root.Controls.Add($tabs, 0, 3)

    $requestsLayout = [System.Windows.Forms.TableLayoutPanel]::new()
    $requestsLayout.Dock = [System.Windows.Forms.DockStyle]::Fill
    $requestsLayout.ColumnCount = 1
    $requestsLayout.RowCount = 3
    [void]$requestsLayout.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Absolute, 39))
    [void]$requestsLayout.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Percent, 100))
    [void]$requestsLayout.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Absolute, 110))
    $tabRequests.Controls.Add($requestsLayout)
    $filterPanel = [System.Windows.Forms.Panel]::new()
    $filterPanel.Dock = [System.Windows.Forms.DockStyle]::Fill
    $filterLabel = New-Label '筛选请求：' 10 6 94 27
    $combo = [System.Windows.Forms.ComboBox]::new()
    $combo.DropDownStyle = [System.Windows.Forms.ComboBoxStyle]::DropDownList
    $combo.Location = [System.Drawing.Point]::new(104, 5)
    $combo.Width = 192
    $combo.Font = $font
    [void]$combo.Items.AddRange([object[]]@('全部请求','仅 SYSTEM 请求','仅阻止屏幕关闭','其他电源请求'))
    $combo.SelectedIndex = 0
    $script:ui.Filter = $combo
    $hint = New-Label '提示：屏幕保持亮起 ≠ 必然阻止电脑休眠' 320 5 550 27 $fontSmall $muted
    $hint.Anchor = [System.Windows.Forms.AnchorStyles]'Top,Left,Right'
    $filterPanel.Controls.AddRange([System.Windows.Forms.Control[]]@($filterLabel,$combo,$hint))
    $requestsLayout.Controls.Add($filterPanel,0,0)

    $grid = [System.Windows.Forms.DataGridView]::new()
    $grid.Dock = [System.Windows.Forms.DockStyle]::Fill
    $grid.AllowUserToAddRows = $false
    $grid.AllowUserToDeleteRows = $false
    $grid.AllowUserToResizeRows = $false
    $grid.ReadOnly = $true
    $grid.MultiSelect = $false
    $grid.SelectionMode = [System.Windows.Forms.DataGridViewSelectionMode]::FullRowSelect
    $grid.RowHeadersVisible = $false
    $grid.BackgroundColor = $white
    $grid.BorderStyle = [System.Windows.Forms.BorderStyle]::FixedSingle
    $grid.GridColor = $border
    $grid.ColumnHeadersHeight = 37
    $grid.RowTemplate.Height = 33
    $grid.EnableHeadersVisualStyles = $false
    $grid.ColumnHeadersDefaultCellStyle.BackColor = $surface
    $grid.ColumnHeadersDefaultCellStyle.ForeColor = $ink
    $grid.ColumnHeadersDefaultCellStyle.Font = [System.Drawing.Font]::new('Microsoft YaHei UI',9.0,[System.Drawing.FontStyle]::Bold)
    $grid.DefaultCellStyle.Font = $fontSmall
    $grid.DefaultCellStyle.SelectionBackColor = [System.Drawing.Color]::FromArgb(226, 240, 250)
    $grid.DefaultCellStyle.SelectionForeColor = $ink
    $grid.AutoSizeRowsMode = [System.Windows.Forms.DataGridViewAutoSizeRowsMode]::None
    [void]$grid.Columns.Add('Impact','影响范围')
    [void]$grid.Columns.Add('Kind','来源类型')
    [void]$grid.Columns.Add('Source','进程 / 驱动 / 服务')
    [void]$grid.Columns.Add('Detail','系统说明')
    [void]$grid.Columns.Add('Section','请求类别')
    $grid.Columns['Impact'].Width = 190
    $grid.Columns['Kind'].Width = 105
    $grid.Columns['Source'].Width = 280
    $grid.Columns['Section'].Width = 120
    $grid.Columns['Detail'].AutoSizeMode = [System.Windows.Forms.DataGridViewAutoSizeColumnMode]::Fill
    foreach ($column in $grid.Columns) {
        $column.SortMode = [System.Windows.Forms.DataGridViewColumnSortMode]::NotSortable
    }
    $script:ui.Grid = $grid
    $requestsLayout.Controls.Add($grid,0,1)
    $detailBox = [System.Windows.Forms.GroupBox]::new()
    $detailBox.Text = '选中项详细说明'
    $detailBox.Dock = [System.Windows.Forms.DockStyle]::Fill
    $detailBox.Padding = [System.Windows.Forms.Padding]::new(8, 13, 8, 6)
    $details = New-TextArea
    $details.Font = $fontSmall
    $details.WordWrap = $true
    $details.ScrollBars = [System.Windows.Forms.ScrollBars]::Vertical
    $details.Text = '点击表格中的一行，可查看完整对象名称与说明。'
    $script:ui.Details = $details
    $detailBox.Controls.Add($details)
    $requestsLayout.Controls.Add($detailBox,0,2)

    $wakeLayout = [System.Windows.Forms.TableLayoutPanel]::new()
    $wakeLayout.Dock = [System.Windows.Forms.DockStyle]::Fill
    $wakeLayout.ColumnCount = 2
    $wakeLayout.RowCount = 2
    [void]$wakeLayout.ColumnStyles.Add([System.Windows.Forms.ColumnStyle]::new([System.Windows.Forms.SizeType]::Percent, 50))
    [void]$wakeLayout.ColumnStyles.Add([System.Windows.Forms.ColumnStyle]::new([System.Windows.Forms.SizeType]::Percent, 50))
    [void]$wakeLayout.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Absolute, 50))
    [void]$wakeLayout.RowStyles.Add([System.Windows.Forms.RowStyle]::new([System.Windows.Forms.SizeType]::Percent, 100))
    $tabWake.Controls.Add($wakeLayout)
    $wakeHelp = New-Label '上次唤醒来源和唤醒定时器用于辅助诊断；它们不等同于当前休眠阻止项。' 12 2 900 39 $fontSmall $muted
    $wakeHelp.Dock = [System.Windows.Forms.DockStyle]::Fill
    $wakeLayout.Controls.Add($wakeHelp,0,0)
    $wakeLayout.SetColumnSpan($wakeHelp,2)
    $lastGroup = [System.Windows.Forms.GroupBox]::new()
    $lastGroup.Text = '上次唤醒来源 · powercfg /lastwake'
    $lastGroup.Dock = [System.Windows.Forms.DockStyle]::Fill
    $lastGroup.Padding = [System.Windows.Forms.Padding]::new(9, 15, 9, 10)
    $lastGroup.Margin = [System.Windows.Forms.Padding]::new(8, 0, 5, 8)
    $lastText = New-TextArea
    $lastText.Text = '等待检测…'
    $script:ui.Wake = $lastText
    $lastGroup.Controls.Add($lastText)
    $timerGroup = [System.Windows.Forms.GroupBox]::new()
    $timerGroup.Text = '活动唤醒定时器 · powercfg /waketimers'
    $timerGroup.Dock = [System.Windows.Forms.DockStyle]::Fill
    $timerGroup.Padding = [System.Windows.Forms.Padding]::new(9, 15, 9, 10)
    $timerGroup.Margin = [System.Windows.Forms.Padding]::new(5, 0, 8, 8)
    $timerText = New-TextArea
    $timerText.Text = '等待检测…'
    $script:ui.Timers = $timerText
    $timerGroup.Controls.Add($timerText)
    $wakeLayout.Controls.Add($lastGroup,0,1)
    $wakeLayout.Controls.Add($timerGroup,1,1)

    $raw = New-TextArea
    $raw.Text = '等待检测…'
    $raw.Dock = [System.Windows.Forms.DockStyle]::Fill
    $script:ui.Raw = $raw
    $tabRaw.Controls.Add($raw)

    $footer = [System.Windows.Forms.Label]::new()
    $footer.Dock = [System.Windows.Forms.DockStyle]::Fill
    $footer.Margin = [System.Windows.Forms.Padding]::new(16, 0, 8, 0)
    $footer.Text = '准备就绪。'
    $footer.Font = $fontSmall
    $footer.ForeColor = $muted
    $footer.TextAlign = [System.Drawing.ContentAlignment]::MiddleLeft
    $script:ui.Footer = $footer
    $root.Controls.Add($footer,0,4)

    $btnRefresh.Add_Click({ Refresh-Inspector })
    $btnClose.Add_Click({ Close-SelectedApplication })
    $btnCopy.Add_Click({ Copy-Report })
    $btnSave.Add_Click({ Save-Report })
    $btnHelp.Add_Click({
        [void][System.Windows.Forms.MessageBox]::Show(
            ('【SYSTEM】活动请求会阻止系统因空闲而自动进入睡眠。' + "`r`n" +
             '【DISPLAY】仅阻止显示器因空闲自动关闭，不等于阻止睡眠。' + "`r`n" +
             '【AWAYMODE】请求以离开模式运行。' + "`r`n" +
             '【EXECUTION】执行请求在传统 S3 睡眠系统上也可能阻止自动休眠；在现代待机系统上行为不同。' + "`r`n" +
             '【PERFBOOST / ACTIVELOCKSCREEN】其他电源请求。' + "`r`n`r`n" +
             '“上次唤醒”说明上一轮唤醒可能原因；“唤醒定时器”用于排查计划唤醒。' + "`r`n" +
             '选中 [PROCESS] 普通程序后可点击“关闭所选程序”；只尝试关闭经完整路径匹配的进程。' + "`r`n" +
             '每次需要确认；若正常关闭失败，再次确认后才允许强制结束。驱动和服务不提供结束功能。' + "`r`n" +
             '本工具不会修改电源设置、关闭驱动或系统服务、添加电源请求豁免。' + "`r`n" +
             '按 F5 可刷新。需要管理员权限，首次打开会出现 Windows UAC 提示。' + "`r`n`r`n" +
             '提醒：没有活动请求不保证电脑能正常休眠。'),
            '诊断说明',
            [System.Windows.Forms.MessageBoxButtons]::OK,
            [System.Windows.Forms.MessageBoxIcon]::Information
        )
    })
    $combo.Add_SelectedIndexChanged({ if ($null -ne $script:state.Parsed) { Update-Grid } })
    $grid.Add_SelectionChanged({
        $script:ui.CloseButton.Enabled = $false
        if ($grid.SelectedRows.Count -gt 0) {
            $entry = $grid.SelectedRows[0].Tag
            if ($null -ne $entry) {
                $script:ui.CloseButton.Enabled = ($entry.Kind -eq 'PROCESS' -and $script:ui.RefreshButton.Enabled)
                $description = if ([string]::IsNullOrWhiteSpace($entry.Detail)) { '（未附带说明）' } else { $entry.Detail }
                $details.Text = '请求类别：' + $entry.Section + '  |  类型：' + $entry.KindText + "`r`n" +
                    '对象：' + $entry.Name + "`r`n" + '说明：' + $description
            }
        }
    })
    $form.Add_KeyDown({ param($s,$e)
        if ($e.KeyCode -eq [System.Windows.Forms.Keys]::F5) {
            Refresh-Inspector
            $e.Handled = $true
        }
    })
    $form.Add_Shown({ Refresh-Inspector })
    [void][System.Windows.Forms.Application]::Run($form)
} catch {
    $message = $_.Exception.Message + "`r`n" + $_.ScriptStackTrace
    try {
        $errorPath = Join-Path ([System.IO.Path]::GetTempPath()) 'SleepInspector-error.log'
        [System.IO.File]::AppendAllText(
            $errorPath, ((Get-Date).ToString('s') + "`r`n" + $message + "`r`n`r`n"),
            [System.Text.UTF8Encoding]::new($true)
        )
        $message += "`r`n`r`n错误日志：" + $errorPath
    } catch { }
    Show-UiError $message '检测器启动失败'
}
