package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	pRegisterClassExW = user32.NewProc("RegisterClassExW")
	pCreateWindowExW  = user32.NewProc("CreateWindowExW")
	pDefWindowProcW   = user32.NewProc("DefWindowProcW")
	pGetMessageW      = user32.NewProc("GetMessageW")
	pTranslateMessage = user32.NewProc("TranslateMessage")
	pDispatchMessageW = user32.NewProc("DispatchMessageW")
	pPostQuitMessage  = user32.NewProc("PostQuitMessage")
	pPostMessageW     = user32.NewProc("PostMessageW")
	pShowWindow       = user32.NewProc("ShowWindow")
	pUpdateWindow     = user32.NewProc("UpdateWindow")
	pDestroyWindow    = user32.NewProc("DestroyWindow")
	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pLoadCursorW      = user32.NewProc("LoadCursorW")
	pSetFocus         = user32.NewProc("SetFocus")
	pBeginPaint       = user32.NewProc("BeginPaint")
	pEndPaint         = user32.NewProc("EndPaint")
	pFillRect         = user32.NewProc("FillRect")
	pSetBkMode        = gdi32.NewProc("SetBkMode")
	pSetTextColor     = gdi32.NewProc("SetTextColor")
	pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	pDeleteObject     = gdi32.NewProc("DeleteObject")
	pCreateFontW      = gdi32.NewProc("CreateFontW")
	pSelectObject     = gdi32.NewProc("SelectObject")
	pDrawTextW        = user32.NewProc("DrawTextW")
	pGetClientRect    = user32.NewProc("GetClientRect")
	pInvalidateRect   = user32.NewProc("InvalidateRect")
	pGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	pRoundRect        = gdi32.NewProc("RoundRect")
	pCreatePen             = gdi32.NewProc("CreatePen")
	pSendMessageTimeoutW   = user32.NewProc("SendMessageTimeoutW")
	pSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	pRegOpenKeyExW         = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueExW        = advapi32.NewProc("RegSetValueExW")
	pRegCloseKey           = advapi32.NewProc("RegCloseKey")
)

const (
	wsCaption    = 0x00C00000
	wsSysMenu    = 0x00080000
	wsVisible    = 0x10000000
	wsExTopmost  = 0x00000008
	wmDestroy    = 0x0002
	wmPaint      = 0x000F
	wmKeyDown    = 0x0100
	wmLBtnDown   = 0x0201
	wmLBtnDbl    = 0x0203
	wmMouseMove  = 0x0200
	wmMouseWheel = 0x020A
	wmEraseBg    = 0x0014
	csHR         = 0x0002
	csVR         = 0x0001
	csDbl        = 0x0008
	swShow       = 5
	vkUp         = 0x26
	vkDown       = 0x28
	vkReturn     = 0x0D
	vkEsc        = 0x1B
	dtSingle     = 0x20
	dtVCtr       = 0x04
	dtCtr        = 0x01
	dtLeft       = 0x00
	transp       = 1
	hkeyCurrentUser      = 0x80000001
	keySetValue          = 0x0002
	regDword             = 4
	spiSetDeskWallpaper  = 0x0014
	spifUpdateIniFile    = 0x01
	spifSendChange       = 0x02
)

type WNDCLASSEXW struct {
	Size uint32; Style uint32; WndProc uintptr; ClsExtra, WndExtra int32
	Inst, Icon, Cursor, Bg uintptr; Menu, Class *uint16; IconSm uintptr
}
type MSG struct {
	Hwnd uintptr; Msg uint32; WP uintptr; LP uintptr; Time uint32; Pt struct{ X, Y int32 }
}
type PAINTSTRUCT struct {
	Hdc uintptr; Erase int32; Paint RECT; R int32; U int32; Res [32]byte
}
type RECT struct{ L, T, R, B int32 }

type Theme struct {
	Name, Scheme, VsTheme, VsExt, Slug string
	Bg, Fg, Ac                         uint32
	Light                              bool
}

// Work types for the worker goroutine
type workType int
const (
	workPreview workType = iota
	workApply
	workRevert
)
type workItem struct {
	kind workType
	idx  int
}

