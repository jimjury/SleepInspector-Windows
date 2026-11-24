package main

import (
	"os"
	"strings"
	"testing"
)

// Source-level contract assertions supplement compile tests when Win32 UI execution
// is unavailable on the Linux build host. They are not pixel-level UI tests.
func TestSelectionButtonAndProcessSafetyContracts(t *testing.T) {
	ui, err := os.ReadFile("ui_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	tools, err := os.ReadFile("request_tools_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(ui)
	w := string(tools)
	for _, token := range []string{
		"action := ActionNone", "action = classifyAction(item)", "title := actionTitle(action)",
		"enabled(ui.buttons[ctlClose], action != ActionNone", "inspectNonProcess(item, action)",
		"if action != ActionCloseProcess", "case ctlClose:", "requestClose()",
		"hdr.Code == listViewItemChanged", "ui.selectionPending", "pSetBkMode, wparam, OPAQUE",
	} {
		if !strings.Contains(s, token) {
			t.Errorf("missing UI invariant: %s", token)
		}
	}
	for _, token := range []string{
		"ActionInspectAudio", "SndVol.exe", "taskmgr.exe", "ActionInspectDriver", "devmgmt.msc",
		"ActionInspectService", "services.msc", "MB_YESNO", "SW_SHOWNORMAL",
	} {
		if !strings.Contains(w, token) {
			t.Errorf("missing driver/service route: %s", token)
		}
	}
	for _, prohibited := range []string{"/requestsoverride", "TerminateProcess", "Stop-Service", "taskkill", "sc stop", "devcon disable"} {
		if strings.Contains(w, prohibited) {
			t.Errorf("non-process path contains unsafe action: %s", prohibited)
		}
	}
}
