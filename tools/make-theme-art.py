# วาดรูปพื้นหลังของธีมการ์ตูน (themes/*.svg) พร้อมการขยับในตัว (CSS ใน SVG)
# ตัวโปรแกรมหยุดการขยับเองตอนไม่ได้ใช้/เกมเปิด ด้วยการใส่ animation-play-state:paused
#   python3 tools/make-theme-art.py   (รันจากรากที่เก็บ) แล้วเพิ่ม ART_V ใน ui.html เพื่อให้ลูกค้าโหลดรูปใหม่
import random, math, os
random.seed(7)
W, H = 1600, 1000
OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'themes')
BASE = '.tw{animation:tw 3.2s ease-in-out infinite}@keyframes tw{50%{opacity:.15}}.f{transform-box:fill-box;transform-origin:center}.fb{transform-box:fill-box;transform-origin:50% 100%}'
def svg(body, defs='', css='', extra=''):
    return f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" preserveAspectRatio="xMidYMid slice"{extra}><defs><style>{BASE}{css}</style>{defs}</defs>{body}</svg>'
def dl(): return f' style="animation-delay:-{random.uniform(0, 4):.1f}s"'
def stars(n, col, rmax=2.2, y1=0, y2=600, x1=0, x2=W):
    return ''.join(f'<circle class="tw" cx="{random.randint(x1, x2)}" cy="{random.randint(y1, y2)}" r="{random.uniform(.6, rmax):.1f}" fill="{col}"{dl()}/>' for _ in range(n))
def save(n, t):
    open(os.path.join(OUT, n), 'w').write(t)

# ---------------- POP ----------------
d = '''<radialGradient id="s" cx="78%" cy="30%" r="70%"><stop offset="0" stop-color="#ffd400"/><stop offset=".35" stop-color="#ff7a1a"/><stop offset="1" stop-color="#2a0f3a"/></radialGradient>
<pattern id="h" width="16" height="16" patternUnits="userSpaceOnUse"><circle cx="8" cy="8" r="3.2" fill="#000" opacity=".22"/></pattern>'''
css = '.ray{transform-origin:1250px 300px;animation:spin 90s steps(540) infinite}@keyframes spin{to{transform:rotate(360deg)}}.bob{animation:bob 2.4s ease-in-out infinite alternate}@keyframes bob{to{transform:translateY(-18px) rotate(1deg)}}.zap{animation:zap 3s steps(1) infinite}@keyframes zap{0%,88%,94%{opacity:1}90%,96%{opacity:.25}}.win{animation:win 5s steps(1) infinite}@keyframes win{60%{opacity:.15}}.spin2{animation:sp2 6s linear infinite}@keyframes sp2{to{transform:rotate(360deg)}}'
b = '<rect width="1600" height="1000" fill="url(#s)"/>'
cx, cy = 1250, 300
rays = ''
for i in range(24):
    a1 = i * 2 * math.pi / 24; a2 = a1 + math.pi / 24
    rays += f'M{cx},{cy}L{cx + 1800 * math.cos(a1):.0f},{cy + 1800 * math.sin(a1):.0f}L{cx + 1800 * math.cos(a2):.0f},{cy + 1800 * math.sin(a2):.0f}Z'
b += f'<path class="ray" d="{rays}" fill="#ffd400" opacity=".18"/><rect width="1600" height="1000" fill="url(#h)"/>'
x = 0; sky = ''; win = ''
while x < 1600:
    w = random.randint(70, 150); h = random.randint(180, 460)
    sky += f'<rect x="{x}" y="{1000 - h}" width="{w}" height="{h}" fill="#1b0b2e" stroke="#000" stroke-width="6"/>'
    for wy in range(1000 - h + 24, 990, 38):
        for wx in range(x + 14, x + w - 18, 30):
            if random.random() < .45:
                win += f'<rect{" class=\"win\"" + dl() if random.random() < .12 else ""} x="{wx}" y="{wy}" width="14" height="18" fill="#ffd400"/>'
    x += w - 4
