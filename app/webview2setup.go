//go:build windows

package main

import (
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const webview2ClientID = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// hasWebView2 เช็กว่ามี Microsoft Edge WebView2 Runtime ในเครื่องหรือยัง (ตามวิธีที่ Microsoft แนะนำ)
func hasWebView2() bool {
	keys := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + webview2ClientID},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\EdgeUpdate\Clients\` + webview2ClientID},
		{registry.CURRENT_USER, `Software\Microsoft\EdgeUpdate\Clients\` + webview2ClientID},
	}
	for _, k := range keys {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		pv, _, err := key.GetStringValue("pv")
		key.Close()
		if err == nil && pv != "" && pv != "0.0.0.0" {
			return true
		}
	}
	return false
}

var (
	pCreateWindowEx  = user32.NewProc("CreateWindowExW")
	pDestroyWindow   = user32.NewProc("DestroyWindow")
	pPeekMessage     = user32.NewProc("PeekMessageW")
	pTranslateMsg    = user32.NewProc("TranslateMessage")
	pDispatchMessage = user32.NewProc("DispatchMessageW")
	pSendMessage     = user32.NewProc("SendMessageW")
	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	pCreateFont      = gdi32.NewProc("CreateFontW")
)

// installWebView2 โหลดตัวติดตั้งจาก Microsoft แล้วติดตั้งแบบเงียบ ระหว่างนั้นแสดงหน้าต่างเล็ก ๆ บอกสถานะ
func installWebView2() bool {
	// หน้าต่างสถานะ
	cls, _ := windows.UTF16PtrFromString("STATIC")
	txt, _ := windows.UTF16PtrFromString("\r\nZan Booster\r\n\r\nกำลังเตรียมโปรแกรมสำหรับใช้งานครั้งแรก\r\n(ติดตั้ง Microsoft WebView2 อัตโนมัติ ประมาณ 1-2 นาที)")
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	w, h := uintptr(460), uintptr(150)
	const wsPopup, wsVisible, wsBorder, ssCenter = 0x80000000, 0x10000000, 0x00800000, 0x1
	const wsExTopmost, wsExToolWindow = 0x8, 0x80
	hwnd, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolWindow, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(txt)),
		wsPopup|wsVisible|wsBorder|ssCenter, (sw-w)/2, (sh-h)/2, w, h, 0, 0, 0, 0)
	if hwnd != 0 {
		face, _ := windows.UTF16PtrFromString("Leelawadee UI")
		font, _, _ := pCreateFont.Call(^uintptr(19), 0, 0, 0, 400, 0, 0, 0, 0, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
		pSendMessage.Call(hwnd, 0x0030, font, 1) // WM_SETFONT
		defer pDestroyWindow.Call(hwnd)
	}

	done := make(chan bool, 1)
	go func() {
		setup := filepath.Join(os.TempDir(), "MicrosoftEdgeWebview2Setup.exe")
		cl := &http.Client{Timeout: 3 * time.Minute}
		resp, err := cl.Get("https://go.microsoft.com/fwlink/p/?LinkId=2124703")
		if err != nil {
			done <- false
			return
		}
		f, err := os.Create(setup)
		if err != nil {
			resp.Body.Close()
			done <- false
			return
		}
		_, err = io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()
		if err != nil {
			done <- false
			return
		}
		cmd := exec.Command(setup, "/silent", "/install")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		err = cmd.Run()
		os.Remove(setup)
		done <- err == nil && hasWebView2()
	}()

	// วนรับ message ให้หน้าต่างไม่ค้างระหว่างติดตั้ง
	var msg [48]byte
	for {
		select {
		case ok := <-done:
			return ok
		default:
		}
		for {
			r, _, _ := pPeekMessage.Call(uintptr(unsafe.Pointer(&msg[0])), 0, 0, 0, 1)
			if r == 0 {
				break
			}
			pTranslateMsg.Call(uintptr(unsafe.Pointer(&msg[0])))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&msg[0])))
		}
		time.Sleep(30 * time.Millisecond)
	}
}
