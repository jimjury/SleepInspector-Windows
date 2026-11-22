//go:build windows

package main

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"
)

const (
	WS_OVERLAPPED                     = 0x00000000
	WS_CAPTION                        = 0x00C00000
	WS_SYSMENU                        = 0x00080000
	WS_THICKFRAME                     = 0x00040000
	WS_MINIMIZEBOX                    = 0x00020000
	WS_MAXIMIZEBOX                    = 0x00010000
	WS_VISIBLE                        = 0x10000000
	WS_CLIPCHILDREN                   = 0x02000000
	WS_CLIPSIBLINGS                   = 0x04000000
	WS_CHILD                          = 0x40000000
	WS_VSCROLL                        = 0x00200000
	WS_HSCROLL                        = 0x00100000
	WS_BORDER                         = 0x00800000
	WS_TABSTOP                        = 0x00010000
	WS_DISABLED                       = 0x08000000
	WS_EX_CLIENTEDGE                  = 0x00000200
	WS_EX_APPWINDOW                   = 0x00040000
	WM_CREATE                         = 0x0001
	WM_DESTROY                        = 0x0002
	WM_MOVE                           = 0x0003
	WM_SIZE                           = 0x0005
	WM_PAINT                          = 0x000F
	WM_CLOSE                          = 0x0010
	WM_ERASEBKGND                     = 0x0014
	WM_SETFONT                        = 0x0030
	WM_GETFONT                        = 0x0031
	WM_SETICON                        = 0x0080
	WM_NOTIFY                         = 0x004E
	WM_COMMAND                        = 0x0111
	WM_DRAWITEM                       = 0x002B
	WM_CTLCOLORSTATIC                 = 0x0138
	WM_CTLCOLOREDIT                   = 0x0133
	WM_GETMINMAXINFO                  = 0x0024
	WM_APP                            = 0x8000
	WM_APP_INIT                       = WM_APP + 1
	WM_APP_SCAN                       = WM_APP + 2
	WM_APP_TAB                        = WM_APP + 3
	WM_APP_CLOSE_VERIFY               = WM_APP + 4
	WM_APP_CLOSE_GRACE                = WM_APP + 5
	WM_APP_CLOSE_FORCED               = WM_APP + 6
	SW_SHOW                           = 5
	SW_SHOWNORMAL                     = 1
	MB_OK                             = 0
	MB_ICONERROR                      = 0x10
	MB_ICONQUESTION                   = 0x20
	MB_ICONWARNING                    = 0x30
	MB_ICONINFORMATION                = 0x40
	MB_YESNO                          = 4
	MB_DEFBUTTON2                     = 0x100
	IDYES                             = 6
	COLOR_WINDOW                      = 5
	BS_OWNERDRAW                      = 0x0000000B
	CBS_DROPDOWNLIST                  = 3
	CBS_HASSTRINGS                    = 0x200
	CBN_SELCHANGE                     = 1
	CB_ADDSTRING                      = 0x0143
	CB_GETCURSEL                      = 0x0147
	CB_SETCURSEL                      = 0x014E
	ES_MULTILINE                      = 4
	ES_AUTOVSCROLL                    = 0x40
	ES_READONLY                       = 0x800
	ES_WANTRETURN                     = 0x1000
	EM_SETSEL                         = 0x00B1
	EM_SETMARGINS                     = 0x00D3
	EC_LEFTMARGIN                     = 0x0001
	EC_RIGHTMARGIN                    = 0x0002
	LVS_REPORT                        = 1
	LVS_SINGLESEL                     = 4
	LVS_SHOWSELALWAYS                 = 8
	LVS_NOSORTHEADER                  = 0x8000
	LVM_FIRST                         = 0x1000
	LVM_INSERTCOLUMNW                 = LVM_FIRST + 97
	LVM_INSERTITEMW                   = LVM_FIRST + 77
	LVM_SETITEMTEXTW                  = LVM_FIRST + 116
	LVM_DELETEALLITEMS                = LVM_FIRST + 9
	LVM_GETNEXTITEM                   = LVM_FIRST + 12
	LVM_SETEXTENDEDLISTVIEWSTYLE      = LVM_FIRST + 54
	LVM_SETBKCOLOR                    = LVM_FIRST + 1
	LVM_SETTEXTCOLOR                  = LVM_FIRST + 36
	LVM_SETTEXTBKCOLOR                = LVM_FIRST + 38
	LVM_SETCOLUMNWIDTH                = LVM_FIRST + 30
	LVS_EX_FULLROWSELECT              = 0x20
	LVS_EX_DOUBLEBUFFER               = 0x10000
	LVS_EX_LABELTIP                   = 0x4000
	LVNI_SELECTED                     = 2
	LVCF_TEXT                         = 4
	LVCF_WIDTH                        = 2
	LVCF_SUBITEM                      = 8
	LVIF_TEXT                         = 1
	LBS_NOTIFY                        = 1
	LB_ADDSTRING                      = 0x0180
	LB_GETCURSEL                      = 0x0188
	LB_SETCURSEL                      = 0x0186
	LBN_DBLCLK                        = 2
	ODS_SELECTED                      = 1
	ODS_DISABLED                      = 4
	ODT_BUTTON                        = 4
	DT_LEFT                           = 0
	DT_CENTER                         = 1
	DT_RIGHT                          = 2
	DT_VCENTER                        = 4
	DT_SINGLELINE                     = 0x20
	DT_END_ELLIPSIS                   = 0x8000
	DT_WORDBREAK                      = 0x10
	DT_NOPREFIX                       = 0x800
	TRANSPARENT                       = 1
	OPAQUE                            = 2
	PS_SOLID                          = 0
	GWLP_WNDPROC                      = -4
	GWL_STYLE                         = -16
	IDC_ARROW                         = 32512
	ICC_LISTVIEW_CLASSES              = 1
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	PROCESS_TERMINATE                 = 1
	SYNCHRONIZE                       = 0x100000
	TH32CS_SNAPPROCESS                = 2
	INVALID_HANDLE_VALUE              = ^uintptr(0)
	STILL_ACTIVE                      = 259
	WAIT_TIMEOUT                      = 258
	CF_UNICODETEXT                    = 13
	GMEM_MOVEABLE                     = 2
	GHND                              = 0x42
	OFN_OVERWRITEPROMPT               = 2
	OFN_NOCHANGEDIR                   = 8
	OFN_PATHMUSTEXIST                 = 0x800
	OFN_EXPLORER                      = 0x80000
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

func (r rect) w() int32 { return r.Right - r.Left }
func (r rect) h() int32 { return r.Bottom - r.Top }

type msg struct {
	Hwnd           uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             point
	LPrivate       uint32
}
type windowClass struct {
	CbSize                             uint32
	Style                              uint32
	WndProc                            uintptr
	ClsExtra                           int32
	WndExtra                           int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSm                             uintptr
}
type paintStruct struct {
	Hdc                uintptr
	Erase              int32
	Paint              rect
	Restore, IncUpdate int32
	Reserved           [32]byte
}
type lvColumn struct {
	Mask         uint32
	Fmt          int32
	Width        int32
	Text         *uint16
	TextMax      int32
	Subitem      int32
	Image        int32
	Order        int32
	WidthMin     int32
	WidthDefault int32
	WidthIdeal   int32
}
type lvItem struct {
	Mask             uint32
	Item, Subitem    int32
	State, StateMask uint32
	Text             *uint16
	TextMax          int32
	Image            int32
	Param            uintptr
	Indent           int32
	GroupId          int32
	Columns          uint32
	ColumnPtr        uintptr
	ColumnFmt        uintptr
	Group            int32
}
type drawItem struct {
	CtlType, CtlID, ItemID, Action, State uint32
	HwndItem, Hdc                         uintptr
	Rc                                    rect
	Data                                  uintptr
}
type initCommon struct {
	Size  uint32
	Flags uint32
}
type logFont struct {
	Height, Width, Escapement, Orientation, Weight                                              int32
	Italic, Underline, Strikeout, Charset, OutPrecision, ClipPrecision, Quality, PitchAndFamily byte
	Face                                                                                        [32]uint16
}
type processEntry struct {
	Size, Usage, Pid        uint32
	Heap                    uintptr
	Module, Threads, Parent uint32
	Base                    int32
	Flags                   uint32
	Exe                     [260]uint16
}
type openFilename struct {
	Size                      uint32
	Owner, Instance           uintptr
	Filter, CustomFilter      *uint16
	MaxCustom, FilterIndex    uint32
	File                      *uint16
	MaxFile                   uint32
	FileTitle                 *uint16
	MaxFileTitle              uint32
	InitialDir, Title         *uint16
	Flags                     uint32
	FileOffset, FileExtension uint16
	DefExt                    *uint16
	CustData, Hook            uintptr
	TemplateName              *uint16
	Reserved                  uintptr
	ReservedNumber, FlagsEx   uint32
}
type minMaxInfo struct {
	Reserved                                 point
	MaxSize, MaxPosition, MinTrack, MaxTrack point
}

var (
	user32                    = syscall.NewLazyDLL("user32.dll")
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	gdi32                     = syscall.NewLazyDLL("gdi32.dll")
	comctl32                  = syscall.NewLazyDLL("comctl32.dll")
	comdlg32                  = syscall.NewLazyDLL("comdlg32.dll")
	shell32                   = syscall.NewLazyDLL("shell32.dll")
	pRegisterClass            = user32.NewProc("RegisterClassExW")
	pCreateWindow             = user32.NewProc("CreateWindowExW")
	pDefWindowProc            = user32.NewProc("DefWindowProcW")
	pShowWindow               = user32.NewProc("ShowWindow")
	pUpdateWindow             = user32.NewProc("UpdateWindow")
	pMoveWindow               = user32.NewProc("MoveWindow")
	pGetClientRect            = user32.NewProc("GetClientRect")
	pInvalidateRect           = user32.NewProc("InvalidateRect")
	pBeginPaint               = user32.NewProc("BeginPaint")
	pEndPaint                 = user32.NewProc("EndPaint")
	pGetMessage               = user32.NewProc("GetMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessage          = user32.NewProc("DispatchMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pPostMessage              = user32.NewProc("PostMessageW")
	pSendMessage              = user32.NewProc("SendMessageW")
	pEnableWindow             = user32.NewProc("EnableWindow")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pSetWindowText            = user32.NewProc("SetWindowTextW")
	pGetWindowText            = user32.NewProc("GetWindowTextW")
	pSetFocus                 = user32.NewProc("SetFocus")
	pMessageBox               = user32.NewProc("MessageBoxW")
	pLoadCursor               = user32.NewProc("LoadCursorW")
	pGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	pSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	pFillRect                 = user32.NewProc("FillRect")
	pDrawText                 = user32.NewProc("DrawTextW")
	pDrawEdge                 = user32.NewProc("DrawEdge")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	pEnumWindows              = user32.NewProc("EnumWindows")
	pIsWindowVisible          = user32.NewProc("IsWindowVisible")
	pIsWindowEnabled          = user32.NewProc("IsWindowEnabled")
	pSetWindowLongPtr         = user32.NewProc("SetWindowLongPtrW")
	pInitCommon               = comctl32.NewProc("InitCommonControlsEx")
	pCreateFontIndirect       = gdi32.NewProc("CreateFontIndirectW")
	pCreateSolidBrush         = gdi32.NewProc("CreateSolidBrush")
	pCreatePen                = gdi32.NewProc("CreatePen")
	pSelectObject             = gdi32.NewProc("SelectObject")
	pDeleteObject             = gdi32.NewProc("DeleteObject")
	pSetBkMode                = gdi32.NewProc("SetBkMode")
	pSetBkColor               = gdi32.NewProc("SetBkColor")
	pSetTextColor             = gdi32.NewProc("SetTextColor")
	pRoundRect                = gdi32.NewProc("RoundRect")
	pRectangle                = gdi32.NewProc("Rectangle")
	pSetDCBrushColor          = gdi32.NewProc("SetDCBrushColor")
	pCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	pDeleteDC                 = gdi32.NewProc("DeleteDC")
	pBitBlt                   = gdi32.NewProc("BitBlt")
	pGetACP                   = kernel32.NewProc("GetACP")
	pGetOEMCP                 = kernel32.NewProc("GetOEMCP")
	pMultiByteToWideChar      = kernel32.NewProc("MultiByteToWideChar")
	pGetModuleHandle          = kernel32.NewProc("GetModuleHandleW")
	pOpenProcess              = kernel32.NewProc("OpenProcess")
	pCloseHandle              = kernel32.NewProc("CloseHandle")
	pGetProcessTimes          = kernel32.NewProc("GetProcessTimes")
	pGetProcessImage          = kernel32.NewProc("QueryFullProcessImageNameW")
	pProcessSnapshot          = kernel32.NewProc("CreateToolhelp32Snapshot")
	pProcessFirst             = kernel32.NewProc("Process32FirstW")
	pProcessNext              = kernel32.NewProc("Process32NextW")
	pQueryDosDevice           = kernel32.NewProc("QueryDosDeviceW")
	pWaitForSingleObject      = kernel32.NewProc("WaitForSingleObject")
	pTerminateProcess         = kernel32.NewProc("TerminateProcess")
	pGetWindowsDirectory      = kernel32.NewProc("GetWindowsDirectoryW")
	pOpenClipboard            = user32.NewProc("OpenClipboard")
	pEmptyClipboard           = user32.NewProc("EmptyClipboard")
	pSetClipboardData         = user32.NewProc("SetClipboardData")
	pCloseClipboard           = user32.NewProc("CloseClipboard")
	pGlobalAlloc              = kernel32.NewProc("GlobalAlloc")
	pGlobalLock               = kernel32.NewProc("GlobalLock")
	pGlobalUnlock             = kernel32.NewProc("GlobalUnlock")
	pGlobalFree               = kernel32.NewProc("GlobalFree")
	pGetSaveFileName          = comdlg32.NewProc("GetSaveFileNameW")
	pIsUserAdmin              = shell32.NewProc("IsUserAnAdmin")
	pShellExecute             = shell32.NewProc("ShellExecuteW")
	pCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
)

func utfPtr(s string) *uint16                              { return syscall.StringToUTF16Ptr(s) }
func sPtr(s string) uintptr                                { return uintptr(unsafe.Pointer(utfPtr(s))) }
func loword(v uintptr) uint16                              { return uint16(v & 0xFFFF) }
func hiword(v uintptr) uint16                              { return uint16((v >> 16) & 0xFFFF) }
func rgb(r, g, b byte) uintptr                             { return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16) }
func call(proc *syscall.LazyProc, args ...uintptr) uintptr { r, _, _ := proc.Call(args...); return r }
func send(hwnd uintptr, id uint32, wparam, lparam uintptr) uintptr {
	return call(pSendMessage, hwnd, uintptr(id), wparam, lparam)
}
func post(hwnd uintptr, id uint32, wparam, lparam uintptr) {
	call(pPostMessage, hwnd, uintptr(id), wparam, lparam)
}
func messagebox(parent uintptr, message, title string, flags uintptr) int {
	return int(call(pMessageBox, parent, sPtr(message), sPtr(title), flags))
}
func makeFont(size, weight int, face string) uintptr {
	lf := logFont{Height: int32(-size), Weight: int32(weight), Quality: 5}
	copy(lf.Face[:], syscall.StringToUTF16(face))
	return call(pCreateFontIndirect, uintptr(unsafe.Pointer(&lf)))
}
func uiFont(hwnd, font uintptr) { send(hwnd, WM_SETFONT, font, 1) }
func createControl(class, title string, style uint32, exStyle uint32, id int32, parent uintptr) uintptr {
	return call(pCreateWindow, uintptr(exStyle), sPtr(class), sPtr(title), uintptr(style|WS_CHILD|WS_VISIBLE), 0, 0, 0, 0, parent, uintptr(id), 0, 0)
}
func move(hwnd uintptr, x, y, w, h int32) {
	if hwnd != 0 {
		call(pMoveWindow, hwnd, uintptr(x), uintptr(y), uintptr(w), uintptr(h), 1)
	}
}
func enabled(hwnd uintptr, b bool) {
	v := uintptr(0)
	if b {
		v = 1
	}
	call(pEnableWindow, hwnd, v)
}
func invalidate(hwnd uintptr) { call(pInvalidateRect, hwnd, 0, 0) }
func windowRect(hwnd uintptr) rect {
	var r rect
	call(pGetClientRect, hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}
func utf16String(buf []uint16) string {
	e := 0
	for e < len(buf) && buf[e] != 0 {
		e++
	}
	return string(utf16.Decode(buf[:e]))
}
func setControlText(hwnd uintptr, text string) { call(pSetWindowText, hwnd, sPtr(text)) }

func decodeConsole(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if utf8.Valid(raw) {
		return string(raw)
	}
	cp := call(pGetOEMCP)
	if cp == 0 {
		cp = call(pGetACP)
	}
	try := func(codepage uintptr) string {
		n := call(pMultiByteToWideChar, codepage, 0, uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)), 0, 0)
		if n == 0 {
			return ""
		}
		wide := make([]uint16, n)
		m := call(pMultiByteToWideChar, codepage, 0, uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)), uintptr(unsafe.Pointer(&wide[0])), n)
		if m == 0 {
			return ""
		}
		return string(utf16.Decode(wide))
	}
	s := try(cp)
	if s == "" {
		s = try(call(pGetACP))
	}
	if s == "" {
		return strings.ToValidUTF8(string(raw), "�")
	}
	return s
}
func runPowerCFG(arg string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powercfg.exe", arg)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	raw, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", fmt.Errorf("命令 %s 执行超时", arg)
	}
	if err != nil {
		return decodeConsole(raw), fmt.Errorf("powercfg %s: %w", arg, err)
	}
	return strings.TrimSpace(decodeConsole(raw)), nil
}
func isAdmin() bool { return call(pIsUserAdmin) != 0 }
func elevate() error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	r := call(pShellExecute, 0, sPtr("runas"), sPtr(path), 0, 0, SW_SHOWNORMAL)
	if r <= 32 {
		return fmt.Errorf("管理员授权未完成（错误码 %d）", r)
	}
	return nil
}

