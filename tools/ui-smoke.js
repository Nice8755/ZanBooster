// UI smoke test before every release: open ui.html (built-in mock) in Thai and English, wait for boot,
// open every page and the main dialogs. Fails on a page error, a hang, or a splash that never goes away.
// usage: node tools/ui-smoke.js <path-to-ui.html> [i18n/en.json]   (needs playwright or playwright-core; uses Edge on Windows)
const path = require('path'), fs = require('fs');
let pw; try { pw = require('playwright'); } catch (e) { pw = require('playwright-core'); }
const ui = path.resolve(process.argv[2] || 'recovered/ui.html'), enFile = process.argv[3] && path.resolve(process.argv[3]);
const en = enFile && fs.existsSync(enFile) ? fs.readFileSync(enFile, 'utf8') : '{}';
const race = (p, ms, what) => Promise.race([p, new Promise((_, j) => setTimeout(() => j(new Error('HUNG: ' + what)), ms))]);
setTimeout(() => { console.log('  ui: FAILED (took too long, page hung)'); process.exit(1); }, 150000);
(async () => {
  const opt = process.platform === 'win32' ? { channel: 'msedge' } : {};
  const b = await pw.chromium.launch(opt); let bad = 0;
  for (const lang of ['th', 'en']) {
    const ctx = await b.newContext(lang === 'en' ? { locale: 'en-US', timezoneId: 'America/New_York', viewport: { width: 1247, height: 811 } } : { locale: 'th-TH', timezoneId: 'Asia/Bangkok', viewport: { width: 1247, height: 811 } });
    const p = await ctx.newPage(); const errs = [];
    p.on('pageerror', e => errs.push(e.message));
    await p.route(u => !u.href.startsWith('file:'), r => r.request().url().includes('en.json') ? r.fulfill({ status: 200, contentType: 'application/json', body: en }) : r.fulfill({ status: 200, contentType: 'text/html', body: '' }));
    try {
      await p.goto('file:///' + ui.replace(/\\/g, '/'), { waitUntil: 'commit' });
      await p.waitForFunction(() => !document.getElementById('splash') && typeof booted !== 'undefined' && booted, null, { timeout: 25000 });
      const pages = await race(p.evaluate(() => [...document.querySelectorAll('.page')].map(x => x.id.replace(/^p-/, ''))), 5000, 'list pages');
      for (const pg of pages) {
        await race(p.evaluate(pg => { document.querySelectorAll('.ov,#gov').forEach(x => x.classList.remove('show')); go(pg); }, pg), 5000, lang + ' page ' + pg);
        await p.waitForTimeout(400);
      }
      for (const sel of ['#bTheme', '#bMus']) { await race(p.evaluate(s => document.querySelector(s)?.click(), sel), 5000, lang + ' ' + sel); await p.waitForTimeout(400); }
      await race(p.evaluate(() => 1), 5000, lang + ' final');
      console.log(`  ui ${lang}: ${pages.length} pages ok${errs.length ? ', errors: ' + errs.slice(0, 3).join(' | ') : ''}`);
      if (errs.length) bad++;
    } catch (e) { console.log(`  ui ${lang}: FAILED ${e.message.split('\n')[0]}`); bad++; }
    await race(ctx.close(), 5000, 'close').catch(() => { });
  }
  await race(b.close(), 5000, 'close').catch(() => { }); process.exit(bad ? 1 : 0);
})();
