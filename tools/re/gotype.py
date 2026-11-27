#!/usr/bin/env python3
"""อ่านโครงสร้าง struct ของ Go (ชื่อช่อง ชนิด ตำแหน่ง และ json tag) จากข้อมูล type ใน exe
ใช้: python3 tools/re/gotype.py beta/ZanBooster.exe 555456      (ตัวเลข = runtime.rodata+N ที่เห็นใน objdump)
     python3 tools/re/gotype.py beta/ZanBooster.exe 0x6f1234    (ที่อยู่ตรง ๆ)"""
import struct, subprocess, sys
exe = open(sys.argv[1], 'rb').read()
pe = struct.unpack_from('<I', exe, 0x3c)[0]
nsec = struct.unpack_from('<H', exe, pe + 6)[0]; optsz = struct.unpack_from('<H', exe, pe + 20)[0]
base = struct.unpack_from('<Q', exe, pe + 24 + 24)[0]
secs = []
for i in range(nsec):
    o = pe + 24 + optsz + i * 40
    _, vsz, va, rsz, raw = struct.unpack_from('<8sIIII', exe, o); secs.append((base + va, vsz, raw, rsz))
def read(va, n):
    for s, vsz, raw, rsz in secs:
        if s <= va < s + max(vsz, rsz): return exe[raw + va - s: raw + va - s + n]
    raise ValueError(hex(va))
syms = {}
for l in subprocess.run(['go', 'tool', 'nm', sys.argv[1]], capture_output=True, text=True).stdout.split('\n'):
    q = l.split()
    if len(q) >= 3: syms[q[2]] = int(q[0], 16)
rodata = syms.get('runtime.rodata'); types = syms.get('runtime.types', rodata)
KINDS = {1:'bool',2:'int',3:'int8',4:'int16',5:'int32',6:'int64',7:'uint',8:'uint8',9:'uint16',10:'uint32',11:'uint64',12:'uintptr',13:'float32',14:'float64',17:'array',18:'chan',19:'func',20:'interface',21:'map',22:'ptr',23:'slice',24:'string',25:'struct',26:'unsafe.Pointer'}
def varint(b, o):
    v = s = 0
    while True:
        c = b[o]; o += 1; v |= (c & 0x7f) << s; s += 7
        if not c & 0x80: return v, o
def name(p):
    b = read(p, 600); flags = b[0]; n, o = varint(b, 1); nm = b[o:o + n].decode('utf-8', 'replace'); o += n; tag = ''
    if flags & 2:
        tn, o = varint(b, o); tag = b[o:o + tn].decode('utf-8', 'replace')
    return nm, tag
def tname(t):
    h = read(t, 48); kind = h[23] & 0x1f; strOff = struct.unpack_from('<i', h, 40)[0]
    try: s = name(types + strOff)[0]
    except Exception: s = '?'
    if h[20] & 2: s = s[1:]  # TFlagExtraStar
    return s, kind
def dump(t, depth=0):
    s, kind = tname(t); print('  ' * depth + f'{s}  ({KINDS.get(kind, kind)})')
    if kind == 25:
        fp, fl = struct.unpack_from('<QQ', read(t + 56, 16))
        for i in range(fl):
            np, tp, off = struct.unpack_from('<QQQ', read(fp + i * 24, 24))
            fn, tag = name(np); ts, tk = tname(tp)
            print('  ' * depth + f'  +{off:<4} {fn:<14} {ts:<28} `{tag}`')
a = sys.argv[2]
t = int(a, 16) if a.startswith('0x') else rodata + int(a)
dump(t)
