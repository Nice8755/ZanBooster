#!/usr/bin/env python3
"""อ่านข้อความ go:string.*+N ยาว L จาก exe: gs.py exe N:L [N:L ...]"""
import struct, subprocess, sys
exe = open(sys.argv[1], 'rb').read()
pe = struct.unpack_from('<I', exe, 0x3c)[0]
nsec = struct.unpack_from('<H', exe, pe + 6)[0]
optsz = struct.unpack_from('<H', exe, pe + 20)[0]
base = struct.unpack_from('<Q', exe, pe + 24 + 24)[0]
secs = []
for i in range(nsec):
    o = pe + 24 + optsz + i * 40
    _, vsz, va, rsz, raw = struct.unpack_from('<8sIIII', exe, o)
    secs.append((base + va, vsz, raw, rsz))
def read(va, n):
    for s, vsz, raw, rsz in secs:
        if s <= va < s + max(vsz, rsz):
            return exe[raw + va - s: raw + va - s + n]
g = None
for l in subprocess.run(['go', 'tool', 'nm', sys.argv[1]], capture_output=True, text=True).stdout.split('\n'):
    q = l.split()
    if len(q) >= 3 and q[2] == 'go:string.*': g = int(q[0], 16)
for a in sys.argv[2:]:
    n, L = a.split(':')
    print(a, repr(read(g + int(n), int(L, 0)).decode('utf-8', 'replace')))