b += sky + win
b += '''<g transform="translate(1180 250) rotate(-8)"><g class="bob"><path d="M-230,-110 Q0,-200 230,-110 Q320,0 230,110 Q120,170 40,150 L-40,250 L-20,140 Q-200,150 -260,60 Q-320,-30 -230,-110Z" fill="#fff" stroke="#000" stroke-width="12"/>
<text x="0" y="40" text-anchor="middle" font-family="Impact,Arial Black,sans-serif" font-size="150" fill="#ff3b3b" stroke="#000" stroke-width="7" paint-order="stroke">ZAN!</text></g></g>
<path class="zap" d="M260,120 L330,120 L280,240 L350,240 L210,450 L250,290 L190,290Z" fill="#ffd400" stroke="#000" stroke-width="10" stroke-linejoin="round"/>
<g transform="translate(142 590)"><path class="f spin2" d="M0,-70 l22,46 50,7 -36,35 9,50 -45,-24 -45,24 9,-50 -36,-35 50,-7z" fill="#29d3ff" stroke="#000" stroke-width="8" stroke-linejoin="round"/></g>'''
save('pop.svg', svg(b, d, css))

# ---------------- KAWAII ----------------
d = '''<linearGradient id="k" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#2a1640"/><stop offset=".6" stop-color="#5a2a62"/><stop offset="1" stop-color="#ff8fc7"/></linearGradient>
<radialGradient id="m" cx=".4" cy=".4"><stop offset="0" stop-color="#fff8fd"/><stop offset="1" stop-color="#ffd6ea"/></radialGradient>'''
css = '.cl{animation:cl 14s ease-in-out infinite alternate}@keyframes cl{to{transform:translateX(60px)}}.cl2{animation:cl 18s ease-in-out infinite alternate-reverse}.ht{animation:ht 6s ease-in infinite}@keyframes ht{0%{transform:translateY(0);opacity:0}15%{opacity:1}100%{transform:translateY(-220px);opacity:0}}.tail{transform-origin:110px 80px;animation:tail 1.6s ease-in-out infinite alternate}@keyframes tail{to{transform:rotate(-16deg)}}.eye{transform-box:fill-box;transform-origin:center;animation:blink 4.5s infinite}@keyframes blink{0%,93%,100%{transform:scaleY(1)}96%{transform:scaleY(.1)}}.cat{animation:breathe 3s ease-in-out infinite alternate}@keyframes breathe{to{transform:translateY(-6px)}}'
b = '<rect width="1600" height="1000" fill="url(#k)"/>' + stars(90, '#fff', 2.4, 0, 650)
b += '<circle cx="300" cy="210" r="110" fill="url(#m)"/><circle cx="265" cy="190" r="16" fill="#ffc2df"/><circle cx="330" cy="250" r="22" fill="#ffc2df"/>'
def cloud(x, y, s, c='#fff', o=1, k='cl'):
    return f'<g transform="translate({x} {y}) scale({s})"><g class="{k}" fill="{c}" opacity="{o}"><circle cx="0" cy="0" r="60"/><circle cx="70" cy="-30" r="80"/><circle cx="160" cy="0" r="65"/><rect x="0" y="0" width="160" height="60" rx="30"/></g></g>'
b += cloud(80, 820, 1.3, '#ffd6ea', .85) + cloud(560, 880, 1.1, '#ffe8f4', .9, 'cl2') + cloud(1300, 300, .8, '#ffd6ea', .5, 'cl2') + cloud(700, 180, .7, '#ffe8f4', .4)
heart = '<path d="M0,10 C0,-10 30,-10 30,10 C30,-10 60,-10 60,10 C60,35 30,50 30,60 C30,50 0,35 0,10Z" fill="#ff8fc7"/>'
for i, (x, y, s) in enumerate([(900, 340, .9), (1480, 620, .7), (520, 520, .6), (160, 620, .5), (1250, 320, .6), (1050, 700, .5)]):
    b += f'<g transform="translate({x} {y}) scale({s}) rotate({random.randint(-20, 20)})"><g class="ht" style="animation-delay:-{i * 1.05:.1f}s">{heart}</g></g>'
