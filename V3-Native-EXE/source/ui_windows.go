//go:build windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	ctlScan          = 101
	ctlClose         = 102
	ctlCopy          = 103
	ctlSave          = 104
	ctlHelp          = 105
	ctlFilter        = 106
	ctlList          = 107
	ctlDetail        = 108
	ctlLog           = 109
	ctlTabRaw        = 201
	ctlTabWake       = 202
	ctlTabTimers     = 203
	ctlTabReport     = 204
	chooserList      = 510
	chooserOK        = 511
	chooserCancel    = 512
	WM_HOTKEY        = 0x0312
	WM_KEYDOWN       = 0x0100
	VK_F5            = 0x74
	WM_MOUSEWHEEL    = 0x020A
	WM_APP_SELECTION = WM_APP + 7
)

type theme struct{ Bg, White, Text, Muted, Border, Primary, LightBlue, Red, RedLight, Amber, AmberLight, Green, GreenLight, LightGray, Shadow, DarkBlue uintptr }

var palette = theme{
	Bg: rgb(245, 247, 251), White: rgb(255, 255, 255), Text: rgb(31, 41, 55), Muted: rgb(111, 124, 143), Border: rgb(226, 232, 240), Primary: rgb(45, 105, 223), LightBlue: rgb(238, 244, 255), Red: rgb(207, 65, 68), RedLight: rgb(255, 242, 241), Amber: rgb(201, 113, 33), AmberLight: rgb(255, 246, 233), Green: rgb(30, 142, 101), GreenLight: rgb(234, 250, 241), LightGray: rgb(248, 250, 253), Shadow: rgb(236, 240, 245), DarkBlue: rgb(33, 69, 148),
}

type buttonCfg struct {
	Title string
	Kind  string
	Tab   int
}

// The listview sends WM_NOTIFY for painting, mouse hover, selection, and more.
// Only LVN_ITEMCHANGED can affect the selected detail panel.
type notifyHeader struct {
	HwndFrom uintptr
	IDFrom   uintptr
	Code     int32
	_        uint32
}
type notifyListView struct {
	Header                      notifyHeader
	Item, SubItem               int32
	NewState, OldState, Changed uint32
	Action                      point
	Param                       uintptr
}
type scanMsg struct{ Result ScanResult }
type tabMsg struct {
	Index int
	Text  string
	Err   error
}
type closeVerifyMsg struct {
	OK     bool
	Reason string
}
type closeGraceMsg struct {
	Exited  bool
	Windows int
}
type closeForcedMsg struct{ Err error }
type appUI struct {
	hwnd, lv, details, logEdit, filter                                        uintptr
	buttons                                                                   map[int]uintptr
	fontNormal, fontBtn, fontHead, fontLarge, fontMetric, fontSmall, fontCard uintptr
	whiteBrush                                                                uintptr
	padding                                                                   int32
	width, height                                                             int32
	scan                                                                      ScanResult
	display                                                                   []int
	lastwake, waketimers                                                      string
	tab                                                                       int
	filterChoice                                                              int
	selected                                                                  int
	scanning, closing                                                         bool
	info                                                                      string
	operation                                                                 []string
	proc                                                                      candidate
	requested                                                                 PowerRequest
	pendingScan                                                               *scanMsg
	pendingTab                                                                *tabMsg
	pendingVerify                                                             *closeVerifyMsg
	pendingGrace                                                              *closeGraceMsg
	pendingForced                                                             *closeForcedMsg
	populating, selectionPending                                              bool
	lastDetailText, lastLogText                                               string
	lastActionTitle                                                           string
	mu                                                                        sync.Mutex
}

var ui appUI
var childMenu = map[int]buttonCfg{
	ctlScan: {"立即检测 (F5)", "primary", -1}, ctlClose: {"处理所选项", "danger", -1}, ctlCopy: {"复制完整报告", "subtle", -1}, ctlSave: {"保存日志", "subtle", -1}, ctlHelp: {"使用说明", "subtle", -1},
	ctlTabRaw: {"原始请求", "tab", 0}, ctlTabWake: {"上次唤醒", "tab", 1}, ctlTabTimers: {"唤醒定时器", "tab", 2}, ctlTabReport: {"完整报告", "tab", 3},
}

