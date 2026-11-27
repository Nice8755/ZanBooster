#!/usr/bin/env python3
"""ถอดตัว exe เก่า (v4.11.15) เพื่อสร้างโค้ด Go ที่หายไปใหม่
อ่านผลของ `go tool objdump` แล้วเติมข้อความ (string) ที่แต่ละบรรทัดอ้างถึง
ใช้: go tool objdump -s 'main\\.licAPI' beta/ZanBooster.exe | python3 tools/re/annotate.py beta/ZanBooster.exe
"""
import re, struct, sys

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
            off = raw + va - s
            return exe[off:off + n]
    return None

rip = re.compile(r'^\s*\S+\s+0x([0-9a-f]+)\s+([0-9a-f]+)\s+(.*)$')
lenre = re.compile(r'MOV[LQ] \$0x([0-9a-f]+), (?:BX|CX|DI|SI|R8|R9|R10|R11|DX)')
lines = sys.stdin.read().split('\n')
for i, ln in enumerate(lines):
    out = ln
    m = rip.match(ln)
    if m and ('LEAQ' in m.group(3)) and ('go:string' in m.group(3) or 'rodata' in m.group(3)):
        addr = int(m.group(1), 16); code = bytes.fromhex(m.group(2))
        if len(code) >= 7 and code[1] == 0x8d:
            disp = struct.unpack_from('<i', code, 3)[0]
            tgt = addr + len(code) + disp
            n = None
            for nx in lines[i + 1:i + 4]:
                mm = lenre.search(nx)
                if mm: n = int(mm.group(1), 16); break
            if n and 0 < n < 2000:
                b = read(tgt, n)
                if b:
                    try: out += '    ; "' + b.decode('utf-8').replace('\n', '\\n') + '"'
                    except UnicodeDecodeError: out += '    ; bytes ' + b[:40].hex()
    # ค่าคงที่ 8 ไบต์ที่เป็นตัวอักษร (เช่น เทียบ "https://")
    mq = re.search(r'MOVQ \$0x([0-9a-f]{9,16}), ', ln)
    if mq:
        v = int(mq.group(1), 16).to_bytes(8, 'little').rstrip(b'\0')
        if v and all(32 <= c < 127 for c in v): out += '    ; "' + v.decode() + '"'
    print(out)