b += cloud(1100, 860, 2.4, '#fff', .95, 'none')
b += '''<g transform="translate(1330 640)"><g class="cat">
<path class="tail" d="M120,120 Q230,90 210,-20 Q200,-60 170,-50 Q190,40 110,80Z" fill="#fff" stroke="#3a2140" stroke-width="7"/>
<ellipse cx="0" cy="110" rx="130" ry="95" fill="#fff" stroke="#3a2140" stroke-width="7"/>
<path d="M-150,-60 L-160,-200 L-60,-130Z M150,-60 L160,-200 L60,-130Z" fill="#fff" stroke="#3a2140" stroke-width="7" stroke-linejoin="round"/>
<path d="M-140,-90 L-145,-170 L-85,-128Z M140,-90 L145,-170 L85,-128Z" fill="#ff8fc7"/>
<ellipse cx="0" cy="-40" rx="170" ry="135" fill="#fff" stroke="#3a2140" stroke-width="7"/>
<g class="eye"><ellipse cx="-62" cy="-40" rx="26" ry="34" fill="#3a2140"/><circle cx="-52" cy="-54" r="10" fill="#fff"/><circle cx="-70" cy="-28" r="5" fill="#fff"/></g>
<g class="eye"><ellipse cx="62" cy="-40" rx="26" ry="34" fill="#3a2140"/><circle cx="72" cy="-54" r="10" fill="#fff"/><circle cx="54" cy="-28" r="5" fill="#fff"/></g>
<ellipse cx="-110" cy="10" rx="28" ry="16" fill="#ff8fc7" opacity=".7"/><ellipse cx="110" cy="10" rx="28" ry="16" fill="#ff8fc7" opacity=".7"/>
<path d="M-16,4 Q0,20 16,4" fill="none" stroke="#3a2140" stroke-width="6" stroke-linecap="round"/><path d="M-6,-6 L6,-6 L0,2Z" fill="#ff8fc7"/>
<path d="M-60,150 q0,40 30,40 M60,150 q0,40 -30,40" fill="none" stroke="#3a2140" stroke-width="6"/>
<path d="M-40,-175 q40,-30 80,0" fill="none" stroke="#ffd6ea" stroke-width="10" stroke-linecap="round"/>
</g></g>'''
save('kawaii.svg', svg(b, d, css))

# ---------------- TOON ----------------
d = '<linearGradient id="t" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#0b1730"/><stop offset="1" stop-color="#1d3d6e"/></linearGradient>'
css = '.moon{animation:moon 3s ease-in-out infinite alternate}@keyframes moon{0%{transform:rotate(-6deg)}100%{transform:rotate(6deg) translateY(-10px)}}.cl{animation:cl 16s ease-in-out infinite alternate}@keyframes cl{to{transform:translateX(90px)}}.tree{animation:tree 2.2s ease-in-out infinite alternate}@keyframes tree{from{transform:rotate(-4deg)}to{transform:rotate(4deg)}}'
b = '<rect width="1600" height="1000" fill="url(#t)"/>' + stars(70, '#fff6c2', 2.6, 0, 600)
b += '''<g transform="translate(1260 230)"><g class="moon"><circle r="130" fill="#ffd86b" stroke="#05070c" stroke-width="10"/><circle cx="-40" cy="-20" r="14" fill="#05070c"/><circle cx="40" cy="-20" r="14" fill="#05070c"/><path d="M-55,30 Q0,85 55,30" fill="none" stroke="#05070c" stroke-width="10" stroke-linecap="round"/><circle cx="-75" cy="20" r="18" fill="#ff9f1c" opacity=".6"/><circle cx="75" cy="20" r="18" fill="#ff9f1c" opacity=".6"/></g></g>'''
for i, (x, y, s) in enumerate([(180, 160, 1), (700, 110, .8), (980, 330, .6)]):
    b += f'<g transform="translate({x} {y}) scale({s})"><g class="cl" style="animation-delay:-{i * 5}s" fill="#fff" stroke="#05070c" stroke-width="9"><path d="M0,60 Q-10,0 50,10 Q70,-40 130,-10 Q190,-30 200,30 Q250,40 230,80 L10,80 Q-20,80 0,60Z"/></g></g>'
