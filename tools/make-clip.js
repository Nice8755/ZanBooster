// คลิปสั้นโปรโมต (TikTok / ดิส): อัดหน้าจอตัวทดลองในเว็บ (recovered/ui.html#demo) พร้อมคำบรรยาย
// ไม่โชว์ตัวเลข FPS จากตัวจำลอง (ตัวเลข FPS ต้องมาจากการวัดจริงเท่านั้น)
// ใช้: node tools/make-clip.js <โฟลเดอร์ผลลัพธ์> [tall|wide]   ต้องมี playwright + ffmpeg
const http = require('http'), fs = require('fs'), path = require('path'), { execFileSync } = require('child_process');
const { chromium } = require('playwright');
const ROOT = path.resolve(__dirname, '..'), OUT = path.resolve(process.argv[2] || '.'), MODE = process.argv[3] || 'tall';
const TALL = MODE === 'tall', W = TALL ? 1080 : 1920, H = TALL ? 1920 : 1080;

const SCENES = [
  { cap: 'เล่น FiveM แล้วกระตุก?', sub: 'Zan Booster ปรับคอมให้เกมลื่นขึ้น ใช้ฟรี', page: 'dash', fx: 600, fy: 420, ms: 3200 },
  { cap: 'ตรวจเครื่องให้ก่อน', sub: 'บอกเลยว่าอะไรทำให้เสีย FPS', page: 'health', act: 'health', fx: 620, fy: 400, ms: 3800 },
  { cap: 'บูสต์ในคลิกเดียว', sub: 'เก็บค่าเดิมไว้ทุกข้อ กดคืนค่าได้', page: 'boost', fx: 470, fy: 400, ms: 3400 },
  { cap: 'ภาพสวยขึ้นด้วยพรีเซ็ต Zan', sub: 'ตัวอย่างจำลองสีบนภาพเกม', shot: ['site/zp/orig-0.jpg', 'site/zp/z3-0.jpg'], ms: 4200 },
  { cap: 'แต่งธีมได้ตามใจ', sub: 'มีทั้งธีมเท่ ธีมน่ารัก', page: 'dash', act: 'themes', fx: 900, fy: 400, ms: 4400 },
  { cap: 'เข้าเซิร์ฟคลิกเดียว', sub: 'ตั้ง build · Pure mode ให้ตรงเซิร์ฟก่อนเปิดเกม', page: 'dash', act: 'scroll', fx: 600, fy: 420, ms: 3400 },
  { end: 1, ms: 3400 },
];

