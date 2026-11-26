const { chromium } = require('playwright');
const fs = require('fs');
const path = require('path');

const base = process.env.SITE_URL || 'http://127.0.0.1:8765';
const outDir = process.env.SHOT_DIR || '/tmp/ufi-site-shots';
fs.mkdirSync(outDir, { recursive: true });

function shot(page, name) {
  return page.screenshot({ path: path.join(outDir, name), fullPage: false });
}

(async () => {
  const errors = [];
  const browser = await chromium.launch({
    executablePath: process.env.CHROME_PATH || '/usr/bin/google-chrome-stable',
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage']
  });
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' });
  page.on('pageerror', err => errors.push(String(err)));
  page.on('console', msg => {
    if (msg.type() === 'error') errors.push(msg.text());
  });

  await page.goto(base + '/', { waitUntil: 'domcontentloaded' });
  await page.waitForFunction(() => {
    const star = document.getElementById('star-count');
    return star && star.textContent && star.textContent !== '…';
  }, null, { timeout: 20000 });
  const star = (await page.locator('#star-count').innerText()).trim();
  const starPlain = star.replace(/[,\s]/g, '');
  if (!/^\d+$/.test(starPlain) && star !== 'GitHub') throw new Error('unexpected star text: ' + star);
  await page.waitForFunction(() => {
    const actions = document.getElementById('download-actions');
    const fallback = document.getElementById('download-fallback');
    const hasAsset = actions && /arm64\.js/.test(actions.innerHTML);
    const fallbackVisible = fallback && !fallback.hidden;
    return hasAsset || fallbackVisible;
  }, null, { timeout: 20000 });
  const asset = page.locator('#download-actions a[href*="arm64.js"]');
  if (await asset.count()) {
    const href = await asset.first().getAttribute('href');
    if (!href.includes('github.com/nanbeiluduo/UFI-Gate/releases/download/')) throw new Error('bad asset href ' + href);
  }
  await page.waitForFunction(() => document.querySelector('#stat-stars .stat-value').textContent !== '…');
  await shot(page, 'homepage.png');
  await page.locator('#download').scrollIntoViewIfNeeded();
  await page.waitForTimeout(300);
  await shot(page, 'download.png');
  await page.locator('#stats').scrollIntoViewIfNeeded();
  await page.waitForTimeout(400);
  await shot(page, 'stats.png');
  await page.screenshot({ path: path.join(outDir, 'homepage-full.png'), fullPage: true });

  const mobile = await browser.newPage({ viewport: { width: 390, height: 844 }, locale: 'zh-CN' });
  await mobile.goto(base + '/', { waitUntil: 'domcontentloaded' });
  await mobile.waitForTimeout(500);
  await mobile.screenshot({ path: path.join(outDir, 'homepage-mobile.png'), fullPage: false });
  await mobile.close();

  await page.goto(base + '/demo.html', { waitUntil: 'domcontentloaded' });
  await page.waitForSelector('iframe[title="手机访客认证页"]');
  const phone = page.frame({ url: /portal\.html\?client=phone/ });
  const admin = page.frame({ url: /admin\.html/ });
  if (!phone || !admin) throw new Error('frames missing');
  phone.on('pageerror', err => errors.push('phone ' + err));
  admin.on('pageerror', err => errors.push('admin ' + err));
  await phone.waitForSelector('#connect');
  await shot(page, 'demo-phone.png');

  await page.locator('#connect-btn').click();
  await phone.waitForSelector('text=连接，从这里开始');
  await phone.locator('[data-method="approval"]').click();
  await phone.locator('#name').fill('路人甲');
  await phone.locator('#reason').fill('临时查资料');
  await phone.locator('#submit').click();
  await phone.waitForSelector('text=正在等待管理员');

  await page.locator('[data-frame-btn="admin"]').click();
  await admin.evaluate(() => { window.UFI_CONFIRM_MS = 200; });
  await admin.locator('#user').fill('admin');
  await admin.locator('#pass').fill('admin123');
  await admin.locator('#loginbtn').click();
  await admin.waitForSelector('text=网络概览');
  await admin.locator('[data-nav="approvals"]').click();
  await admin.waitForSelector('text=路人甲');
  await shot(page, 'demo-admin.png');
  await admin.locator('[data-action="allow-request"]').click();
  await admin.locator('#decisionReason').fill('可以上网');
  await admin.locator('#dialogForm button.primary').click();
  await admin.locator('#confirm-accept').click({ timeout: 5000 });
  await page.locator('[data-frame-btn="phone"]').click();
  await phone.waitForSelector('text=连接成功', { timeout: 8000 });
  await page.locator('#sim-btn').click();
  await phone.waitForFunction(() => {
    const text = document.body.innerText;
    return /今日已用/.test(text) && !/今日已用 0 B/.test(text);
  }, null, { timeout: 8000 });
  await shot(page, 'demo-phone-online.png');

  await page.locator('[data-frame-btn="tablet"]').click();
  await page.waitForTimeout(300);
  await shot(page, 'demo-tablet.png');
  await page.locator('[data-frame-btn="pc"]').click();
  await page.waitForTimeout(300);
  await shot(page, 'demo-pc.png');
  await page.locator('[data-frame-btn="split"]').click();
  await page.waitForTimeout(400);
  await shot(page, 'demo-split.png');

  const tablet = page.frame({ url: /client=tablet/ });
  await page.locator('[data-frame-btn="tablet"]').click();
  await page.locator('#target').selectOption('tablet');
  await page.locator('#connect-btn').click();
  await tablet.waitForSelector('#username', { timeout: 5000 });
  await tablet.locator('[data-method="account"]').click();
  await tablet.locator('#username').fill('guest');
  await tablet.locator('#password').fill('wifi2026');
  await tablet.locator('#submit').click();
  await tablet.waitForSelector('text=连接成功');

  await page.locator('#white-btn').click();
  await tablet.waitForSelector('text=已放行该地址');
  await page.locator('#black-btn').click();
  await tablet.waitForSelector('text=此网站暂不可访问');

  await page.locator('[data-frame-btn="admin"]').click();
  await admin.locator('[data-nav="appearance"]').click();
  await admin.locator('#title').fill('欢迎来到演示网络');
  await admin.locator('[data-action="save-settings"]').click();
  await admin.waitForSelector('text=设置已保存');
  await page.evaluate(() => {
    UFIDemo.update(s => {
      const device = UFIDemo.findDevice(s, s.clients.tablet.mac);
      device.until = 0;
      device.trusted = false;
      UFIDemo.browse(s, 'tablet', 'normal');
    });
  });
  await page.locator('[data-frame-btn="tablet"]').click();
  await tablet.waitForSelector('text=欢迎来到演示网络');

  const pc = page.frame({ url: /client=pc/ });
  await page.locator('[data-frame-btn="pc"]').click();
  await page.locator('#target').selectOption('pc');
  await page.locator('#connect-btn').click();
  await pc.locator('[data-method="captcha"]').click();
  await pc.waitForSelector('#answer');
  const code = await pc.evaluate(() => JSON.parse(localStorage.getItem('ufi-gate-demo-v1')).clients.pc.captcha.code);
  await pc.locator('#answer').fill(code);
  await pc.locator('#submit').click();
  await pc.waitForSelector('text=连接成功');

  if (errors.length) {
    console.error(errors.join('\n'));
    throw new Error('page errors: ' + errors.length);
  }
  console.log('site demo ok', star);
  await browser.close();
})().catch(err => {
  console.error(err);
  process.exit(1);
});
