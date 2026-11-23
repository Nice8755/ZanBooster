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
	// version bump first, so it only touches the original bytes (not the new ui/engine text)
	for _, a := range os.Args[3:] {
		if v, ok := strings.CutPrefix(a, "ver="); ok {
			from, to, _ := strings.Cut(v, ":")
			must(bumpVersion(exe, from, to))
		}
	}
	for _, a := range os.Args[3:] {
		if strings.HasPrefix(a, "ver=") {
			continue
		}
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

// bumpVersion swaps the version string (e.g. 4.11.15 -> 4.11.16) everywhere outside the
// embedded ui.html / engine.ps1: appVersion, window title, User-Agent, report name, the
// UTF-16 file-version resource, and the numeric VS_FIXEDFILEINFO fields.
func bumpVersion(exe []byte, from, to string) error {
	if len(from) != len(to) || from == "" {
		return fmt.Errorf("ver: %q and %q must have the same length", from, to)
	}
	var skip [][2]int
	for _, p := range parts {
		if s, e, err := find(exe, p); err == nil {
			skip = append(skip, [2]int{s, e})
		}
	}
	inSkip := func(i int) bool {
		for _, r := range skip {
			if i >= r[0] && i < r[1] {
				return true
			}
		}
		return false
	}
	utf16 := func(s string) []byte {
		b := make([]byte, 0, len(s)*2)
		for _, c := range []byte(s) {
			b = append(b, c, 0)
		}
		return b
	}
	n := 0
	for _, pair := range [][2][]byte{{[]byte(from), []byte(to)}, {utf16(from), utf16(to)}} {
		for i := 0; ; {
			j := bytes.Index(exe[i:], pair[0])
			if j < 0 {
				break
			}
			j += i
			// skip longer numbers like 4.11.150 or 14.11.15
			after := j + len(pair[0])
			digitNext := after < len(exe) && exe[after] >= '0' && exe[after] <= '9'
			digitPrev := j > 0 && exe[j-1] >= '0' && exe[j-1] <= '9'
			if !inSkip(j) && !digitNext && !digitPrev {
				copy(exe[j:], pair[1])
				n++
			}
			i = after
		}
	}
	// VS_FIXEDFILEINFO: signature FEEF04BD, then struct version, FileVersionMS/LS, ProductVersionMS/LS
	var fv, tv [4]uint16
	if _, err := fmt.Sscanf(from, "%d.%d.%d", &fv[0], &fv[1], &fv[2]); err != nil {
		return err
	}
	if _, err := fmt.Sscanf(to, "%d.%d.%d", &tv[0], &tv[1], &tv[2]); err != nil {
		return err
	}
	le := func(v [4]uint16) []byte {
		return []byte{byte(v[1]), byte(v[1] >> 8), byte(v[0]), byte(v[0] >> 8), byte(v[3]), byte(v[3] >> 8), byte(v[2]), byte(v[2] >> 8)}
	}
	sig := []byte{0xBD, 0x04, 0xEF, 0xFE}
	for i := 0; ; {
		j := bytes.Index(exe[i:], sig)
		if j < 0 || i+j+24 > len(exe) {
			break
		}
		k := i + j
		i = k + 4
		for _, off := range []int{k + 8, k + 16} {
			if bytes.Equal(exe[off:off+8], le(fv)) {
				copy(exe[off:], le(tv))
				n++
			}
		}
	}
	fmt.Printf("version %s -> %s: %d places\n", from, to, n)
	return nil
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