var (
	themes      []Theme
	curIdx      int
	hwndMain    uintptr
	fApp, fTitle, fBtn uintptr
	rApply, rCancel    RECT
	hovApply, hovCancel bool
	scroll     int
	maxVis     = 6
	origWT, origVS   string
	origWinLight     uint32
	origAccent       uint32
	origWallpaper    string
	logFile          *os.File
	wallpaperDir     string

	// Channel for sending work to the worker goroutine
	workCh = make(chan workItem, 32)
)

func initLog() {
	h, _ := os.UserHomeDir()
	f, _ := os.OpenFile(filepath.Join(h, "theme-picker.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if f != nil { logFile = f; log.SetOutput(f) }
	log.Println("started")
}

func hx(hex string) uint32 {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 { return 0 }
	b := func(s string) byte {
		var v byte
		for _, c := range s {
			v <<= 4
			if c >= '0' && c <= '9' { v |= byte(c-'0') } else if c >= 'a' && c <= 'f' { v |= byte(c-'a'+10) } else if c >= 'A' && c <= 'F' { v |= byte(c-'A'+10) }
		}
		return v
	}
	return uint32(b(hex[0:2])) | uint32(b(hex[2:4]))<<8 | uint32(b(hex[4:6]))<<16
}

func initThemes() {
	type t struct {
		n, slug, s, v, e, bg, fg, ac string
		light                        bool
	}
	for _, d := range []t{
		{"Catppuccin Latte", "catppuccin-latte", "Omarchy Catppuccin Latte", "Catppuccin Latte", "catppuccin.catppuccin-vsc", "#eff1f5", "#4c4f69", "#1e66f5", true},
		{"Catppuccin", "catppuccin", "Omarchy Catppuccin", "Catppuccin Mocha", "catppuccin.catppuccin-vsc", "#1e1e2e", "#cdd6f4", "#89b4fa", false},
		{"Ethereal", "ethereal", "Omarchy Ethereal", "Ethereal", "Bjarne.ethereal-omarchy", "#060B1E", "#ffcead", "#7d82d9", false},
		{"Everforest", "everforest", "Omarchy Everforest", "Everforest Dark", "sainnhe.everforest", "#2d353b", "#d3c6aa", "#7fbbb3", false},
		{"Flexoki Light", "flexoki-light", "Omarchy Flexoki Light", "flexoki-light", "shadesOfBuntu.flexoki-light", "#FFFCF0", "#100F0F", "#205EA6", true},
		{"Gruvbox", "gruvbox", "Omarchy Gruvbox", "Gruvbox Dark Medium", "jdinhlife.gruvbox", "#282828", "#d4be98", "#7daea3", false},
		{"Hackerman", "hackerman", "Omarchy Hackerman", "Hackerman", "Bjarne.hackerman-omarchy", "#0B0C16", "#ddf7ff", "#82FB9C", false},
		{"Kanagawa", "kanagawa", "Omarchy Kanagawa", "Kanagawa", "qufiwefefwoyn.kanagawa", "#1f1f28", "#dcd7ba", "#7e9cd8", false},
		{"Matte Black", "matte-black", "Omarchy Matte Black", "Matte Black", "TahaYVR.matteblack", "#121212", "#bebebe", "#e68e0d", false},
		{"Miasma", "miasma", "Omarchy Miasma", "In The Fog Dark", "ganevru.in-the-fog-theme", "#222222", "#c2c2b0", "#78824b", false},
		{"Nord", "nord", "Omarchy Nord", "Nord", "arcticicestudio.nord-visual-studio-code", "#2e3440", "#d8dee9", "#81a1c1", false},
		{"Osaka Jade", "osaka-jade", "Omarchy Osaka Jade", "Ocean Green: Dark", "jovejonovski.ocean-green", "#111c18", "#C1C497", "#509475", false},
		{"Ristretto", "ristretto", "Omarchy Ristretto", "Monokai Pro (Filter Ristretto)", "monokai.theme-monokai-pro-vscode", "#2c2525", "#e6d9db", "#f38d70", false},
		{"Rose Pine", "rose-pine", "Omarchy Rose Pine", "Rosé Pine Dawn", "mvllow.rose-pine", "#faf4ed", "#575279", "#56949f", true},
		{"Tokyo Night", "tokyo-night", "Omarchy Tokyo Night", "Tokyo Night", "enkia.tokyo-night", "#1a1b26", "#a9b1d6", "#7aa2f7", false},
		{"Vantablack", "vantablack", "Omarchy Vantablack", "Vantablack", "Bjarne.vantablack-omarchy", "#0d0d0d", "#ffffff", "#8d8d8d", false},
		{"White", "white", "Omarchy White", "White", "Bjarne.white-theme", "#ffffff", "#000000", "#6e6e6e", true},
	} {
		themes = append(themes, Theme{d.n, d.s, d.v, d.e, d.slug, hx(d.bg), hx(d.fg), hx(d.ac), d.light})
	}
}

func wtP() string {
	h, _ := os.UserHomeDir()
	matches, _ := filepath.Glob(filepath.Join(h, "AppData", "Local", "Packages", "Microsoft.WindowsTerminal*", "LocalState", "settings.json"))
	if len(matches) > 0 { return matches[0] }
	return filepath.Join(h, "AppData", "Local", "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json")
}
func vsP() string { h, _ := os.UserHomeDir(); return filepath.Join(h, "AppData", "Roaming", "Code", "User", "settings.json") }
func wpDir() string { h, _ := os.UserHomeDir(); return filepath.Join(h, "AppData", "Local", "omarchy-wsl", "wallpapers") }

// --- Registry helpers ---

func setRegDword(subkey, name string, value uint32) {
	var hKey uintptr
	sk, _ := syscall.UTF16PtrFromString(subkey)
	ret, _, _ := pRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(sk)), 0, keySetValue, uintptr(unsafe.Pointer(&hKey)))
	if ret != 0 { log.Printf("RegOpenKeyEx %s failed: %d", subkey, ret); return }
	defer pRegCloseKey.Call(hKey)
	n, _ := syscall.UTF16PtrFromString(name)
	ret, _, _ = pRegSetValueExW.Call(hKey, uintptr(unsafe.Pointer(n)), 0, regDword, uintptr(unsafe.Pointer(&value)), 4)
	if ret != 0 { log.Printf("RegSetValueEx %s failed: %d", name, ret) }
}

