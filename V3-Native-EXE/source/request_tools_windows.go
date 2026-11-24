//go:build windows

package main

import "fmt"

// This opens standard Windows UI tools only after explicit user confirmation.
// It never disables a driver, stops a service or changes power-request overrides.
func launchBuiltInWindowsTool(tool string) error {
	result := call(pShellExecute, ui.hwnd, sPtr("open"), sPtr(tool), 0, 0, SW_SHOWNORMAL)
	if result <= 32 {
		return fmt.Errorf("无法打开 Windows 工具 %s（错误代码：%d）", tool, result)
	}
	return nil
}

func inspectNonProcess(item PowerRequest, action RequestAction) {
	switch action {
	case ActionInspectAudio:
		instructions := "当前选中的是音频驱动的 SYSTEM 请求，不是可直接结束的普通程序。\n\n" +
			"来源：" + item.Name + "\n\n" +
			"可能是浏览器、播放器、通话软件或其他音频会话间接使驱动保持活动。仅凭 powercfg 无法确定实际占用音频的 PID。\n\n" +
			"是否打开【音量合成器】和【任务管理器】？\n\n" +
			"你可以在音量合成器里识别音频应用，再在应用里正常退出或在任务管理器手动结束明确识别的程序。随后返回本工具点击“立即检测”。\n\n" +
			"注意：静音不一定解除休眠阻止；本工具不会停用 Realtek 驱动或自动结束任何进程。"
		if messagebox(ui.hwnd, instructions, "音频驱动占用排查", MB_YESNO|MB_ICONINFORMATION|MB_DEFBUTTON2) != IDYES {
			return
		}
		mixerOpened, taskManagerOpened := false, false
		if err := launchBuiltInWindowsTool("SndVol.exe"); err != nil {
			// Windows versions without legacy mixer still offer a Settings page.
			if fallbackErr := launchBuiltInWindowsTool("ms-settings:apps-volume"); fallbackErr != nil {
				messagebox(ui.hwnd, err.Error()+"\n"+fallbackErr.Error(), "音量合成器未打开", MB_ICONWARNING)
			} else {
				mixerOpened = true
			}
		} else {
			mixerOpened = true
		}
		if err := launchBuiltInWindowsTool("taskmgr.exe"); err != nil {
			messagebox(ui.hwnd, err.Error(), "任务管理器未打开", MB_ICONWARNING)
		} else {
			taskManagerOpened = true
		}
		if mixerOpened || taskManagerOpened {
			ui.info = "音频排查工具已启动；识别应用并正常退出后，请点击“立即检测”检查是否解除。"
		} else {
			ui.info = "无法启动音频排查工具，请手动打开音量合成器与任务管理器。"
		}
		invalidate(ui.hwnd)
	case ActionInspectDriver:
		text := "该阻止项来自驱动程序，而不是普通进程：\n\n" + item.Name + "\n\n" +
			"程序不能直接结束驱动。是否打开 Windows 设备管理器查看设备信息与驱动状态？\n\n" +
			"不建议为了休眠而直接禁用设备、卸载驱动或添加 powercfg 豁免。"
		if messagebox(ui.hwnd, text, "驱动请求排查", MB_YESNO|MB_ICONINFORMATION|MB_DEFBUTTON2) == IDYES {
			if err := launchBuiltInWindowsTool("devmgmt.msc"); err != nil {
				messagebox(ui.hwnd, err.Error(), "无法打开设备管理器", MB_ICONWARNING)
			}
		}
	case ActionInspectService:
		text := "该阻止项由 Windows 服务提出：\n\n" + item.Name + "\n\n" +
			"不是一个可以安全按名称结束的普通进程。是否打开 Windows 服务管理器查看它的说明与依赖关系？\n\n" +
			"本工具不会自行停止服务；停止系统服务可能导致其他功能中断。"
		if messagebox(ui.hwnd, text, "服务请求排查", MB_YESNO|MB_ICONINFORMATION|MB_DEFBUTTON2) == IDYES {
			if err := launchBuiltInWindowsTool("services.msc"); err != nil {
				messagebox(ui.hwnd, err.Error(), "无法打开服务管理器", MB_ICONWARNING)
			}
		}
	default:
		messagebox(ui.hwnd, "该请求没有可安全执行的关联处理操作。", "无法处理该来源", MB_ICONINFORMATION)
	}
}