const wrap = `<!doctype html><html><head><meta charset="utf-8"><style>
*{box-sizing:border-box}html,body{margin:0;width:${W}px;height:${H}px;overflow:hidden;background:#05060d;font-family:"Leelawadee UI",Loma,"Segoe UI",sans-serif;color:#fff}
.bg{position:absolute;inset:0;background:radial-gradient(900px 700px at 15% 10%,rgba(0,229,255,.22),transparent 70%),radial-gradient(900px 700px at 90% 90%,rgba(255,43,214,.2),transparent 70%)}
.cap{position:absolute;left:0;right:0;top:${TALL ? 150 : 40}px;text-align:center;padding:0 60px;transition:opacity .35s,transform .35s}
.cap b{display:block;font-size:${TALL ? 82 : 64}px;font-weight:800;line-height:1.25;text-shadow:0 0 30px rgba(0,229,255,.55)}
.cap small{display:block;font-size:${TALL ? 40 : 30}px;color:#b9c3e0;margin-top:14px}
.cap.h{opacity:0;transform:translateY(20px)}
.stage{position:absolute;left:${TALL ? 40 : 384}px;top:${TALL ? 470 : 200}px;width:${TALL ? 1000 : 1152}px;height:${TALL ? 1180 : 720}px;border-radius:22px;overflow:hidden;box-shadow:0 0 0 2px rgba(0,229,255,.5),0 30px 90px rgba(0,0,0,.7),0 0 80px rgba(0,229,255,.25);transition:opacity .4s}
iframe{position:absolute;left:0;top:0;width:1280px;height:800px;border:0;display:block;transform-origin:0 0;transition:transform 1.1s cubic-bezier(.4,0,.2,1)}
.shot{position:absolute;inset:0;opacity:0;transition:opacity .4s}.shot.on{opacity:1}.shot img{position:absolute;inset:0;width:100%;height:100%;object-fit:cover}
.shot .a{clip-path:inset(0 0 0 50%)}.shot i{position:absolute;top:0;bottom:0;left:50%;width:4px;background:#fff;box-shadow:0 0 20px #00e5ff}
.shot em{position:absolute;bottom:22px;font-style:normal;font-weight:700;font-size:28px;background:rgba(0,0,0,.6);padding:6px 16px;border-radius:20px}
.foot{position:absolute;left:0;right:0;bottom:${TALL ? 120 : 34}px;text-align:center;font-size:${TALL ? 36 : 28}px;color:#cfd6ea}
.foot b{color:#00e5ff}
.end{position:absolute;inset:0;display:grid;place-items:center;opacity:0;transition:opacity .5s;text-align:center;background:#05060d}.end.on{opacity:1}
.end img{width:${TALL ? 260 : 200}px}.end h1{font-size:${TALL ? 96 : 80}px;margin:20px 0 6px;letter-spacing:4px}.end h1 span{color:#00e5ff}
.end p{font-size:${TALL ? 44 : 34}px;color:#cfd6ea;margin:8px 0}.end .d{display:inline-block;margin-top:26px;padding:18px 40px;border-radius:999px;background:linear-gradient(90deg,#00e5ff,#8b5cff,#ff2bd6);color:#05060d;font-weight:800;font-size:${TALL ? 44 : 34}px}
</style></head><body><div class="bg"></div>
<div class="cap h" id="cap"><b></b><small></small></div>
<div class="stage" id="stage"><iframe id="f" src="/recovered/ui.html#demo"></iframe><div class="shot" id="shot"><img class="o"><img class="a"><i></i><em style="left:22px">ปกติ</em><em style="right:22px">พรีเซ็ต Zan</em></div></div>
<div class="foot">Zan Booster · <b>ใช้ฟรี</b> · โปรแกรมไม่แตะไฟล์เกม</div>
<div class="end" id="end"><div><img src="/site/logo.png"><h1>ZAN <span>BOOSTER</span></h1><p>ปรับคอมให้เล่น FiveM ลื่นขึ้น</p><p>ใช้ฟรี · คืนค่าได้ทุกข้อ</p><div class="d">รับโปรแกรมในดิส ZAN HUB</div><p style="font-size:${TALL ? 32 : 26}px;margin-top:22px;color:#9aa4c4">discord.gg/pCCqStQwmp</p></div></div>
</body></html>`;

