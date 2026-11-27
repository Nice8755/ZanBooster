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

syms = {}
import subprocess
for l in subprocess.run(['go', 'tool', 'nm', sys.argv[1]], capture_output=True, text=True).stdout.split('\n'):
    q = l.split()
    if len(q) >= 3 and q[2].startswith('main.'): syms[q[2]] = int(q[0], 16)
def gstr(name):
    a = syms.get(name)
    if a is None: return None
    b = read(a, 16)
    if not b: return None
    ptr, n = struct.unpack('<QQ', b)
    if not (0 < n < 3000): return None
    v = read(ptr, n)
    try: return v.decode('utf-8')
    except Exception: return None
rip = re.compile(r'^\s*\S+\s+0x([0-9a-f]+)\s+([0-9a-f]+)\s+(.*)$')
stkre = re.compile(r'MOVQ \$0x([0-9a-f]+), 0x[0-9a-f]+\(SP\)')
lenre = re.compile(r'MOV[LQ] \$0x([0-9a-f]+), (?:BX|CX|DI|SI|R8|R9|R10|R11|DX)')
lines = sys.stdin.read().split('\n')
for i, ln in enumerate(lines):
    out = ln
    m = rip.match(ln)
    if m and ('LEAQ' in m.group(3)) and '(SB)' in m.group(3) and 'main.' not in m.group(3) and 'type:' not in m.group(3):
        addr = int(m.group(1), 16); code = bytes.fromhex(m.group(2))
        if len(code) >= 7 and code[1] == 0x8d:
            disp = struct.unpack_from('<i', code, 3)[0]
            tgt = addr + len(code) + disp
            n = None
            rr = re.search(r', (\w+)\s*$', m.group(3).split(';')[0].strip())
            if rr:  # LEAQ X, R; MOVQ R, off(base); MOVQ $len, off+8(base)  (ค่าในโครงสร้าง/บน stack)
                for nx in lines[i + 1:i + 4]:
                    mo = re.search(r'MOVQ ' + rr.group(1) + r', 0x([0-9a-f]+)\((SP|AX|CX|DX|BX|SI|DI|R\d+)\)', nx)
                    if mo:
                        want = '0x%x(%s)' % (int(mo.group(1), 16) + 8, mo.group(2))
                        for ny in lines[max(0, i - 4):i + 8]:
                            ml = re.search(r'MOVQ \$0x([0-9a-f]+), ' + re.escape(want), ny)
                            if ml: n = int(ml.group(1), 16); break
                        break
            if n is None:
                for nx in lines[max(0, i - 2):i]:  # ความยาวที่เก็บลง stack ก่อนหน้า (ส่วนของ filepath.Join)
                    mm = stkre.search(nx)
                    if mm: n = int(mm.group(1), 16)
            for nx in ([] if n else lines[i + 1:i + 4]):
                mm = lenre.search(nx)
                if mm: n = int(mm.group(1), 16); break
            if n is None:
                for nx in lines[i - 3:i] + lines[i + 1:i + 4]:
                    mm = stkre.search(nx)
                    if mm: n = int(mm.group(1), 16); break
            if n and 0 < n < 2000:
                b = read(tgt, n)
                if b:
                    try:
                        t = b.decode('utf-8')
                        if all(c.isprintable() or c in '\n\r\t' for c in t): out += '    ; "' + t.replace('\n', '\\n') + '"'
                    except UnicodeDecodeError: pass
    mg = re.search(r'MOVQ (main\.[A-Za-z0-9_]+)\(SB\), ', ln)
    if mg:
        v = gstr(mg.group(1))
        if v is not None: out += '    ; ' + mg.group(1) + ' = "' + v.replace('\n', '\\n')[:300] + '"'
    # ก้อนข้อมูลคงที่ (struct ที่มีแต่ข้อความ เช่น healthItem{...}) : ลองอ่านเป็นชุดของ string
    if m and 'LEAQ' in m.group(3) and '; ' not in out and '(SB)' in m.group(3) and 'main.' not in m.group(3):
        code = bytes.fromhex(m.group(2))
        if len(code) >= 7 and code[1] == 0x8d:
            tgt = int(m.group(1), 16) + len(code) + struct.unpack_from('<i', code, 3)[0]
            got = []
            try:
                for k in range(8):
                    p_, n_ = struct.unpack('<QQ', read(tgt + 16 * k, 16))
                    if n_ == 0 and p_ == 0: got.append(''); continue
                    if not (0 < n_ < 3000): break
                    got.append(read(p_, n_).decode('utf-8'))
            except Exception: got = []
            if sum(1 for g in got if g) >= 2: out += '    ; struct ' + repr(got)
            elif len(got) >= 1 and got[0]: out += '    ; boxed "' + got[0] + '"'
    # ค่าคงที่ 8 ไบต์ที่เป็นตัวอักษร (เช่น เทียบ "https://")
    mq = re.search(r'MOVQ \$0x([0-9a-f]{9,16}), ', ln)
    if mq:
        v = int(mq.group(1), 16).to_bytes(8, 'little').rstrip(b'\0')
        if v and all(32 <= c < 127 for c in v): out += '    ; "' + v.decode() + '"'
    print(out)