func scale(n int32) int32 { return n * ui.padding / 100 }
func initFonts() {
	dpi := call(pGetDpiForWindow, ui.hwnd)
	if dpi < 80 || dpi > 384 {
		dpi = 96
	}
	// Control fonts adapt to DPI; logical spacing uses the same scaling.
	ui.padding = int32(dpi * 100 / 96)
	if ui.padding > 120 {
		ui.padding = 120
	}
	f := func(n int) int { return int(int32(n) * ui.padding / 100) }
	ui.fontNormal = makeFont(f(15), 400, "Microsoft YaHei UI")
	ui.fontBtn = makeFont(f(15), 500, "Microsoft YaHei UI")
	ui.fontHead = makeFont(f(23), 700, "Microsoft YaHei UI")
	ui.fontLarge = makeFont(f(30), 700, "Segoe UI")
	ui.fontMetric = makeFont(f(17), 600, "Microsoft YaHei UI")
	ui.fontSmall = makeFont(f(13), 400, "Microsoft YaHei UI")
	ui.fontCard = makeFont(f(16), 600, "Microsoft YaHei UI")
	ui.whiteBrush = call(pCreateSolidBrush, palette.White)
}
func createUI(hwnd uintptr) {
	ui.hwnd = hwnd
	ui.buttons = make(map[int]uintptr)
	ui.selected = -1
	ui.tab = 0
	initFonts()
	init := initCommon{Size: uint32(unsafe.Sizeof(initCommon{})), Flags: ICC_LISTVIEW_CLASSES}
	call(pInitCommon, uintptr(unsafe.Pointer(&init)))
	for id, cfg := range childMenu {
		child := createControl("BUTTON", cfg.Title, BS_OWNERDRAW|WS_TABSTOP, 0, int32(id), hwnd)
		ui.buttons[id] = child
		uiFont(child, ui.fontBtn)
	}
	ui.filter = createControl("COMBOBOX", "", CBS_DROPDOWNLIST|CBS_HASSTRINGS|WS_TABSTOP|WS_VSCROLL, 0, ctlFilter, hwnd)
	uiFont(ui.filter, ui.fontNormal)
	for _, name := range []string{"全部请求", "阻止自动休眠", "阻止屏幕关闭", "其他请求"} {
		send(ui.filter, CB_ADDSTRING, 0, sPtr(name))
	}
	send(ui.filter, CB_SETCURSEL, 0, 0)
	ui.lv = createControl("SysListView32", "", LVS_REPORT|LVS_SINGLESEL|LVS_SHOWSELALWAYS|LVS_NOSORTHEADER|WS_TABSTOP, 0, ctlList, hwnd)
	uiFont(ui.lv, ui.fontNormal)
	send(ui.lv, LVM_SETEXTENDEDLISTVIEWSTYLE, LVS_EX_FULLROWSELECT|LVS_EX_DOUBLEBUFFER|LVS_EX_LABELTIP, LVS_EX_FULLROWSELECT|LVS_EX_DOUBLEBUFFER|LVS_EX_LABELTIP)
	send(ui.lv, LVM_SETBKCOLOR, 0, palette.White)
	send(ui.lv, LVM_SETTEXTBKCOLOR, 0, palette.White)
	send(ui.lv, LVM_SETTEXTCOLOR, 0, palette.Text)
	for i, name := range []string{"影响", "来源", "对象 / 进程路径", "类别"} {
		col := lvColumn{Mask: LVCF_TEXT | LVCF_WIDTH | LVCF_SUBITEM, Width: 100, Text: utfPtr(name), Subitem: int32(i)}
		send(ui.lv, LVM_INSERTCOLUMNW, uintptr(i), uintptr(unsafe.Pointer(&col)))
	}
	ui.details = createControl("EDIT", "选中检测结果后，这里将显示完整路径和详细信息。", ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY|WS_VSCROLL, 0, ctlDetail, hwnd)
	uiFont(ui.details, ui.fontNormal)
	ui.logEdit = createControl("EDIT", "正在准备检测...", ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY|ES_WANTRETURN|WS_VSCROLL, 0, ctlLog, hwnd)
	uiFont(ui.logEdit, ui.fontNormal)
	// Add consistent text padding. With no WS_HSCROLL, long powercfg paths wrap.
	for _, child := range []uintptr{ui.details, ui.logEdit} {
		send(child, EM_SETMARGINS, EC_LEFTMARGIN|EC_RIGHTMARGIN, uintptr(uint32(scale(9))|uint32(scale(9))<<16))
	}
	enabled(ui.buttons[ctlClose], false)
	ui.info = "就绪 · 点击立即检测查看当前休眠请求"
	refreshLayout()
}
func refreshLayout() {
	if ui.hwnd == 0 {
		return
	}
	cr := windowRect(ui.hwnd)
	ui.width = cr.w()
	ui.height = cr.h()
	p := scale(24)
	gap := scale(12)
	toolY := scale(224)
	btnY := toolY + scale(16)
	btnH := scale(38)
	left := p + scale(14)
	btnInfo := []struct {
		id int
		w  int32
	}{{ctlScan, 150}, {ctlClose, 157}, {ctlCopy, 146}, {ctlSave, 110}, {ctlHelp, 110}}
	for _, b := range btnInfo {
		w := scale(b.w)
		move(ui.buttons[b.id], left, btnY, w, btnH)
		left += w + scale(9)
	}
	comboW := scale(168)
	move(ui.filter, ui.width-p-scale(16)-comboW, btnY+scale(1), comboW, scale(250))
	listTop := scale(359)
	tabY := ui.height - scale(265)
	if tabY < scale(495) {
		tabY = scale(495)
	}
	listBottom := tabY - scale(17)
	boxLeft := p + scale(12)
	detailW := scale(340)
	if ui.width < scale(1160) {
		detailW = scale(292)
	}
	detailLeft := ui.width - p - detailW
	listRight := detailLeft - gap
	listH := listBottom - listTop - scale(52)
	if listH < scale(80) {
		listH = scale(80)
	}
	listX := boxLeft
	listW := listRight - listX - scale(12)
	move(ui.lv, listX, listTop+scale(40), listW, listH)
	colWidth := listW - scale(24) - scale(119) - scale(104) - scale(83)
	if colWidth < scale(180) {
		colWidth = scale(180)
	}
	for i, w := range []int32{scale(119), scale(104), colWidth, scale(83)} {
		send(ui.lv, LVM_SETCOLUMNWIDTH, uintptr(i), uintptr(w))
	}
	move(ui.details, detailLeft+scale(18), listTop+scale(73), detailW-scale(36), listH-scale(39))
	tabsX := p + scale(14)
	for _, tb := range []struct {
		id int
		w  int32
	}{{ctlTabRaw, 112}, {ctlTabWake, 110}, {ctlTabTimers, 143}, {ctlTabReport, 112}} {
		move(ui.buttons[tb.id], tabsX, tabY+scale(11), scale(tb.w), scale(38))
		tabsX += scale(tb.w + 8)
	}
	bottomY := tabY + scale(62)
	bottomH := ui.height - bottomY - scale(41)
	if bottomH < scale(60) {
		bottomH = scale(60)
	}
	move(ui.logEdit, p+scale(14), bottomY, ui.width-2*p-scale(28), bottomH)
	invalidate(ui.hwnd)
}
func drawRound(hdc uintptr, r rect, fill, stroke uintptr, rad int32) {
	brush := call(pCreateSolidBrush, fill)
	pen := call(pCreatePen, PS_SOLID, 1, stroke)
	oldB := call(pSelectObject, hdc, brush)
	oldP := call(pSelectObject, hdc, pen)
	call(pRoundRect, hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(rad), uintptr(rad))
	call(pSelectObject, hdc, oldP)
	call(pSelectObject, hdc, oldB)
	call(pDeleteObject, brush)
	call(pDeleteObject, pen)
}
func drawText(hdc uintptr, text string, r rect, font, color uintptr, flags uintptr) {
	old := call(pSelectObject, hdc, font)
	call(pSetBkMode, hdc, TRANSPARENT)
	call(pSetTextColor, hdc, color)
	call(pDrawText, hdc, sPtr(text), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags|DT_NOPREFIX)
	call(pSelectObject, hdc, old)
}
func makeR(x, y, w, h int32) rect   { return rect{x, y, x + w, y + h} }
func drawPanel(hdc uintptr, r rect) { drawRound(hdc, r, palette.White, palette.Border, scale(14)) }
func headerAndCards(hdc uintptr) {
	p := scale(24)
	w := ui.width - 2*p
	// Brand identity and page description.
	drawRound(hdc, makeR(p, scale(23), scale(41), scale(41)), palette.Primary, palette.Primary, scale(11))
	drawText(hdc, "Z", makeR(p+scale(2), scale(27), scale(37), scale(34)), ui.fontHead, palette.White, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "休眠阻止项检测器", makeR(p+scale(57), scale(20), scale(450), scale(35)), ui.fontHead, palette.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "SleepInspector  /  Windows 电源诊断", makeR(p+scale(58), scale(60), scale(530), scale(22)), ui.fontSmall, palette.Muted, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	badgeR := makeR(ui.width-p-scale(190), scale(35), scale(185), scale(31))
	drawRound(hdc, badgeR, palette.GreenLight, palette.GreenLight, scale(16))
	drawText(hdc, "● 只读检测 · 安全关闭", badgeR, ui.fontSmall, palette.Green, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	// Statistics panel cards.
	cardY := scale(104)
	cardH := scale(103)
	gap := scale(14)
	cardW := (w - 2*gap) / 3
	a, b, c := countRequests(ui.scan.Items)
	type stat struct {
		caption            string
		count              int
		detail             string
		accent, background uintptr
	}
	data := []stat{{"阻止自动休眠", a, "SYSTEM · 影响自动休眠", palette.Red, palette.RedLight}, {"阻止屏幕关闭", b, "DISPLAY · 仅影响屏幕", palette.Primary, palette.LightBlue}, {"其他电源请求", c, "执行 / 离开模式等", palette.Amber, palette.AmberLight}}
	for i, s := range data {
		x := p + int32(i)*(cardW+gap)
		r := makeR(x, cardY, cardW, cardH)
		drawPanel(hdc, r)
		drawRound(hdc, makeR(x+scale(17), cardY+scale(18), scale(5), scale(65)), s.accent, s.accent, scale(4))
		drawText(hdc, s.caption, makeR(x+scale(36), cardY+scale(11), cardW-scale(130), scale(30)), ui.fontCard, palette.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, fmt.Sprintf("%d", s.count), makeR(x+cardW-scale(85), cardY+scale(9), scale(63), scale(50)), ui.fontLarge, s.accent, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
		drawText(hdc, s.detail, makeR(x+scale(36), cardY+scale(62), cardW-scale(49), scale(27)), ui.fontSmall, palette.Muted, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	}
	// Toolbar panel.
	drawPanel(hdc, makeR(p, scale(224), w, scale(70)))
	drawText(hdc, "操作", makeR(p+scale(15), scale(207), scale(75), scale(19)), ui.fontSmall, palette.Muted, DT_LEFT|DT_SINGLELINE)
	// Contents.
	sectionY := scale(311)
	drawText(hdc, "当前阻止项", makeR(p+scale(3), sectionY, w/2, scale(33)), ui.fontMetric, palette.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	statusLabel := "检查中..."
	if !ui.scanning {
		if ui.scan.When.IsZero() {
			statusLabel = "尚未检测"
		} else if ui.scan.Err != "" {
			statusLabel = "检测未完成"
		} else {
			statusLabel = "检测时间  " + ui.scan.When.Format("15:04:05")
		}
	}
	drawText(hdc, statusLabel, makeR(ui.width-p-scale(300), sectionY, scale(296), scale(30)), ui.fontSmall, palette.Muted, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
	tabY := ui.height - scale(265)
	if tabY < scale(495) {
		tabY = scale(495)
	}
	paneTop := scale(347)
	paneBottom := tabY - scale(17)
	detailW := scale(340)
	if ui.width < scale(1160) {
		detailW = scale(292)
	}
	detailLeft := ui.width - p - detailW
	drawPanel(hdc, makeR(p, paneTop, detailLeft-p-scale(13), paneBottom-paneTop))
	drawPanel(hdc, makeR(detailLeft, paneTop, detailW, paneBottom-paneTop))
	drawText(hdc, "检测结果", makeR(p+scale(18), paneTop+scale(9), scale(165), scale(26)), ui.fontCard, palette.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, fmt.Sprintf("共 %d 项", len(ui.display)), makeR(detailLeft-scale(103), paneTop+scale(10), scale(75), scale(26)), ui.fontSmall, palette.Muted, DT_RIGHT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "选中项详情", makeR(detailLeft+scale(17), paneTop+scale(11), detailW-scale(34), scale(25)), ui.fontCard, palette.Text, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, "展示完整路径及请求说明", makeR(detailLeft+scale(17), paneTop+scale(38), detailW-scale(34), scale(25)), ui.fontSmall, palette.Muted, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	// Diagnostics section.
	drawPanel(hdc, makeR(p, tabY, w, ui.height-tabY-scale(15)))
	// Bottom status line.
	label := ui.info
	if label == "" {
		label = "就绪"
	}
	drawText(hdc, label, makeR(p+scale(13), ui.height-scale(35), ui.width-2*p-scale(32), scale(20)), ui.fontSmall, palette.Muted, DT_LEFT|DT_VCENTER|DT_END_ELLIPSIS|DT_SINGLELINE)
}
func paintUI(hwnd uintptr) {
	var ps paintStruct
	hdc := call(pBeginPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	r := windowRect(hwnd)
	w, h := r.w(), r.h()
	mem := call(pCreateCompatibleDC, hdc)
	bmp := call(pCreateCompatibleBitmap, hdc, uintptr(w), uintptr(h))
	if mem != 0 && bmp != 0 {
		old := call(pSelectObject, mem, bmp)
		brush := call(pCreateSolidBrush, palette.Bg)
		call(pFillRect, mem, uintptr(unsafe.Pointer(&r)), brush)
		call(pDeleteObject, brush)
		headerAndCards(mem)
		call(pBitBlt, hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, 0x00CC0020)
		call(pSelectObject, mem, old)
	}
	if bmp != 0 {
		call(pDeleteObject, bmp)
	}
	if mem != 0 {
		call(pDeleteDC, mem)
	}
	call(pEndPaint, hwnd, uintptr(unsafe.Pointer(&ps)))
}
func paintButton(p *drawItem) {
	id := int(p.CtlID)
	cfg, ok := childMenu[id]
	if !ok {
		return
	}
	r := p.Rc
	active := true
	if p.State&ODS_DISABLED != 0 {
		active = false
	}
	selected := cfg.Tab >= 0 && cfg.Tab == ui.tab
	fill, stroke, textColor := palette.White, palette.Border, palette.Text
	switch cfg.Kind {
	case "primary":
		fill, stroke, textColor = palette.Primary, palette.Primary, palette.White
	case "danger":
		fill, stroke, textColor = palette.RedLight, palette.RedLight, palette.Red
	case "tab":
		fill, stroke, textColor = palette.White, palette.White, palette.Muted
		if selected {
			fill, stroke, textColor = palette.LightBlue, palette.LightBlue, palette.Primary
		}
	}
	if !active {
		fill, stroke, textColor = palette.LightGray, palette.Border, palette.Muted
	}
	if p.State&ODS_SELECTED != 0 && active {
		if cfg.Kind == "primary" {
			fill = palette.DarkBlue
		} else if cfg.Kind != "tab" {
			fill = palette.LightBlue
		}
	}
	drawRound(p.Hdc, rect{r.Left + 1, r.Top + 1, r.Right - 1, r.Bottom - 1}, fill, stroke, scale(9))
	drawText(p.Hdc, cfg.Title, rect{r.Left + scale(5), r.Top, r.Right - scale(5), r.Bottom}, ui.fontBtn, textColor, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
}
func selectedItem() (PowerRequest, bool) {
	idx := int(int32(uint32(send(ui.lv, LVM_GETNEXTITEM, ^uintptr(0), LVNI_SELECTED))))
	if idx < 0 || idx >= len(ui.display) {
		return PowerRequest{}, false
	}
	index := ui.display[idx]
	if index < 0 || index >= len(ui.scan.Items) {
		return PowerRequest{}, false
	}
	return ui.scan.Items[index], true
}
func updateDetails() {
	item, ok := selectedItem()
	text := "选择左侧表格中的一个项目，可在这里查看完整的可执行文件路径及详细请求说明。\r\n\r\n说明：SYSTEM 会阻止自动休眠，DISPLAY 通常只影响屏幕自动关闭。"
	if ok {
		t := "影响：" + impactText(item.Section) + "\r\n\r\n来源：" + kindText(item.Kind) + "\r\n\r\n完整对象路径：\r\n" + item.Name
		if item.Detail != "" {
			t += "\r\n\r\n补充信息：\r\n" + item.Detail
		}
		t += "\r\n\r\n处理建议：\r\n" + actionDescription(classifyAction(item))
		text = t
	}
	// Avoid needlessly resetting the read-only EDIT control on every notification.
	if text != ui.lastDetailText {
		setControlText(ui.details, text)
		// Erase stale text pixels before drawing the new content.
		call(pInvalidateRect, ui.details, 0, 1)
		ui.lastDetailText = text
	}
	action := ActionNone
	if ok {
		action = classifyAction(item)
	}
	title := actionTitle(action)
	if ui.lastActionTitle != title {
		cfg := childMenu[ctlClose]
		cfg.Title = title
		if action == ActionCloseProcess {
			cfg.Kind = "danger"
		} else {
			cfg.Kind = "subtle"
		}
		childMenu[ctlClose] = cfg
		ui.lastActionTitle = title
		// Owner-drawn buttons use childMenu text; redraw only the affected button.
		invalidate(ui.buttons[ctlClose])
	}
	enabled(ui.buttons[ctlClose], action != ActionNone && !ui.closing && !ui.scanning)
	// No parent invalidation: listview paint notifications must never repaint the whole UI.
}
func populateList() {
	ui.populating = true
	defer func() { ui.populating = false }()
	send(ui.lv, LVM_DELETEALLITEMS, 0, 0)
	ui.display = nil
	for i, item := range ui.scan.Items {
		match := false
		switch ui.filterChoice {
		case 0:
			match = true
		case 1:
			match = item.Section == "SYSTEM"
		case 2:
			match = item.Section == "DISPLAY"
		case 3:
			match = item.Section != "SYSTEM" && item.Section != "DISPLAY"
		}
		if !match {
			continue
		}
		index := len(ui.display)
		ui.display = append(ui.display, i)
		cols := []string{impactText(item.Section), kindText(item.Kind), item.Name, item.Section}
		first := lvItem{Mask: LVIF_TEXT, Item: int32(index), Subitem: 0, Text: utfPtr(cols[0])}
		send(ui.lv, LVM_INSERTITEMW, 0, uintptr(unsafe.Pointer(&first)))
		for c := 1; c < len(cols); c++ {
			v := lvItem{Subitem: int32(c), Text: utfPtr(cols[c])}
			send(ui.lv, LVM_SETITEMTEXTW, uintptr(index), uintptr(unsafe.Pointer(&v)))
		}
	}
	updateDetails()
}
func updateLog() {
	var body string
	switch ui.tab {
	case 0:
		body = ui.scan.Raw
		if body == "" {
			body = "尚无检测结果。点击“立即检测”开始。"
		}
		if ui.scan.Err != "" {
			body = "检测异常：" + ui.scan.Err + "\r\n\r\n" + body
		}
	case 1:
		body = ui.lastwake
		if body == "" {
			body = "正在读取上次唤醒信息..."
		}
	case 2:
		body = ui.waketimers
		if body == "" {
			body = "正在读取唤醒定时器..."
		}
	case 3:
		body = reportText(ui.scan, ui.lastwake, ui.waketimers, ui.operation)
	}
	text := strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")
	if text != ui.lastLogText {
		setControlText(ui.logEdit, text)
		send(ui.logEdit, EM_SETSEL, 0, 0)
		// Full child erase prevents remnants of the previously displayed tab.
		call(pInvalidateRect, ui.logEdit, 0, 1)
		ui.lastLogText = text
	}
	// Owner-drawn tab buttons need an explicit repaint; avoid repainting the parent.
	for _, id := range []int{ctlTabRaw, ctlTabWake, ctlTabTimers, ctlTabReport} {
		invalidate(ui.buttons[id])
	}
}
func startScan() {
	if ui.scanning || ui.closing {
		return
	}
	ui.scanning = true
	ui.info = "正在检测 Windows 电源请求..."
	enabled(ui.buttons[ctlScan], false)
	enabled(ui.buttons[ctlClose], false)
	invalidate(ui.hwnd)
	go func() {
		raw, err := runPowerCFG("/requests")
		r := ParseRequests(raw)
		if err != nil {
			r.Err = err.Error()
		}
		ui.mu.Lock()
		ui.pendingScan = &scanMsg{Result: r}
		ui.mu.Unlock()
		post(ui.hwnd, WM_APP_SCAN, 0, 0)
	}()
}
func switchTab(n int) {
	if n < 0 || n > 3 {
		return
	}
	ui.tab = n
	updateLog()
	if (n == 1 && ui.lastwake == "") || (n == 2 && ui.waketimers == "") {
		arg := "/lastwake"
		if n == 2 {
			arg = "/waketimers"
		}
		go func(index int) {
			txt, err := runPowerCFG(arg)
			ui.mu.Lock()
			ui.pendingTab = &tabMsg{Index: index, Text: txt, Err: err}
			ui.mu.Unlock()
			post(ui.hwnd, WM_APP_TAB, 0, 0)
		}(n)
	}
}
func requestClose() {
	if ui.closing || ui.scanning {
		return
	}
	item, ok := selectedItem()
	if !ok {
		messagebox(ui.hwnd, "请先在检测结果中选中一条阻止项。", "请选择项目", MB_ICONINFORMATION)
		return
	}
	action := classifyAction(item)
	if action != ActionCloseProcess {
		inspectNonProcess(item, action)
		return
	}
	path, err := normalizeTarget(item.Name)
	if err != nil {
		messagebox(ui.hwnd, err.Error(), "无法确认进程", MB_ICONWARNING)
		return
	}
	candidates, err := findCandidates(path)
	if err != nil {
		messagebox(ui.hwnd, err.Error(), "无法安全关闭", MB_ICONWARNING)
		return
	}
	if len(candidates) == 0 {
		messagebox(ui.hwnd, "没有找到完整路径一致的进程。它可能已经退出，请重新检测。", "未找到目标", MB_ICONINFORMATION)
		return
	}
	candidate := candidates[0]
	if len(candidates) > 1 {
		idx := selectCandidate(candidates)
		if idx < 0 || idx >= len(candidates) {
			return
		}
		candidate = candidates[idx]
	}
	msg := "即将尝试正常关闭以下程序：\n\n" + processDescription(candidate) + "\n\n关闭前请保存程序中的工作。\n注意：同一路径的多个实例不能仅凭 powercfg 确认哪个真正发出了请求。\n\n是否继续？"
	if messagebox(ui.hwnd, msg, "确认关闭所选程序", MB_YESNO|MB_ICONWARNING|MB_DEFBUTTON2) != IDYES {
		return
	}
	ui.proc = candidate
	ui.requested = item
	ui.closing = true
	ui.info = fmt.Sprintf("正在确认 %s（PID %d）是否仍阻止休眠...", candidate.Name, candidate.Pid)
	enabled(ui.buttons[ctlClose], false)
	enabled(ui.buttons[ctlScan], false)
	invalidate(ui.hwnd)
	go func() {
		active := requestStillActive(item)
		ident := checkCandidate(candidate)
		ui.mu.Lock()
		ui.pendingVerify = &closeVerifyMsg{OK: active && ident, Reason: "进程身份或阻止项已经发生变化，操作已取消。请重新检测。"}
		ui.mu.Unlock()
		post(ui.hwnd, WM_APP_CLOSE_VERIFY, 0, 0)
	}()
}
func finishClose(message string) {
	ui.closing = false
	ui.info = message
	enabled(ui.buttons[ctlScan], true)
	updateDetails()
	invalidate(ui.hwnd)
}
func onCommand(id int, notify uint16) {
	switch id {
	case ctlScan:
		startScan()
	case ctlClose:
		requestClose()
	case ctlCopy:
		if err := copyClipboard(ui.hwnd, reportText(ui.scan, ui.lastwake, ui.waketimers, ui.operation)); err != nil {
			messagebox(ui.hwnd, err.Error(), "复制失败", MB_ICONERROR)
		} else {
			ui.info = "完整报告已复制到剪贴板"
			invalidate(ui.hwnd)
		}
	case ctlSave:
		if saved, err := saveReportDialog(ui.hwnd, reportText(ui.scan, ui.lastwake, ui.waketimers, ui.operation)); err != nil {
			messagebox(ui.hwnd, err.Error(), "保存失败", MB_ICONERROR)
		} else if saved {
			ui.info = "报告已保存为 TXT 文档"
			invalidate(ui.hwnd)
		}
	case ctlHelp:
		messagebox(ui.hwnd, "SleepInspector V1.8  /  使用说明\n\n1. 点击“立即检测”查看当前的电源请求。\n2. SYSTEM 会阻止自动休眠，DISPLAY 影响屏幕关闭。\n3. 选中普通程序，使用“关闭所选程序”，在确认与核验后尝试正常关闭。\n4. 选中音频驱动（例如 Realtek），使用“排查音频占用”，打开 Windows 音量合成器与任务管理器，查找可能使用音频的应用并自行关闭。\n5. 选中其他驱动或服务，可查看安全的管理入口。\n6. 下方保留原始输出、上次唤醒、唤醒定时器与完整报告。\n\n重要：DRIVER 不是 PROCESS，本工具不会强行结束驱动、停用音频设备或更改 powercfg 请求豁免。", "使用说明", MB_ICONINFORMATION)
	case ctlFilter:
		if notify == CBN_SELCHANGE {
			ui.filterChoice = int(send(ui.filter, CB_GETCURSEL, 0, 0))
			populateList()
		}
	case ctlTabRaw:
		switchTab(0)
	case ctlTabWake:
		switchTab(1)
	case ctlTabTimers:
		switchTab(2)
	case ctlTabReport:
		switchTab(3)
	}
}
func appWndProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case WM_CREATE:
		createUI(hwnd)
		return 0
	case WM_SIZE:
		refreshLayout()
		return 0
	case WM_GETMINMAXINFO:
		if lparam != 0 {
			p := (*minMaxInfo)(unsafe.Pointer(lparam))
			p.MinTrack = point{X: scale(1010), Y: scale(745)}
		}
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_PAINT:
		paintUI(hwnd)
		return 0
	case WM_DRAWITEM:
		if lparam != 0 {
			paintButton((*drawItem)(unsafe.Pointer(lparam)))
			return 1
		}
	case WM_CTLCOLORSTATIC, WM_CTLCOLOREDIT:
		if lparam == ui.details || lparam == ui.logEdit {
			// Read-only EDIT controls send WM_CTLCOLORSTATIC. Transparent
			// backgrounds caused ghosted/overprinted text when content changed.
			call(pSetTextColor, wparam, palette.Text)
			call(pSetBkColor, wparam, palette.White)
			call(pSetBkMode, wparam, OPAQUE)
			return ui.whiteBrush
		}
	case WM_COMMAND:
		onCommand(int(loword(wparam)), hiword(wparam))
		return 0
	case WM_NOTIFY:
		// Critical V1.6 flicker fix: V1.5 handled *every* listview notification.
		// NM_CUSTOMDRAW (during painting) then called InvalidateRect via
		// updateDetails, creating an endless repaint/notify/repaint feedback loop.
		if wparam == ctlList && lparam != 0 && !ui.populating {
			hdr := (*notifyHeader)(unsafe.Pointer(lparam))
			if hdr.HwndFrom == ui.lv && hdr.Code == listViewItemChanged {
				change := (*notifyListView)(unsafe.Pointer(lparam))
				if shouldUpdateSelectedRow(hdr.Code, change.Changed, change.OldState, change.NewState) && !ui.selectionPending {
					ui.selectionPending = true
					post(hwnd, WM_APP_SELECTION, 0, 0)
				}
			}
		}
		return 0
	case WM_APP_SELECTION:
		// Coalesce old-row deselect + new-row select into one edit update.
		ui.selectionPending = false
		updateDetails()
		return 0
	case WM_APP_INIT:
		startScan()
		return 0
	case WM_APP_SCAN:
		ui.mu.Lock()
		payload := ui.pendingScan
		ui.pendingScan = nil
		ui.mu.Unlock()
		if payload != nil {
			ui.scan = payload.Result
			ui.scanning = false
			enabled(ui.buttons[ctlScan], true)
			a, b, c := countRequests(ui.scan.Items)
			if ui.scan.Err != "" {
				ui.info = "检测失败：" + ui.scan.Err
			} else {
				ui.info = fmt.Sprintf("检测完成 · 休眠阻止 %d 项 / 屏幕阻止 %d 项 / 其他 %d 项", a, b, c)
			}
			populateList()
			updateLog()
			invalidate(ui.hwnd)
		}
		return 0
	case WM_APP_TAB:
		ui.mu.Lock()
		payload := ui.pendingTab
		ui.pendingTab = nil
		ui.mu.Unlock()
		if payload != nil {
			t := payload.Text
			if payload.Err != nil {
				t = "读取失败：" + payload.Err.Error() + "\n" + payload.Text
			}
			if payload.Index == 1 {
				ui.lastwake = t
			} else {
				ui.waketimers = t
			}
			if ui.tab == payload.Index {
				updateLog()
			}
		}
		return 0
	case WM_APP_CLOSE_VERIFY:
		ui.mu.Lock()
		payload := ui.pendingVerify
		ui.pendingVerify = nil
		ui.mu.Unlock()
		if payload != nil {
			if !payload.OK {
				finishClose(payload.Reason)
				messagebox(hwnd, payload.Reason, "安全校验未通过", MB_ICONWARNING)
				return 0
			}
			ui.info = fmt.Sprintf("正在等待 %s 正常退出...", ui.proc.Name)
			invalidate(ui.hwnd)
			c := ui.proc
			go func() {
				n := attemptGraceful(c)
				for i := 0; i < 13; i++ {
					if !isRunning(c) {
						break
					}
					time.Sleep(200 * time.Millisecond)
				}
				exited := !isRunning(c)
				ui.mu.Lock()
				ui.pendingGrace = &closeGraceMsg{Exited: exited, Windows: n}
				ui.mu.Unlock()
				post(ui.hwnd, WM_APP_CLOSE_GRACE, 0, 0)
			}()
		}
		return 0
	case WM_APP_CLOSE_GRACE:
		ui.mu.Lock()
		payload := ui.pendingGrace
		ui.pendingGrace = nil
		ui.mu.Unlock()
		if payload != nil {
			c := ui.proc
			if payload.Exited {
				ui.operation = append(ui.operation, fmt.Sprintf("%s 已正常退出（PID %d）", time.Now().Format("15:04:05"), c.Pid))
				finishClose("程序已正常退出，正在重新检测...")
				startScan()
				return 0
			}
			why := "程序尚未退出。"
			if payload.Windows == 0 {
				why = "未找到可正常关闭的主窗口。"
			}
			prompt := why + "\n\n" + processDescription(c) + "\n\n是否强制结束此进程？这可能丢失尚未保存的数据。"
			if messagebox(hwnd, prompt, "是否强制结束？", MB_YESNO|MB_ICONWARNING|MB_DEFBUTTON2) != IDYES {
				ui.operation = append(ui.operation, fmt.Sprintf("%s 用户取消强制结束 PID %d", time.Now().Format("15:04:05"), c.Pid))
				finishClose("已取消强制结束，程序继续运行")
				return 0
			}
			go func() {
				var err error
				if !requestStillActive(ui.requested) {
					err = fmt.Errorf("原阻止项已经消失或无法重新确认，已取消强制结束")
				} else {
					err = forceTerminate(c)
				}
				ui.mu.Lock()
				ui.pendingForced = &closeForcedMsg{Err: err}
				ui.mu.Unlock()
				post(ui.hwnd, WM_APP_CLOSE_FORCED, 0, 0)
			}()
		}
		return 0
	case WM_APP_CLOSE_FORCED:
		ui.mu.Lock()
		payload := ui.pendingForced
		ui.pendingForced = nil
		ui.mu.Unlock()
		if payload != nil {
			if payload.Err != nil {
				finishClose("强制结束失败")
				messagebox(hwnd, payload.Err.Error(), "操作未完成", MB_ICONERROR)
			} else {
				ui.operation = append(ui.operation, fmt.Sprintf("%s 用户确认强制结束 PID %d", time.Now().Format("15:04:05"), ui.proc.Pid))
				finishClose("程序已结束，正在重新检测...")
				startScan()
			}
		}
		return 0
	case WM_CLOSE:
		call(pDestroyWindow, hwnd)
		return 0
	case WM_DESTROY:
		for _, v := range []uintptr{ui.fontNormal, ui.fontBtn, ui.fontHead, ui.fontLarge, ui.fontMetric, ui.fontSmall, ui.fontCard, ui.whiteBrush} {
			if v != 0 {
				call(pDeleteObject, v)
			}
		}
		call(pPostQuitMessage, 0)
		return 0
	}
	return call(pDefWindowProc, hwnd, uintptr(message), wparam, lparam)
}

// Single-instance process picker. No broad "kill all with this exe name" behavior.
type choiceState struct {
	Hwnd, List, BtnYes, BtnNo uintptr
	Items                     []candidate
	Selected                  int
	Done                      bool
}

var chooser *choiceState

func chooseProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case WM_CREATE:
		if chooser == nil {
			return 0
		}
		chooser.Hwnd = hwnd
		chooser.List = createControl("LISTBOX", "", WS_BORDER|WS_VSCROLL|WS_TABSTOP|LBS_NOTIFY, 0, chooserList, hwnd)
		uiFont(chooser.List, ui.fontNormal)
		move(chooser.List, scale(16), scale(14), scale(540), scale(210))
		for _, c := range chooser.Items {
			send(chooser.List, LB_ADDSTRING, 0, sPtr(fmt.Sprintf("PID %-8d  |  %s", c.Pid, c.Path)))
		}
		send(chooser.List, LB_SETCURSEL, 0, 0)
		chooser.BtnYes = createControl("BUTTON", "确认选择", WS_TABSTOP, 0, chooserOK, hwnd)
		chooser.BtnNo = createControl("BUTTON", "取消", WS_TABSTOP, 0, chooserCancel, hwnd)
		uiFont(chooser.BtnYes, ui.fontNormal)
		uiFont(chooser.BtnNo, ui.fontNormal)
		move(chooser.BtnYes, scale(340), scale(237), scale(105), scale(35))
		move(chooser.BtnNo, scale(453), scale(237), scale(103), scale(35))
		return 0
	case WM_COMMAND:
		id := int(loword(wparam))
		if id == chooserOK || (id == chooserList && hiword(wparam) == LBN_DBLCLK) {
			chooser.Selected = int(send(chooser.List, LB_GETCURSEL, 0, 0))
			call(pDestroyWindow, hwnd)
			return 0
		}
		if id == chooserCancel {
			chooser.Selected = -1
			call(pDestroyWindow, hwnd)
			return 0
		}
	case WM_CLOSE:
		chooser.Selected = -1
		call(pDestroyWindow, hwnd)
		return 0
	case WM_DESTROY:
		if chooser != nil {
			chooser.Done = true
		}
		return 0
	}
	return call(pDefWindowProc, hwnd, uintptr(message), wparam, lparam)
}
func selectCandidate(candidates []candidate) int {
	chooser = &choiceState{Items: candidates, Selected: -1}
	hwnd := call(pCreateWindow, WS_EX_APPWINDOW, sPtr("SleepInspectorChoice"), sPtr("请选择要关闭的进程实例（同路径多 PID）"), WS_CAPTION|WS_SYSMENU|WS_VISIBLE, uintptr(220), uintptr(180), uintptr(scale(580)), uintptr(scale(326)), ui.hwnd, 0, call(pGetModuleHandle, 0), 0)
	if hwnd == 0 {
		chooser = nil
		return -1
	}
	enabled(ui.hwnd, false)
	call(pShowWindow, hwnd, SW_SHOW)
	call(pUpdateWindow, hwnd)
	var m msg
	for !chooser.Done {
		v := int32(call(pGetMessage, uintptr(unsafe.Pointer(&m)), 0, 0, 0))
		if v <= 0 {
			break
		}
		call(pTranslateMessage, uintptr(unsafe.Pointer(&m)))
		call(pDispatchMessage, uintptr(unsafe.Pointer(&m)))
	}
	idx := chooser.Selected
	chooser = nil
	enabled(ui.hwnd, true)
	call(pSetFocus, ui.hwnd)
	return idx
}
func main() {
	runtime.LockOSThread()
	// Preserve the user's original one-click elevated diagnostics flow.
	if !isAdmin() {
		if err := elevate(); err != nil {
			messagebox(0, "程序需要管理员权限才能准确读取电源请求。\n\n"+err.Error(), "需要管理员权限", MB_ICONWARNING)
		}
		return
	}
	call(pSetProcessDPIAware)
	inst := call(pGetModuleHandle, 0)
	className := utfPtr("SleepInspectorMainV18")
	wc := windowClass{Style: 3, WndProc: syscall.NewCallback(appWndProc), Instance: inst, Cursor: call(pLoadCursor, 0, IDC_ARROW), ClassName: className}
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	if call(pRegisterClass, uintptr(unsafe.Pointer(&wc))) == 0 {
		messagebox(0, "无法注册窗口类型。", "程序启动失败", MB_ICONERROR)
		return
	}
	ccName := utfPtr("SleepInspectorChoice")
	cc := windowClass{WndProc: syscall.NewCallback(chooseProc), Instance: inst, Cursor: call(pLoadCursor, 0, IDC_ARROW), ClassName: ccName}
	cc.CbSize = uint32(unsafe.Sizeof(cc))
	call(pRegisterClass, uintptr(unsafe.Pointer(&cc)))
	hwnd := call(pCreateWindow, WS_EX_APPWINDOW, uintptr(unsafe.Pointer(className)), sPtr("Windows 休眠阻止项检测器 V1.8  |  SleepInspector"), WS_OVERLAPPED|WS_CAPTION|WS_SYSMENU|WS_THICKFRAME|WS_MINIMIZEBOX|WS_MAXIMIZEBOX|WS_CLIPCHILDREN|WS_CLIPSIBLINGS|WS_VISIBLE, uintptr(110), uintptr(60), uintptr(1360), uintptr(920), 0, 0, inst, 0)
	if hwnd == 0 {
		messagebox(0, "Windows 无法创建主窗口。", "程序启动失败", MB_ICONERROR)
		return
	}
	icon32 := appIcon(32)
	icon64 := appIcon(64)
	if icon32 != 0 {
		send(hwnd, WM_SETICON, 0, icon32)
	}
	if icon64 != 0 {
		send(hwnd, WM_SETICON, 1, icon64)
	}
	call(pShowWindow, hwnd, SW_SHOW)
	call(pUpdateWindow, hwnd)
	post(hwnd, WM_APP_INIT, 0, 0)
	var m msg
	for {
		code := int32(call(pGetMessage, uintptr(unsafe.Pointer(&m)), 0, 0, 0))
		if code <= 0 {
			break
		}
		if m.Message == WM_KEYDOWN && m.WParam == VK_F5 {
			startScan()
			continue
		}
		call(pTranslateMessage, uintptr(unsafe.Pointer(&m)))
		call(pDispatchMessage, uintptr(unsafe.Pointer(&m)))
	}
	os.Exit(0)
}
