//go:build windows

package main

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestMachineCodeThisPC(t *testing.T) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		t.Skip(err)
	}
	g, _, _ := k.GetStringValue("MachineGuid")
	k.Close()
	c := machineCode(g)
	if len(c) != 14 || c[:3] != "ZB-" || c[8] != '-' {
		t.Fatalf("bad format %q", c)
	}
	t.Log("this PC:", c)
	if _, _, err := parseLicKey("ZAN2.AAAA", machineBytes(g)); err != errKeyShort {
		t.Fatalf("want errKeyShort, got %v", err)
	}
}
