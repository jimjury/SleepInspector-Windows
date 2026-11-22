package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type PowerRequest struct {
	Section string
	Kind    string
	Name    string
	Detail  string
}

type ScanResult struct {
	When    time.Time
	Items   []PowerRequest
	Raw     string
	Unknown []string
	Err     string
}

var reqLine = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*(.*?)\s*$`)
var headerLine = regexp.MustCompile(`^\s*([^:：]+)\s*[:：]\s*$`)

func sectionFromLine(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DISPLAY", "显示", "显示器":
		return "DISPLAY"
	case "SYSTEM", "系统":
		return "SYSTEM"
	case "AWAYMODE", "离开模式":
		return "AWAYMODE"
	case "EXECUTION", "执行":
		return "EXECUTION"
	case "PERFBOOST", "性能提升":
		return "PERFBOOST"
	case "ACTIVELOCKSCREEN", "锁定屏幕", "活动锁定屏幕":
		return "ACTIVELOCKSCREEN"
	}
	return ""
}
func kindFromLine(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "PROCESS", "进程":
		return "PROCESS"
	case "DRIVER", "驱动", "驱动程序":
		return "DRIVER"
	case "SERVICE", "服务":
		return "SERVICE"
	}
	return "UNKNOWN"
}
func isNone(s string) bool {
	x := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), ".。!"))
	switch strings.ToLower(x) {
	case "none", "无", "没有", "无请求", "无活动请求", "无任何请求":
		return true
	}
	return false
}
func ParseRequests(output string) ScanResult {
	result := ScanResult{Raw: output, When: time.Now(), Items: []PowerRequest{}}
	var section string
	seenSection := false
	var active *PowerRequest
	flush := func() {
		if active != nil {
			result.Items = append(result.Items, *active)
			active = nil
		}
	}
	for _, rawline := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawline)
		if line == "" {
			continue
		}
		if m := headerLine.FindStringSubmatch(line); m != nil {
			if newSection := sectionFromLine(m[1]); newSection != "" {
				flush()
				section = newSection
				seenSection = true
				continue
			}
		}
		if section == "" {
			result.Unknown = append(result.Unknown, line)
			continue
		}
		if m := reqLine.FindStringSubmatch(line); m != nil {
			kind := kindFromLine(m[1])
			if kind != "UNKNOWN" {
				flush()
				active = &PowerRequest{Section: section, Kind: kind, Name: strings.TrimSpace(m[2])}
				continue
			}
		}
		if isNone(line) {
			continue
		}
		if active != nil {
			if active.Detail != "" {
				active.Detail += "\n"
			}
			active.Detail += line
		} else {
			result.Unknown = append(result.Unknown, fmt.Sprintf("%s: %s", section, line))
		}
	}
	flush()
	if !seenSection && strings.TrimSpace(output) != "" {
		result.Err = "无法识别 powercfg 输出中的请求分类；请核对原始命令输出。"
	}
	return result
}
func impactText(section string) string {
	switch section {
	case "SYSTEM":
		return "阻止自动休眠"
	case "DISPLAY":
		return "阻止屏幕关闭"
	case "AWAYMODE":
		return "请求离开模式"
	case "EXECUTION":
		return "执行请求"
	case "PERFBOOST":
		return "性能请求"
	case "ACTIVELOCKSCREEN":
		return "锁屏请求"
	}
	return "其他请求"
}
func kindText(kind string) string {
	switch kind {
	case "PROCESS":
		return "程序 / 进程"
	case "DRIVER":
		return "驱动程序"
	case "SERVICE":
		return "系统服务"
	}
	return "其他来源"
}
func countRequests(items []PowerRequest) (system, display, other int) {
	for _, item := range items {
		switch item.Section {
		case "SYSTEM":
			system++
		case "DISPLAY":
			display++
		default:
			other++
		}
	}
	return
}
func reportText(s ScanResult, lastwake, waketimers string, ops []string) string {
	var b strings.Builder
	b.WriteString("SleepInspector 休眠诊断报告\r\n")
	b.WriteString("检测时间：" + s.When.Format("2006-01-02 15:04:05") + "\r\n")
	a, d, o := countRequests(s.Items)
	b.WriteString(fmt.Sprintf("自动休眠阻止项：%d    屏幕阻止项：%d    其他请求：%d\r\n", a, d, o))
	if s.Err != "" {
		b.WriteString("检测提示：" + s.Err + "\r\n")
	}
	b.WriteString("\r\n--- 当前请求 / powercfg /requests ---\r\n")
	b.WriteString(s.Raw + "\r\n")
	b.WriteString("\r\n--- 上次唤醒 / powercfg /lastwake ---\r\n")
	if lastwake == "" {
		b.WriteString("（尚未查询）")
	} else {
		b.WriteString(lastwake)
	}
	b.WriteString("\r\n\r\n--- 唤醒定时器 / powercfg /waketimers ---\r\n")
	if waketimers == "" {
		b.WriteString("（尚未查询）")
	} else {
		b.WriteString(waketimers)
	}
	if len(ops) > 0 {
		b.WriteString("\r\n\r\n--- 本次操作 ---\r\n")
		for _, v := range ops {
			b.WriteString(v + "\r\n")
		}
	}
	return b.String()
}