func getRegDword(subkey, name string) uint32 {
	pRegQueryValueExW := advapi32.NewProc("RegQueryValueExW")
	var hKey uintptr
	sk, _ := syscall.UTF16PtrFromString(subkey)
	ret, _, _ := pRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(sk)), 0, 0x0001, uintptr(unsafe.Pointer(&hKey)))
	if ret != 0 { return 0 }
	defer pRegCloseKey.Call(hKey)
	n, _ := syscall.UTF16PtrFromString(name)
	var val uint32
	var sz uint32 = 4
	pRegQueryValueExW.Call(hKey, uintptr(unsafe.Pointer(n)), 0, 0, uintptr(unsafe.Pointer(&val)), uintptr(unsafe.Pointer(&sz)))
	return val
}

func broadcastThemeChange() {
	p, _ := syscall.UTF16PtrFromString("ImmersiveColorSet")
	var result uintptr
	pSendMessageTimeoutW.Call(0xFFFF, 0x001A, 0, uintptr(unsafe.Pointer(p)), 0x0002, 5000, uintptr(unsafe.Pointer(&result)))
	// Also broadcast WindowMetrics for taskbar
	p2, _ := syscall.UTF16PtrFromString("WindowMetrics")
	pSendMessageTimeoutW.Call(0xFFFF, 0x001A, 0, uintptr(unsafe.Pointer(p2)), 0x0002, 5000, uintptr(unsafe.Pointer(&result)))
}

// DWM undocumented API for live colorization update
type dwmColorizationParams struct {
	Color            uint32
	Afterglow        uint32
	ColorBalance     uint32
	AfterglowBalance uint32
	BlurBalance      uint32
	GlassReflection  uint32
	OpaqueBlend      uint32
}