b += '<path d="M0,760 Q300,600 620,740 T1600,700 L1600,1000 L0,1000Z" fill="#2ec4ff" stroke="#05070c" stroke-width="10"/>'
b += '<path d="M0,860 Q400,720 820,860 T1600,820 L1600,1000 L0,1000Z" fill="#7bff5c" stroke="#05070c" stroke-width="10"/>'
b += '<path d="M0,940 Q500,860 1000,950 T1600,930 L1600,1000 L0,1000Z" fill="#3fb83a" stroke="#05070c" stroke-width="10"/>'
for i, x in enumerate((240, 520, 1080, 1420)):
    b += f'<g transform="translate({x} {880 if x < 800 else 860})"><g class="tree" style="transform-origin:0 40px;animation-delay:-{i * .6:.1f}s"><rect x="-10" y="-20" width="20" height="60" fill="#8b5a2b" stroke="#05070c" stroke-width="6"/><circle cy="-50" r="50" fill="#5cff6e" stroke="#05070c" stroke-width="8"/></g></g>'
save('toon.svg', svg(b, d, css))

# ---------------- MANGA ----------------
css = '.sl{animation:sl 1.4s steps(2) infinite}@keyframes sl{50%{opacity:.08}}.scarf{transform-origin:1125px 605px;animation:scarf .9s ease-in-out infinite alternate}@keyframes scarf{from{transform:skewY(-4deg) scaleX(.96)}to{transform:skewY(5deg) scaleX(1.05)}}.don{transform-origin:300px 260px;animation:don 2.4s ease-out infinite}@keyframes don{0%{transform:scale(.9);opacity:.15}10%{transform:scale(1.06);opacity:.45}100%{transform:scale(1);opacity:.25}}.glow{animation:glow 3s ease-in-out infinite alternate}@keyframes glow{to{stroke-width:22;opacity:.7}}'
cx, cy = 1150, 420
lines = ''
for i in range(90):
    a = i * 2 * math.pi / 90 + random.uniform(-.02, .02); r1 = random.randint(330, 420); w = random.uniform(.006, .016)
    lines += f'M{cx + r1 * math.cos(a):.0f},{cy + r1 * math.sin(a):.0f}L{cx + 2000 * math.cos(a - w):.0f},{cy + 2000 * math.sin(a - w):.0f}L{cx + 2000 * math.cos(a + w):.0f},{cy + 2000 * math.sin(a + w):.0f}Z'
b = '<rect width="1600" height="1000" fill="#0b0b0e"/>' + f'<g class="sl"><path d="{lines}" fill="#f2f2f2" opacity=".14"/></g>'
b += f'<circle cx="{cx}" cy="{cy}" r="300" fill="#f2f2f2"/><circle class="glow" cx="{cx}" cy="{cy}" r="300" fill="none" stroke="#ff2e4d" stroke-width="10"/>'
b += '<path d="M700,1000 L700,820 L1600,780 L1600,1000Z" fill="#000"/><rect x="760" y="760" width="22" height="70" fill="#000"/><rect x="740" y="752" width="62" height="14" fill="#000"/>'
b += '''<g transform="translate(1110 800)" fill="#000">
<path d="M-38,-270 L-60,-320 L-30,-300 L-40,-350 L-5,-318 L5,-372 L28,-322 L62,-352 L50,-300 L84,-306 L56,-262 Q60,-210 20,-200 L-20,-200 Q-55,-215 -38,-270Z"/>
<path d="M-30,-205 L30,-205 L36,-185 L-36,-185Z"/>
<path d="M-60,-185 L60,-185 L80,-60 L95,0 L45,0 L28,-80 L12,0 L-12,0 L-28,-80 L-45,0 L-95,0 L-80,-60Z"/>
<path d="M-60,-180 L-95,-60 L-80,-55 L-48,-150Z M60,-180 L95,-60 L80,-55 L48,-150Z"/>
</g><g class="scarf">
<path d="M1125,610 Q1000,590 900,640 Q820,670 760,650 Q840,610 900,600 Q980,575 1130,595Z" fill="#ff2e4d"/>
<path d="M1120,622 Q1030,640 960,690 Q930,710 890,712 Q950,680 990,650 Q1040,620 1118,608Z" fill="#b3102b"/></g>'''
b += '<g class="don"><text x="140" y="300" font-family="Yu Gothic,Meiryo,sans-serif" font-weight="900" font-size="190" fill="none" stroke="#f2f2f2" stroke-width="5" transform="rotate(-10 140 300)">ドン!!</text></g>'
save('manga.svg', svg(b, '', css))