func copyClipboard(hwnd uintptr, text string) error {
	buf := utf16.Encode([]rune(text))
	buf = append(buf, 0)
	if call(pOpenClipboard, hwnd) == 0 {
		return errors.New("无法打开剪贴板，请重试")
	}
	defer call(pCloseClipboard)
	if call(pEmptyClipboard) == 0 {
		return errors.New("清空剪贴板失败")
	}
	size := uintptr(len(buf) * 2)
	memory := call(pGlobalAlloc, GHND, size)
	if memory == 0 {
		return errors.New("剪贴板内存分配失败")
	}
	ptr := call(pGlobalLock, memory)
	if ptr == 0 {
		call(pGlobalFree, memory)
		return errors.New("剪贴板内存锁定失败")
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(size))
	copy(dst, unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), int(size)))
	call(pGlobalUnlock, memory)
	if call(pSetClipboardData, CF_UNICODETEXT, memory) == 0 {
		call(pGlobalFree, memory)
		return errors.New("写入剪贴板失败")
	}
	return nil
}
func saveReportDialog(hwnd uintptr, text string) (bool, error) {
	pathbuf := make([]uint16, 2048)
	filename := "SleepInspector_" + time.Now().Format("20060102_150405") + ".txt"
	copy(pathbuf, syscall.StringToUTF16(filename))
	filter := syscall.StringToUTF16("文本文件 (*.txt)\x00*.txt\x00所有文件 (*.*)\x00*.*\x00\x00")
	title := utfPtr("保存休眠诊断报告")
	defext := utfPtr("txt")
	ofn := openFilename{Owner: hwnd, Filter: &filter[0], File: &pathbuf[0], MaxFile: uint32(len(pathbuf)), Title: title, DefExt: defext, Flags: OFN_EXPLORER | OFN_PATHMUSTEXIST | OFN_NOCHANGEDIR | OFN_OVERWRITEPROMPT}
	ofn.Size = uint32(unsafe.Sizeof(ofn))
	if call(pGetSaveFileName, uintptr(unsafe.Pointer(&ofn))) == 0 {
		return false, nil
	}
	p := utf16String(pathbuf)
	if p == "" {
		return false, nil
	}
	raw := append([]byte{0xef, 0xbb, 0xbf}, []byte(strings.ReplaceAll(text, "\r\n", "\n"))...)
	if err := os.WriteFile(p, raw, 0644); err != nil {
		return false, err
	}
	return true, nil
}