var pGetProcAddress = kernel32.NewProc("GetProcAddress")

func setDwmColor(argb uint32) {
	dwm, err := syscall.LoadLibrary("dwmapi.dll")
	if err != nil { log.Printf("LoadLibrary dwmapi.dll: %v", err); return }
	defer syscall.FreeLibrary(dwm)

	// Undocumented: ordinal 127 = DwmGetColorizationParameters, 131 = DwmSetColorizationParameters
	getAddr, _, _ := pGetProcAddress.Call(uintptr(dwm), 127)
	if getAddr == 0 { log.Printf("GetProcAddress 127 failed"); return }
	setAddr, _, _ := pGetProcAddress.Call(uintptr(dwm), 131)
	if setAddr == 0 { log.Printf("GetProcAddress 131 failed"); return }

	var params dwmColorizationParams
	ret, _, _ := syscall.SyscallN(getAddr, uintptr(unsafe.Pointer(&params)))
	if ret != 0 { log.Printf("DwmGet failed: 0x%X", ret); return }

	params.Color = argb
	params.Afterglow = argb
	ret, _, _ = syscall.SyscallN(setAddr, uintptr(unsafe.Pointer(&params)), 0)
	if ret != 0 {
		log.Printf("DwmSet failed: 0x%X", ret)
	} else {
		log.Printf("DwmSetColorizationParameters OK: 0x%08X", argb)
	}
}

// --- Windows dark/light mode + accent color ---