const srv = http.createServer((q, r) => {
  const u = decodeURIComponent(q.url.split(/[?#]/)[0]);
  if (u === '/__clip') { r.writeHead(200, { 'content-type': 'text/html; charset=utf-8' }); return r.end(wrap); }
  const f = path.join(ROOT, path.normalize(u));
  if (!f.startsWith(ROOT) || !fs.existsSync(f) || fs.statSync(f).isDirectory()) { r.writeHead(404); return r.end(); }
  const ty = { '.html': 'text/html; charset=utf-8', '.json': 'application/json', '.png': 'image/png', '.jpg': 'image/jpeg', '.js': 'text/javascript' }[path.extname(f)] || 'application/octet-stream';
  r.writeHead(200, { 'content-type': ty }); fs.createReadStream(f).pipe(r);
});

(async () => {
  await new Promise(r => srv.listen(0, '127.0.0.1', r));
  const base = `http://127.0.0.1:${srv.address().port}`;
  fs.mkdirSync(OUT, { recursive: true });
  const vdir = path.join(OUT, 'raw-' + MODE); fs.rmSync(vdir, { recursive: true, force: true });
  const b = await chromium.launch();
  const ctx = await b.newContext({ viewport: { width: W, height: H }, recordVideo: { dir: vdir, size: { width: W, height: H } }, locale: 'th-TH' });
  const p = await ctx.newPage(); const tv = Date.now();
  await p.goto(base + '/__clip');
  const f = p.frame({ url: /ui\.html/ });
  await f.waitForFunction(() => window.__settingsLoaded && !document.getElementById('splash'), null, { timeout: 30000 }).catch(() => { });
  await p.waitForTimeout(3500);
  // ไม่ให้ตัวเลข FPS จากตัวจำลองขึ้นในคลิป + ปิดหน้าต่างที่เด้งเอง
  await f.evaluate(() => {
    const st = document.createElement('style'); st.textContent = '#hFps,.hfps,#ob,#toasts,.toast,#mus,.rdyb,[class*="nudge"]{display:none!important}'; document.head.appendChild(st);
    document.querySelectorAll('.ov.show').forEach(o => o.classList.remove('show'));
    try { if (window.musStop) musStop(); } catch (e) { }
  });
  const t0 = Date.now(); const marks = [];
  for (const s of SCENES) {
    marks.push(Date.now() - t0);
    await p.evaluate(s => {
      const c = document.getElementById('cap'); c.classList.add('h');
      setTimeout(() => { if (!s.end) { c.querySelector('b').textContent = s.cap; c.querySelector('small').textContent = s.sub; c.classList.remove('h'); } }, 300);
      const sh = document.getElementById('shot'); sh.classList.toggle('on', !!s.shot);
      if (s.shot) { sh.querySelector('.o').src = '/' + s.shot[0]; sh.querySelector('.a').src = '/' + s.shot[1]; }
      document.getElementById('end').classList.toggle('on', !!s.end);
      const fr = document.getElementById('f'), st = document.getElementById('stage'), bw = st.clientWidth, bh = st.clientHeight;
      if (bw === 1152) fr.style.transform = 'scale(.9)';
      else if (s.fx) { const k = bh / 800, tx = Math.min(0, Math.max(bw - 1280 * k, bw / 2 - s.fx * k)), ty = Math.min(0, Math.max(bh - 800 * k, bh / 2 - s.fy * k)); fr.style.transform = `translate(${tx}px,${ty}px) scale(${k})`; }
    }, s);
    if (s.page) await f.evaluate(pg => { document.querySelectorAll('.ov.show').forEach(o => o.classList.remove('show')); go(pg); }, s.page);
    if (s.act === 'health') f.evaluate(() => { try { runHealth(); } catch (e) { } });
    if (s.act === 'scroll') f.evaluate(() => { const l = document.getElementById('slist'); l && l.scrollTo({ top: 260, behavior: 'smooth' }); });
    if (s.act === 'themes') { for (const t of ['kawaii', 'cyber', 'inferno', 'sakura', 'neon']) { await f.evaluate(t => { try { themeApply(t); } catch (e) { } }, t); await p.waitForTimeout(s.ms / 5); } continue; }
    if (s.shot) { // เลื่อนเส้นแบ่งก่อน/หลัง
      for (let i = 0; i <= 40; i++) { await p.evaluate(x => { document.querySelector('#shot .a').style.clipPath = `inset(0 0 0 ${x}%)`; document.querySelector('#shot i').style.left = x + '%'; }, 90 - i * 1.6); await p.waitForTimeout(s.ms / 44); }
      continue;
    }
    await p.waitForTimeout(s.ms);
  }
  const vid = await p.video().path(); await ctx.close(); await b.close(); srv.close();
  const out = path.join(OUT, `zan-clip-${MODE}.mp4`);
  // ตัดช่วงโหลดตอนต้นออก (เริ่มที่ฉากแรก)
  const ss = Math.max(0, (t0 - tv) / 1000 + 0.2);
  execFileSync('ffmpeg', ['-y', '-loglevel', 'error', '-ss', String(ss), '-i', vid, '-vf', 'fps=30,format=yuv420p', '-c:v', 'libx264', '-preset', 'medium', '-crf', '20', '-movflags', '+faststart', out]);
  console.log('clip', out, 'scene marks(ms)', marks.join(','));
})().catch(e => { console.error(e); process.exit(1); });
