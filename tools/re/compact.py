#!/usr/bin/env python3
"""ย่อผลของ annotate.py ให้อ่านง่าย: แสดงทีละบรรทัดของโค้ดเดิม เฉพาะคำสั่งที่บอกความหมาย
(CALL ฟังก์ชัน, ข้อความ, ค่าคงที่, ตัวแปรระดับบน) ตัดคำสั่งจัดการ stack ทิ้ง"""
import re, sys
keep = re.compile(r'CALL|; "|\$0x|main\.|JMP|J[A-Z]+ |CMP|TEST')
skip = re.compile(r'morestack|INT \$0x3|runtime\.(gcWriteBarrier|morestack)|CMPQ SP, 0x10\(R14\)')
cur = None
for ln in sys.stdin:
    ln = ln.rstrip('\n')
    if ln.startswith('TEXT '):
        print('\n' + ln.split()[1]); cur = None; continue
    p = ln.split('\t')
    if len(p) < 4: continue
    src = p[0].strip()
    rest = [x for x in p[1:] if x.strip()]
    if len(rest) < 3: continue
    ins = rest[2].strip(); ann = ' '.join(x.strip() for x in rest[3:])
    full = ins + ('  ' + ann if ann else '')
    if skip.search(full) or not keep.search(full): continue
    if src != cur:
        print('  ' + src); cur = src
    print('      ' + full)