# ---------------- SLIME ----------------
d = '''<radialGradient id="g" cx=".5" cy=".35" r=".7"><stop offset="0" stop-color="#dfff7a"/><stop offset=".7" stop-color="#9be000"/><stop offset="1" stop-color="#5a9a00"/></radialGradient>
<linearGradient id="bg" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#120b1f"/><stop offset="1" stop-color="#2a1150"/></linearGradient>'''
css = '.sq{transform-box:fill-box;transform-origin:50% 100%;animation:sq 1.5s ease-in-out infinite alternate}@keyframes sq{from{transform:scale(1.06,.93)}to{transform:scale(.96,1.05)}}.bu{animation:bu 7s linear infinite}@keyframes bu{0%{transform:translateY(0);opacity:0}10%{opacity:.6}100%{transform:translateY(-420px);opacity:0}}.eyes{transform-box:fill-box;transform-origin:center;animation:blink 5s infinite}@keyframes blink{0%,94%,100%{transform:scaleY(1)}97%{transform:scaleY(.1)}}'
b = '<rect width="1600" height="1000" fill="url(#bg)"/>'
drip = 'M0,0 L1600,0 L1600,40 '
x = 1600
while x > 0:
    w = random.randint(60, 140); dpt = random.randint(30, 150)
    drip += f'Q{x - w * .25:.0f},{40} {x - w * .5:.0f},{40 + dpt} Q{x - w * .75:.0f},{40} {x - w:.0f},40 '
    x -= w
drip += 'L0,40Z'
b += f'<path d="{drip}" fill="#9d4dff" opacity=".85"/>'
for _ in range(26):
    r = random.randint(6, 30)
    b += f'<circle class="bu" style="animation-delay:-{random.uniform(0, 7):.1f}s" cx="{random.randint(0, 1600)}" cy="{random.randint(500, 1000)}" r="{r}" fill="none" stroke="#00ffc8" stroke-width="3"/>'
def slime(x, y, s, c='url(#g)', dly=0):
    return f'''<g transform="translate({x} {y}) scale({s})"><g class="sq" style="animation-delay:-{dly}s"><path d="M-170,0 Q-180,-150 -60,-200 Q0,-260 60,-200 Q180,-150 170,0 Q175,40 120,40 L-120,40 Q-175,40 -170,0Z" fill="{c}" stroke="#2b4a00" stroke-width="8"/>
<ellipse cx="-70" cy="-150" rx="34" ry="18" fill="#fff" opacity=".6" transform="rotate(-30 -70 -150)"/>
<g class="eyes"><ellipse cx="-55" cy="-70" rx="24" ry="36" fill="#1a1030"/><ellipse cx="55" cy="-70" rx="24" ry="36" fill="#1a1030"/><circle cx="-46" cy="-86" r="9" fill="#fff"/><circle cx="64" cy="-86" r="9" fill="#fff"/></g>
<path d="M-26,-20 Q0,10 26,-20" fill="none" stroke="#1a1030" stroke-width="8" stroke-linecap="round"/><ellipse cx="-100" cy="-30" rx="22" ry="12" fill="#ff7ad6" opacity=".6"/><ellipse cx="100" cy="-30" rx="22" ry="12" fill="#ff7ad6" opacity=".6"/></g></g>'''
b += '<ellipse cx="1300" cy="930" rx="330" ry="40" fill="#000" opacity=".35"/>' + slime(1300, 880, 1.5) + slime(980, 930, .55, '#00ffc8', .5) + slime(1560, 940, .45, '#c77dff', 1)
save('slime.svg', svg(b, d, css))

