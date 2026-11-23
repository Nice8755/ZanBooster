// patchexe: write an edited ui.html / engine.ps1 back into a released ZanBooster.exe.
//
// The Go source for v2.6-v4.11.15 was lost, but go:embed keeps these files raw inside the
// exe. The new file must not be longer than the original; it is padded with spaces to the
// exact original length so every pointer and length in the binary stays valid.
//
//	go run ./tools/patchexe <in.exe> <out.exe> ui=<new ui.html> engine=<new engine.ps1>
//
// Both files are optional. Line endings are converted to LF (the exe stores LF).
package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

type part struct {
	name       string
	start, end string // markers: first and last bytes of the embedded file
	padBefore  string // padding is inserted right before this (inside a comment-safe spot)
}

var parts = map[string]part{
	"ui":     {"ui.html", "<!DOCTYPE html", "</html>", "</html>"},
	"engine": {"engine.ps1", "param(\n    [string]$Action", "Emit @{ t = 'error'; action = $Action; m = $_.Exception.Message }\n}", ""},
}

func find(exe []byte, p part) (int, int, error) {
	s := bytes.Index(exe, []byte(p.start))
	if s < 0 || bytes.Index(exe[s+1:], []byte(p.start)) >= 0 {
		return 0, 0, fmt.Errorf("%s: start marker not found exactly once", p.name)
	}
	e := bytes.Index(exe[s:], []byte(p.end))
	if e < 0 {
		return 0, 0, fmt.Errorf("%s: end marker not found", p.name)
	}
	e = s + e + len(p.end)
	if e < len(exe) && exe[e] == '\n' {
		e++
	}
	return s, e, nil
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: patchexe <in.exe> <out.exe> ui=<file> engine=<file>")
		os.Exit(2)
	}
	exe, err := os.ReadFile(os.Args[1])
	must(err)
	for _, a := range os.Args[3:] {
		k, path, ok := strings.Cut(a, "=")
		p, known := parts[k]
		if !ok || !known {
			must(fmt.Errorf("bad arg %q", a))
		}
		s, e, err := find(exe, p)
		must(err)
		nb, err := os.ReadFile(path)
		must(err)
		nb = bytes.TrimPrefix(nb, []byte("\xef\xbb\xbf"))
		nb = bytes.ReplaceAll(nb, []byte("\r\n"), []byte("\n"))
		old := e - s
		if len(nb) > old {
			must(fmt.Errorf("%s is %d bytes, %d over the slot of %d. Trim it first", p.name, len(nb), len(nb)-old, old))
		}
		pad := bytes.Repeat([]byte(" "), old-len(nb))
		if p.padBefore != "" {
			i := bytes.LastIndex(nb, []byte(p.padBefore))
			nb = append(append(append([]byte{}, nb[:i]...), pad...), nb[i:]...)
		} else {
			// engine.ps1: pad on the last line (trailing spaces after the closing brace are harmless)
			nb = bytes.TrimRight(nb, "\n")
			nb = append(append(nb, pad[:max(0, len(pad)-1)]...), '\n')
			for len(nb) < old {
				nb = append(nb, ' ')
			}
		}
		copy(exe[s:e], nb)
		fmt.Printf("%s: slot %d bytes, new %d bytes, padding %d\n", p.name, old, old-len(pad), len(pad))
	}
	must(os.WriteFile(os.Args[2], exe, 0o755))
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