// Process identification: strict equality of executable paths, never by base name.
type candidate struct {
	Pid        uint32
	Path, Name string
	Created    uint64
}

func normalizeTarget(requestPath string) (string, error) {
	p := strings.TrimSpace(strings.Trim(requestPath, "\""))
	if strings.HasPrefix(p, "\\\\?\\") {
		p = p[4:]
	}
	if len(p) >= 4 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' && p[2] == '\\' {
		if !strings.HasSuffix(strings.ToLower(p), ".exe") {
			return "", errors.New("目标不是 EXE 可执行文件")
		}
		return filepath.Clean(p), nil
	}
	if !strings.HasPrefix(strings.ToLower(p), strings.ToLower(`\Device\HarddiskVolume`)) {
		return "", errors.New("无法安全识别完整的磁盘程序路径")
	}
	for letter := 'A'; letter <= 'Z'; letter++ {
		drive := string(letter) + ":"
		raw := make([]uint16, 1024)
		n := call(pQueryDosDevice, sPtr(drive), uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)))
		if n == 0 {
			continue
		}
		mapped := utf16String(raw)
		if len(p) > len(mapped) && strings.EqualFold(p[:len(mapped)], mapped) && p[len(mapped)] == '\\' {
			dos := drive + p[len(mapped):]
			if !strings.HasSuffix(strings.ToLower(dos), ".exe") {
				return "", errors.New("目标不是 EXE 可执行文件")
			}
			return filepath.Clean(dos), nil
		}
	}
	return "", errors.New("设备路径无法映射到磁盘盘符，已拒绝自动关闭")
}

