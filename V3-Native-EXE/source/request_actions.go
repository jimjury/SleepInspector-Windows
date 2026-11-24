package main

import "strings"

// Selected blocker kinds have different, explicitly bounded remedies.
// A driver request is not a process and is never passed to TerminateProcess.
type RequestAction string

const (
	ActionNone           RequestAction = "none"
	ActionCloseProcess   RequestAction = "close_process"
	ActionInspectAudio   RequestAction = "inspect_audio"
	ActionInspectDriver  RequestAction = "inspect_driver"
	ActionInspectService RequestAction = "inspect_service"
)

func classifyAction(item PowerRequest) RequestAction {
	switch strings.ToUpper(strings.TrimSpace(item.Kind)) {
	case "PROCESS":
		return ActionCloseProcess
	case "DRIVER":
		if isAudioDeviceRequest(item.Name) {
			return ActionInspectAudio
		}
		return ActionInspectDriver
	case "SERVICE":
		return ActionInspectService
	default:
		return ActionNone
	}
}

func isAudioDeviceRequest(name string) bool {
	lower := strings.ToLower(name)
	for _, term := range []string{"realtek", "hdaudio", "high definition audio", "usb audio", "bluetooth audio", "audio", "sound", "扬声器", "音频", "声卡", "耳机", "麦克风"} {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func actionTitle(action RequestAction) string {
	switch action {
	case ActionCloseProcess:
		return "关闭所选程序"
	case ActionInspectAudio:
		return "排查音频占用"
	case ActionInspectDriver:
		return "排查驱动请求"
	case ActionInspectService:
		return "查看服务处理"
	default:
		return "处理所选项"
	}
}

func actionDescription(action RequestAction) string {
	switch action {
	case ActionCloseProcess:
		return "这是普通程序请求。可按完整路径匹配进程；关闭前会核对进程身份并要求确认。"
	case ActionInspectAudio:
		return "这是音频驱动请求，不是普通程序。可查看音量合成器中的音频应用并逐一正常关闭，再返回刷新；不能直接结束驱动。"
	case ActionInspectDriver:
		return "这是设备驱动请求。可打开设备管理器调查驱动状态；此工具不会禁用或结束设备驱动。"
	case ActionInspectService:
		return "这是系统服务请求。可打开服务管理器查看名称、状态与依赖；此工具不会停止系统服务。"
	}
	return "当前项目无法直接处理。"
}
