//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestZanAssets(t *testing.T) {
	zgLoad()
	if !strings.HasPrefix(zgSrc, "// ZanGrade.fx") || len(zgSrc) != 45368 {
		t.Fatalf("ZanGrade.fx ถอดไม่ถูก (%d)", len(zgSrc))
	}
	if !strings.Contains(zsTmpl, "ZAN_SHARP_ON %s") {
		t.Fatal("แม่แบบ ZanSharp ถอดไม่ถูก")
	}
}

func TestZanPresetBody(t *testing.T) {
	p := zanPresets["p1"]
	if p.File != "Zan_Preset_01.ini" {
		t.Fatal(p.File)
	}
	if !strings.HasPrefix(p.Body, "Techniques=ZanGrade@ZanGrade.fx,ZanQV@ZanQV.fx,ZanNVE@ZanNVE.fx\r\n") {
		t.Fatalf("หัวพรีเซ็ตผิด: %q", p.Body[:80])
	}
	if !strings.Contains(p.Body, "\r\n[ZanGrade.fx]\r\nBlack=0.030000\r\nWhite=1.000000\r\n") {
		t.Fatal("ค่าพรีเซ็ตผิดรูปแบบ")
	}
	if strings.Contains(p.Body, "[ZanGrade.fx]\r\nAurora=") || !strings.Contains(p.Body, "[ZanQV.fx]\r\n") {
		t.Fatal("แบ่งช่องตามหมวดผิด")
	}
	if len(zanPresets) != 12 {
		t.Fatal(len(zanPresets))
	}
}

func TestReshadeCalcID(t *testing.T) {
	if id := reshadeCalcID(); len(id) != 8 {
		t.Fatal(id)
	}
}

func TestZanSharpTmpl(t *testing.T) {
	zgLoad()
	s := strings.Count(zsTmpl, "%s")
	if s != 4 {
		t.Fatal(s)
	}
}