var protectedNames = map[string]bool{
	"system": true, "system idle process": true, "registry": true, "csrss.exe": true, "smss.exe": true, "wininit.exe": true, "winlogon.exe": true, "lsass.exe": true, "services.exe": true, "svchost.exe": true, "fontdrvhost.exe": true, "dwm.exe": true, "taskhostw.exe": true, "sihost.exe": true, "explorer.exe": true, "conhost.exe": true, "powershell.exe": true, "pwsh.exe": true, "cmd.exe": true, "taskmgr.exe": true, "audiodg.exe": true, "spoolsv.exe": true, "searchindexer.exe": true, "wmiprvse.exe": true, "runtimebroker.exe": true, "shellexperiencehost.exe": true, "startmenuexperiencehost.exe": true, "sleepinspector_v1.5_windows_x64.exe": true, "sleepinspector_v1.6_windows_x64.exe": true, "sleepinspector_v1.7_windows_x64.exe": true,
}

func protectReason(path string) string {
	p := filepath.Clean(strings.TrimSpace(path))
	base := strings.ToLower(filepath.Base(p))
	if protectedNames[base] {
		return "该程序属于受保护的 Windows 组件或检测器自身"
	}
	if !strings.HasSuffix(base, ".exe") {
		return "目标不是标准 EXE 程序"
	}
	wdir := make([]uint16, 520)
	n := call(pGetWindowsDirectory, uintptr(unsafe.Pointer(&wdir[0])), uintptr(len(wdir)))
	if n > 0 && int(n) < len(wdir) {
		dir := strings.TrimRight(utf16String(wdir[:n]), "\\") + "\\"
		if strings.HasPrefix(strings.ToLower(p), strings.ToLower(dir)) {
			return "位于 Windows 系统目录，禁止结束"
		}
	}
	return ""
}
func processHandle(pid uint32, rights uintptr) uintptr {
	return call(pOpenProcess, rights, 0, uintptr(pid))
}
func getProcessStart(h uintptr) (uint64, bool) {
	var a, b, c, d uint64
	if call(pGetProcessTimes, h, uintptr(unsafe.Pointer(&a)), uintptr(unsafe.Pointer(&b)), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&d))) == 0 {
		return 0, false
	}
	return a, true
}
func getProcessPath(h uintptr) string {
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if call(pGetProcessImage, h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))) == 0 {
		return ""
	}
	return string(utf16.Decode(buf[:n]))
}
func findCandidates(target string) ([]candidate, error) {
	if r := protectReason(target); r != "" {
		return nil, errors.New(r)
	}
	shot := call(pProcessSnapshot, TH32CS_SNAPPROCESS, 0)
	if shot == INVALID_HANDLE_VALUE || shot == 0 {
		return nil, errors.New("无法枚举当前进程")
	}
	defer call(pCloseHandle, shot)
	pe := processEntry{}
	pe.Size = uint32(unsafe.Sizeof(pe))
	var result []candidate
	if call(pProcessFirst, shot, uintptr(unsafe.Pointer(&pe))) == 0 {
		return nil, errors.New("无法读取进程列表")
	}
	for {
		pid := pe.Pid
		if pid > 4 && pid != uint32(os.Getpid()) {
			h := processHandle(pid, PROCESS_QUERY_LIMITED_INFORMATION)
			if h != 0 {
				path := getProcessPath(h)
				if path != "" && strings.EqualFold(filepath.Clean(path), filepath.Clean(target)) {
					started, ok := getProcessStart(h)
					if ok {
						result = append(result, candidate{Pid: pid, Path: path, Name: filepath.Base(path), Created: started})
					}
				}
				call(pCloseHandle, h)
			}
		}
		if call(pProcessNext, shot, uintptr(unsafe.Pointer(&pe))) == 0 {
			break
		}
	}
	return result, nil
}
func checkCandidate(c candidate) bool {
	if c.Pid <= 4 || c.Pid == uint32(os.Getpid()) || protectReason(c.Path) != "" {
		return false
	}
	h := processHandle(c.Pid, PROCESS_QUERY_LIMITED_INFORMATION|SYNCHRONIZE)
	if h == 0 {
		return false
	}
	defer call(pCloseHandle, h)
	if call(pWaitForSingleObject, h, 0) != WAIT_TIMEOUT {
		return false
	}
	p := getProcessPath(h)
	t, ok := getProcessStart(h)
	return ok && t == c.Created && strings.EqualFold(filepath.Clean(p), filepath.Clean(c.Path))
}
func isRunning(c candidate) bool { return checkCandidate(c) }
func attemptGraceful(c candidate) int {
	if !checkCandidate(c) {
		return 0
	}
	n := 0
	cb := syscall.NewCallback(func(hwnd, lp uintptr) uintptr {
		var pid uint32
		call(pGetWindowThreadProcessId, hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid == c.Pid && call(pIsWindowVisible, hwnd) != 0 {
			// A WM_CLOSE notification leaves the application free to prompt users to save.
			post(hwnd, WM_CLOSE, 0, 0)
			n++
		}
		return 1
	})
	call(pEnumWindows, cb, 0)
	return n
}
func forceTerminate(c candidate) error {
	if !checkCandidate(c) {
		return errors.New("目标已经退出或身份发生改变，已取消强制结束")
	}
	h := processHandle(c.Pid, PROCESS_QUERY_LIMITED_INFORMATION|PROCESS_TERMINATE|SYNCHRONIZE)
	if h == 0 {
		return errors.New("无权限结束此进程")
	}
	defer call(pCloseHandle, h)
	p := getProcessPath(h)
	t, ok := getProcessStart(h)
	if !ok || t != c.Created || !strings.EqualFold(filepath.Clean(p), filepath.Clean(c.Path)) {
		return errors.New("进程身份校验失败，已取消操作")
	}
	if call(pTerminateProcess, h, 1) == 0 {
		return errors.New("Windows 拒绝结束该进程")
	}
	call(pWaitForSingleObject, h, 1200)
	return nil
}
func requestStillActive(item PowerRequest) bool {
	raw, err := runPowerCFG("/requests")
	if err != nil {
		return false
	}
	for _, now := range ParseRequests(raw).Items {
		if item.Kind == "PROCESS" && now.Kind == "PROCESS" && item.Section == now.Section && strings.EqualFold(item.Name, now.Name) {
			return true
		}
	}
	return false
}
func processDescription(c candidate) string {
	return fmt.Sprintf("程序：%s\nPID：%d\n完整路径：%s", c.Name, c.Pid, c.Path)
}

//go:embed assets/icon.ico
var embeddedIcon []byte

func appIcon(size int) uintptr {
	b := embeddedIcon
	if len(b) < 6 || binary.LittleEndian.Uint16(b[2:4]) != 1 {
		return 0
	}
	count := int(binary.LittleEndian.Uint16(b[4:6]))
	pick := -1
	best := 100000
	for i := 0; i < count && 6+i*16+16 <= len(b); i++ {
		off := 6 + i*16
		w := int(b[off])
		if w == 0 {
			w = 256
		}
		difference := w - size
		if difference < 0 {
			difference = -difference
		}
		if difference < best {
			best = difference
			pick = off
		}
	}
	if pick < 0 {
		return 0
	}
	length := int(binary.LittleEndian.Uint32(b[pick+8 : pick+12]))
	offset := int(binary.LittleEndian.Uint32(b[pick+12 : pick+16]))
	if length <= 0 || offset < 0 || offset+length > len(b) {
		return 0
	}
	data := b[offset : offset+length]
	return call(pCreateIconFromResourceEx, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 1, 0x30000, uintptr(size), uintptr(size), 0)
}