func setWinTheme(light bool, accentRGB uint32) {
	var mode uint32 = 0
	if light { mode = 1 }
	setRegDword(`SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme", mode)
	setRegDword(`SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "SystemUsesLightTheme", mode)

	// Convert RGB (as stored in Theme.Ac — BBGGRR) to ABGR for registry (0xFFBBGGRR)
	abgr := 0xFF000000 | accentRGB
	// ColorizationColor uses ARGB with ~C4 alpha for transparency blend
	// We need to convert BBGGRR to AARRGGBB
	r := accentRGB & 0xFF
	g := (accentRGB >> 8) & 0xFF
	b := (accentRGB >> 16) & 0xFF
	argb := 0xC4000000 | (r << 16) | (g << 8) | b
	setRegDword(`SOFTWARE\Microsoft\Windows\DWM`, "AccentColor", abgr)
	setRegDword(`SOFTWARE\Microsoft\Windows\DWM`, "ColorizationColor", argb)
	setRegDword(`SOFTWARE\Microsoft\Windows\DWM`, "ColorizationAfterglow", argb)
	setRegDword(`SOFTWARE\Microsoft\Windows\DWM`, "ColorPrevalence", 1)
	setRegDword(`SOFTWARE\Microsoft\Windows\DWM`, "EnableWindowColorization", 1)
	setRegDword(`SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "ColorPrevalence", 1)

	broadcastThemeChange()
	setDwmColor(argb)
	log.Printf("setWinTheme light=%v accent=0x%08X argb=0x%08X", light, abgr, argb)
}



func readWinTheme() (lightMode uint32, accent uint32) {
	lightMode = getRegDword(`SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme")
	accent = getRegDword(`SOFTWARE\Microsoft\Windows\DWM`, "AccentColor")
	return
}

// --- Wallpaper ---

func setWallpaper(path string) {
	if _, err := os.Stat(path); err != nil { log.Printf("wallpaper not found: %s", path); return }
	p, _ := syscall.UTF16PtrFromString(path)
	pSystemParametersInfoW.Call(spiSetDeskWallpaper, 0, uintptr(unsafe.Pointer(p)), spifUpdateIniFile|spifSendChange)
	log.Printf("setWallpaper: %s", path)
}

func getWallpaper() string {
	buf := make([]uint16, 260)
	pSystemParametersInfoW.Call(0x0073, 260, uintptr(unsafe.Pointer(&buf[0])), 0) // SPI_GETDESKWALLPAPER
	return syscall.UTF16ToString(buf)
}

func findWallpaper(slug string) string {
	dir := filepath.Join(wallpaperDir, slug)
	entries, err := os.ReadDir(dir)
	if err != nil { return "" }
	for _, e := range entries {
		n := strings.ToLower(e.Name())
		if strings.HasSuffix(n, ".jpg") || strings.HasSuffix(n, ".png") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func setWT(scheme string) {
	log.Printf("setWT: %s", scheme)
	data, err := os.ReadFile(wtP())
	if err != nil { log.Printf("err: %v", err); return }
	var s map[string]interface{}
	json.Unmarshal(data, &s)
	p, _ := s["profiles"].(map[string]interface{})
	if p == nil { return }
	d, _ := p["defaults"].(map[string]interface{})
	if d == nil { d = map[string]interface{}{}; p["defaults"] = d }
	d["colorScheme"] = scheme
	if l, ok := p["list"].([]interface{}); ok {
		for _, i := range l {
			if pr, ok := i.(map[string]interface{}); ok {
				if _, h := pr["colorScheme"]; h { pr["colorScheme"] = scheme }
			}
		}
	}
	out, _ := json.MarshalIndent(s, "", "    ")
	os.WriteFile(wtP(), out, 0644)
	log.Println("setWT done")
}

func setVS(theme string) {
	log.Printf("setVS: %s", theme)
	path := vsP()
	data, _ := os.ReadFile(path)
	c := string(data)
	re := regexp.MustCompile(`("workbench\.colorTheme"\s*:\s*)"[^"]*"`)
	if re.MatchString(c) {
		c = re.ReplaceAllString(c, `${1}"`+theme+`"`)
	} else {
		c = strings.Replace(c, "{", "{\n    \"workbench.colorTheme\": \""+theme+"\",", 1)
	}

	// Write directly using Windows CreateFile + WriteFile + FlushFileBuffers
	// This triggers NTFS change notifications that VS Code watches via ReadDirectoryChangesW
	pathW, _ := syscall.UTF16PtrFromString(path)
	h, err := syscall.CreateFile(pathW,
		syscall.GENERIC_WRITE, 0, nil, syscall.CREATE_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		log.Printf("setVS CreateFile err: %v", err)
		// Fallback to os.WriteFile
		os.WriteFile(path, []byte(c), 0644)
		return
	}
	buf := []byte(c)
	var written uint32
	syscall.WriteFile(h, buf, &written, nil)
	flushFileBuffers := kernel32.NewProc("FlushFileBuffers")
	flushFileBuffers.Call(uintptr(h))
	syscall.CloseHandle(h)
	log.Println("setVS done")
}

func installExt(ext string) {
	if ext == "" { return }
	h, _ := os.UserHomeDir()
	cp := ""
	for _, c := range []string{
		filepath.Join(h, "AppData", "Local", "Programs", "Microsoft VS Code", "bin", "code.cmd"),
		`C:\Program Files\Microsoft VS Code\bin\code.cmd`,
	} {
		if _, err := os.Stat(c); err == nil { cp = c; break }
	}
	if cp == "" { return }
	out, _ := exec.Command("cmd", "/c", cp, "--list-extensions").Output()
	for _, l := range strings.Split(string(out), "\n") {
		if strings.EqualFold(strings.TrimSpace(l), ext) { return }
	}
	log.Printf("installing %s", ext)
	exec.Command("cmd", "/c", cp, "--install-extension", ext, "--force").Run()
}

func readWT() string {
	data, _ := os.ReadFile(wtP())
	var s map[string]interface{}
	json.Unmarshal(data, &s)
	if p, _ := s["profiles"].(map[string]interface{}); p != nil {
		if d, _ := p["defaults"].(map[string]interface{}); d != nil {
			v, _ := d["colorScheme"].(string); return v
		}
	}
	return ""
}
func readVS() string {
	data, _ := os.ReadFile(vsP())
	if m := regexp.MustCompile(`"workbench\.colorTheme"\s*:\s*"([^"]+)"`).FindStringSubmatch(string(data)); len(m) > 1 { return m[1] }
	return ""
}

// Worker goroutine — runs on its own OS thread, does all file I/O.
// Only the latest preview request matters; older ones are skipped.
func worker() {
	runtime.LockOSThread()
	for w := range workCh {
		// Drain channel to get latest request (skip stale ones)
		latest := w
		drain:
		for {
			select {
			case nw := <-workCh:
				latest = nw
			default:
				break drain
			}
		}

		switch latest.kind {
		case workPreview:
			if latest.idx >= 0 && latest.idx < len(themes) {
				t := themes[latest.idx]
				log.Printf("preview %d: %s", latest.idx, t.Name)
				setWT(t.Scheme)
				setVS(t.VsTheme)
				setWinTheme(t.Light, t.Ac)
				if wp := findWallpaper(t.Slug); wp != "" { setWallpaper(wp) }
			}
		case workApply:
			if latest.idx >= 0 && latest.idx < len(themes) {
				t := themes[latest.idx]
				log.Printf("apply %d: %s", latest.idx, t.Name)
				setWT(t.Scheme)
				installExt(t.VsExt)
				setVS(t.VsTheme)
				setWinTheme(t.Light, t.Ac)
				if wp := findWallpaper(t.Slug); wp != "" { setWallpaper(wp) }
			}
			// Signal done
			pPostMessageW.Call(hwndMain, wmDestroy, 0, 0)
		case workRevert:
			log.Printf("revert")
			if origWT != "" { setWT(origWT) }
			if origVS != "" { setVS(origVS) }
			setRegDword(`SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme", origWinLight)
			setRegDword(`SOFTWARE\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "SystemUsesLightTheme", origWinLight)
			setRegDword(`SOFTWARE\Microsoft\Windows\DWM`, "AccentColor", origAccent)
			broadcastThemeChange()
			if origWallpaper != "" { setWallpaper(origWallpaper) }
			pPostMessageW.Call(hwndMain, wmDestroy, 0, 0)
		}
	}
}

// --- GUI ---

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }
func mkF(sz int, bold bool) uintptr {
	w := int32(400); if bold { w = 700 }
	f, _, _ := pCreateFontW.Call(uintptr(int32(-sz)), 0, 0, 0, uintptr(w), 0, 0, 0, 0, 0, 0, 4, 0, uintptr(unsafe.Pointer(u16("Segoe UI"))))
	return f
}
func rr(hdc uintptr, r RECT, rad int32, f, b uint32) {
	br, _, _ := pCreateSolidBrush.Call(uintptr(f))
	pn, _, _ := pCreatePen.Call(0, 1, uintptr(b))
	ob, _, _ := pSelectObject.Call(hdc, br)
	op, _, _ := pSelectObject.Call(hdc, pn)
	pRoundRect.Call(hdc, uintptr(r.L), uintptr(r.T), uintptr(r.R), uintptr(r.B), uintptr(rad), uintptr(rad))
	pSelectObject.Call(hdc, ob); pSelectObject.Call(hdc, op)
	pDeleteObject.Call(br); pDeleteObject.Call(pn)
}
func ds(hdc uintptr, s string, r *RECT, fl uintptr) {
	p := u16(s); pDrawTextW.Call(hdc, uintptr(unsafe.Pointer(p)), uintptr(len(s)), uintptr(unsafe.Pointer(r)), fl)
}
func ensVis() {
	if curIdx < scroll { scroll = curIdx }
	if curIdx >= scroll+maxVis { scroll = curIdx - maxVis + 1 }
}
func inR(x, y int32, r RECT) bool { return x >= r.L && x <= r.R && y >= r.T && y <= r.B }

func wndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	switch msg {
	case wmPaint:
		var ps PAINTSTRUCT
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		var cr RECT
		pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
		bg, _, _ := pCreateSolidBrush.Call(0x1e1e1e)
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&cr)), bg); pDeleteObject.Call(bg)
		pSetBkMode.Call(hdc, transp)
		pSelectObject.Call(hdc, fTitle); pSetTextColor.Call(hdc, 0xffffff)
		tr := RECT{20, 12, cr.R - 20, 44}; ds(hdc, "Omarchy Theme Picker", &tr, dtLeft|dtSingle|dtVCtr)
		pSelectObject.Call(hdc, fApp); pSetTextColor.Call(hdc, 0x888888)
		sr := RECT{20, 44, cr.R - 80, 64}; ds(hdc, "Arrow/Click: preview | Enter: apply | Esc: cancel", &sr, dtLeft|dtSingle|dtVCtr)
		if len(themes) > maxVis {
			pSetTextColor.Call(hdc, 0x666666)
			ind := fmt.Sprintf("%d/%d", curIdx+1, len(themes))
			ir := RECT{cr.R - 80, 44, cr.R - 12, 64}; ds(hdc, ind, &ir, dtCtr|dtSingle|dtVCtr)
		}
		vis := maxVis; if len(themes) < vis { vis = len(themes) }
		pSelectObject.Call(hdc, fApp)
		for vi := 0; vi < vis; vi++ {
			i := scroll + vi; if i >= len(themes) { break }
			t := themes[i]; y := int32(76 + vi*42)
			r := RECT{12, y, cr.R - 12, y + 38}
			if i == curIdx { rr(hdc, r, 6, t.Bg, t.Ac); pSetTextColor.Call(hdc, uintptr(t.Fg)) } else { rr(hdc, r, 6, 0x2a2a2a, 0x2a2a2a); pSetTextColor.Call(hdc, 0xcccccc) }
			nr := RECT{r.L + 16, r.T, r.R - 80, r.B}; ds(hdc, t.Name, &nr, dtLeft|dtSingle|dtVCtr)
			dy := y + 13; dx := r.R - 60
			for _, c := range []uint32{t.Bg, t.Fg, t.Ac} { rr(hdc, RECT{dx, dy, dx + 12, dy + 12}, 6, c, 0x555555); dx += 16 }
		}
		by := int32(76 + vis*42 + 10); bw := int32(120); bh := int32(36)
		bx := (cr.R - bw*2 - 12) / 2
		pSelectObject.Call(hdc, fBtn)
		rApply = RECT{bx, by, bx + bw, by + bh}
		ab := uint32(0x4CAF50); if hovApply { ab = 0x66BB6A }
		rr(hdc, rApply, 8, ab, 0x388E3C); pSetTextColor.Call(hdc, 0xffffff); ds(hdc, "Apply", &rApply, dtCtr|dtSingle|dtVCtr)
		rCancel = RECT{bx + bw + 12, by, bx + bw*2 + 12, by + bh}
		cb := uint32(0x444444); if hovCancel { cb = 0x555555 }
		rr(hdc, rCancel, 8, cb, 0x555555); pSetTextColor.Call(hdc, 0xcccccc); ds(hdc, "Cancel", &rCancel, dtCtr|dtSingle|dtVCtr)
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmEraseBg:
		return 1
	case wmKeyDown:
		switch wp {
		case vkUp:
			if curIdx > 0 { curIdx-- } else { curIdx = len(themes) - 1 }
			ensVis(); workCh <- workItem{workPreview, curIdx}; pInvalidateRect.Call(hwnd, 0, 1)
		case vkDown:
			if curIdx < len(themes)-1 { curIdx++ } else { curIdx = 0 }
			ensVis(); workCh <- workItem{workPreview, curIdx}; pInvalidateRect.Call(hwnd, 0, 1)
		case vkReturn:
			workCh <- workItem{workApply, curIdx}
		case vkEsc:
			workCh <- workItem{workRevert, 0}
		}
		return 0
	case wmLBtnDown:
		cx := int32(lp & 0xFFFF); cy := int32(lp >> 16)
		if inR(cx, cy, rApply) { workCh <- workItem{workApply, curIdx}; return 0 }
		if inR(cx, cy, rCancel) { workCh <- workItem{workRevert, 0}; return 0 }
		idx := scroll + int(cy-76)/42
		if idx >= 0 && idx < len(themes) {
			curIdx = idx; ensVis(); workCh <- workItem{workPreview, curIdx}; pInvalidateRect.Call(hwnd, 0, 1)
		}
		return 0
	case wmLBtnDbl:
		idx := scroll + int(int32(lp>>16)-76)/42
		if idx >= 0 && idx < len(themes) { curIdx = idx; workCh <- workItem{workApply, curIdx} }
		return 0
	case wmMouseMove:
		mx := int32(lp & 0xFFFF); my := int32(lp >> 16)
		na := inR(mx, my, rApply); nc := inR(mx, my, rCancel)
		if na != hovApply || nc != hovCancel { hovApply = na; hovCancel = nc; pInvalidateRect.Call(hwnd, 0, 1) }
		return 0
	case wmMouseWheel:
		d := int16(wp >> 16)
		if d > 0 && curIdx > 0 { curIdx-- } else if d < 0 && curIdx < len(themes)-1 { curIdx++ }
		ensVis(); workCh <- workItem{workPreview, curIdx}; pInvalidateRect.Call(hwnd, 0, 1)
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0); return 0
	}
	ret, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return ret
}