# ---------------- PIXEL ----------------
P = 20
def px(grid, ox, oy, cmap, s=P):
    out = ''
    for j, row in enumerate(grid):
        for i, ch in enumerate(row):
            if ch in cmap: out += f'<rect x="{ox + i * s}" y="{oy + j * s}" width="{s}" height="{s}" fill="{cmap[ch]}"/>'
    return out
css = '.inv{animation:inv 2.4s steps(4) infinite alternate}@keyframes inv{to{transform:translateX(120px)}}.inv2{animation:inv 1.8s steps(3) infinite alternate-reverse}.gh{animation:gh 1.6s steps(2) infinite alternate}@keyframes gh{to{transform:translateY(-24px)}}.pt{animation:tw 2s steps(2) infinite}.hb{transform-box:fill-box;transform-origin:center;animation:hb 1s steps(2) infinite}@keyframes hb{50%{transform:scale(1.15)}}'
b = '<rect width="1600" height="1000" fill="#05060a"/>'
for _ in range(80):
    b += f'<rect class="pt"{dl()} x="{random.randint(0, 79) * 20}" y="{random.randint(0, 30) * 20}" width="{random.choice([6, 10])}" height="{random.choice([6, 10])}" fill="#c4ffd2"/>'
moon = ['..XXXX..', '.XXXXXX.', 'XXXXXXXX', 'XXXXX.XX', 'XXXXXXXX', 'XX.XXXXX', '.XXXXXX.', '..XXXX..']
b += px(moon, 1240, 120, {'X': '#fff6c2'}, 22)
for (base, col, amp, off) in [(700, '#1b2a4a', 260, 0), (800, '#16203a', 200, 300)]:
    # one path per mountain range instead of 80 rects
    d2 = f'M0,1000'
    for i in range(0, 81):
        h = int((amp * (0.5 + 0.5 * math.sin(i / 7 + off))) / 20) * 20
        d2 += f'V{base - h}h20'
    b += f'<path d="{d2}V1000Z" fill="{col}"/>'
b += '<rect x="0" y="880" width="1600" height="120" fill="#0d3b1e"/>' + ''.join(f'<rect x="{i * 20}" y="{860 if i % 3 else 870}" width="20" height="20" fill="#3dff6e"/>' for i in range(80))
inv = ['..X.....X..', '...X...X...', '..XXXXXXX..', '.XX.XXX.XX.', 'XXXXXXXXXXX', 'X.XXXXXXX.X', 'X.X.....X.X', '...XX.XX...']
b += '<g class="inv">' + px(inv, 300, 200, {'X': '#ff3df2'}, 18) + '</g><g class="inv2">' + px(inv, 620, 340, {'X': '#3dc8ff'}, 12) + '</g>'
ghost = ['..XXXX..', '.XXXXXX.', 'XX..X..X', 'XX..X..X', 'XXXXXXXX', 'XXXXXXXX', 'XX.XX.XX', 'X...X..X']
b += '<g class="gh">' + px(ghost, 1120, 700, {'X': '#3dff6e'}, 22) + '</g>'
heart = ['.XX.XX.', 'XXXXXXX', 'XXXXXXX', '.XXXXX.', '..XXX..', '...X...']
b += '<g class="hb">' + px(heart, 140, 560, {'X': '#ff3d6b'}, 14) + '</g>'
save('pixel.svg', svg(b, '', css, ' shape-rendering="crispEdges"'))
print('ok')
