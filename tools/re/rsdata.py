#!/usr/bin/env python3
"""ดึงข้อมูลพรีเซ็ต ReShade ชุดเก่า (p1-p12, ตารางหมวด ZanQV/ZanNVE) จาก exe 4.11.15 เป็นโค้ด Go
ใช้: python3 tools/re/rsdata.py beta/ZanBooster.exe > app/reshade_data.go"""
import struct, subprocess, sys
exe = open(sys.argv[1], 'rb').read()
pe = struct.unpack_from('<I', exe, 0x3c)[0]; nsec = struct.unpack_from('<H', exe, pe + 6)[0]
optsz = struct.unpack_from('<H', exe, pe + 20)[0]; base = struct.unpack_from('<Q', exe, pe + 24 + 24)[0]
secs = []
for i in range(nsec):
    o = pe + 24 + optsz + i * 40; _, vsz, va, rsz, raw = struct.unpack_from('<8sIIII', exe, o); secs.append((base + va, vsz, raw, rsz))
def read(va, n):
    for s, vsz, raw, rsz in secs:
        if s <= va < s + max(vsz, rsz): return exe[raw + va - s: raw + va - s + n]
sym = {' '.join(l.split()[2:]): int(l.split()[0], 16) for l in subprocess.run(['go', 'tool', 'nm', sys.argv[1]], capture_output=True, text=True).stdout.split('\n') if len(l.split()) >= 3}
print('//go:build windows\n\npackage main\n\n// reshade_data.go: สร้างด้วย tools/re/rsdata.py จาก exe 4.11.15 (ห้ามแก้มือ)\n')
# zgOwner: ชื่อค่า -> หมวด (1 = ZanQV, 2 = ZanNVE)
names = sym['runtime.modinfo.str'] + 440; vals = sym['unique..dict.Make[net/netip.addrDetail]'] + 3128
print('var zgOwner = map[string]int{')
for i in range(28):
    p, n = struct.unpack('<QQ', read(names + i * 16, 16)); v = struct.unpack('<q', read(vals + i * 8, 8))[0]
    print(f'\t"{read(p, n).decode()}": {v},')
print('}\n')
F = ['Black','Exposure','Clarity','Contrast','Saturation','Vibrance','SkinProtect','SkinLift','ShadowTint3','HighTint3','SplitAmt','Temp','Tint','BloomThresh','Bloom','Fade','Vignette','Sharp','Grain','SkyDeep','SkyClouds','LightTint','LightGlow','RoadDark','RoadCool','LightColor3','FoliageTint3','Filmic','AO','HorizonDehaze','SunShafts','RoadGloss','SpecPop','Atmosphere','CurveR','CurveG','CurveB','SkyGradient','NeonBoost','LensFlare','Halation','ChromaAb','RainAuto','DNRole','RoadClean','Aurora','RoadLines','RoadLift','RoadFlat','FoliageMix','FoliageSat','FoliageBright','WaterSat','CloudAmount','RainDrops','LightTwoTone','Emissive','SunWarmth','FogAmount','LightWarm3','LightWhite3','AuroraC13','AuroraC23','FogColor3']
d = sym['internal/sync..dict.entry[*internal/abi.Type,interface {}]'] + 3184
print('var zanVals = map[string]zgv{')
for k in range(12):
    f = struct.unpack('<82d', read(d + k * 656, 656)); i = 0; parts = []
    for n in F:
        if n.endswith('3'):
            v = f[i:i + 3]; i += 3
            if any(v): parts.append(f'{n[:-1]}: v3{{{v[0]!r}, {v[1]!r}, {v[2]!r}}}')
        else:
            if f[i]: parts.append(f'{n}: {f[i]!r}')
            i += 1
    print(f'\t"p{k+1}": {{' + ', '.join(parts) + '},')
print('}')