func main() {
	runtime.LockOSThread() // Lock main goroutine to OS thread for Windows message loop

	initLog()
	defer func() { if logFile != nil { logFile.Close() } }()
	initThemes()
	wallpaperDir = wpDir()

	// CLI mode: theme-picker.exe --apply <slug>
	// Used by the bash theme script to set Windows theme/accent/wallpaper
	if len(os.Args) >= 3 && os.Args[1] == "--apply" {
		slug := os.Args[2]
		for _, t := range themes {
			if t.Slug == slug {
				log.Printf("CLI apply: %s", t.Name)
				setWinTheme(t.Light, t.Ac)
				if wp := findWallpaper(t.Slug); wp != "" { setWallpaper(wp) }
				fmt.Printf("Windows: %s mode, accent applied, wallpaper set\n", map[bool]string{true: "light", false: "dark"}[t.Light])
				return
			}
		}
		fmt.Fprintf(os.Stderr, "Unknown theme: %s\n", slug)
		os.Exit(1)
	}

	origWT = readWT(); origVS = readVS()
	origWinLight, origAccent = readWinTheme()
	origWallpaper = getWallpaper()
	log.Printf("orig WT=%s VS=%s WinLight=%d Accent=0x%08X Wallpaper=%s", origWT, origVS, origWinLight, origAccent, origWallpaper)
	for i, t := range themes { if t.Scheme == origWT { curIdx = i; break } }
	ensVis()

	// Start worker goroutine BEFORE creating window
	go worker()

	hI, _, _ := pGetModuleHandleW.Call(0)
	cur, _, _ := pLoadCursorW.Call(0, 32512)
	cl := u16("OmarchyTP")
	fApp = mkF(14, false); fTitle = mkF(18, true); fBtn = mkF(14, true)
	wc := WNDCLASSEXW{Size: uint32(unsafe.Sizeof(WNDCLASSEXW{})), Style: csHR | csVR | csDbl,
		WndProc: syscall.NewCallback(wndProc), Inst: hI, Cursor: cur, Class: cl}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	v := maxVis; if len(themes) < v { v = len(themes) }
	w := int32(400); h := int32(76 + int32(v)*42 + 70 + 32) // +32 for title bar chrome
	sw, _, _ := pGetSystemMetrics.Call(0); sh, _, _ := pGetSystemMetrics.Call(1)
	hwnd, _, _ := pCreateWindowExW.Call(wsExTopmost, uintptr(unsafe.Pointer(cl)),
		uintptr(unsafe.Pointer(u16("Omarchy Themes"))), wsCaption|wsSysMenu|wsVisible,
		uintptr((int32(sw)-w)/2), uintptr((int32(sh)-h)/2), uintptr(w), uintptr(h), 0, 0, hI, 0)
	hwndMain = hwnd
	pShowWindow.Call(hwnd, swShow); pUpdateWindow.Call(hwnd); pSetFocus.Call(hwnd)

	// Message loop — completely free, no I/O here
	var msg MSG
	for {
		ret, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if ret == 0 { break }
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	pDeleteObject.Call(fApp); pDeleteObject.Call(fTitle); pDeleteObject.Call(fBtn)
	log.Println("exit")
}
