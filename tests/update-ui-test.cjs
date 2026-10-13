// DEMO_BINARY=/path/to/demo CHROME_PATH=/path/to/browser node tests/update-ui-test.cjs
const {chromium}=require('playwright');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..');fs.mkdirSync(path.join(root,'dist'),{recursive:true});
const state=fs.mkdtempSync(path.join(root,'dist','update-ui-'));
const daemon=spawn(process.env.DEMO_BINARY||path.join(root,'ufi-gate-demo'),['--demo','--dir',state],{stdio:['ignore','pipe','pipe']});
let logs='';daemon.stderr.on('data',b=>logs+=b);const sleep=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{let browser;try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:8081/admin-api/ready')).ok)break}catch{}if(i===99)throw Error(logs);await sleep(100)}
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||undefined});
 const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('http://127.0.0.1:8081/#'+fs.readFileSync(path.join(state,'setup.token'),'utf8'));
 assert.equal(await page.locator('#message-float').count(),0);
 await page.locator('#user').fill('admin');await page.locator('#pass').fill('AdminPass123');await page.locator('#loginbtn').click();await page.locator('#message-float').waitFor();
 for(const section of ['dashboard','devices','approvals','accounts','auth','policy','appearance','push','system','updates','logs']){await page.locator('[data-nav='+section+']').click();assert(await page.locator('#message-float').isVisible())}
 await page.locator('#message-float').click();await page.locator('.message-item').first().waitFor();await page.locator('#message-read-all').click();await page.waitForFunction(()=>document.querySelector('#message-count').textContent==='0');await page.locator('#message-close').click();
 await page.reload();await page.locator('#message-float').waitFor();assert.equal(await page.locator('#message-count').innerText(),'0');
 await page.locator('[data-nav=push]').click();await page.locator('#notification-settings').waitFor();await page.locator('[data-push-kind=update_available]').check();await page.locator('[data-push-kind=quota]').uncheck();await page.locator('#notification-save').click();await page.waitForFunction(()=>document.querySelector('#toast').textContent==='通知推送范围已保存');
 const prefs=await page.evaluate(()=>fetch('/admin-api/notifications').then(r=>r.json()));assert(prefs.push.update_available);assert(!prefs.push.quota);
 await page.locator('[data-nav=updates]').click();await page.locator('#update-panel').waitFor();await page.locator('#update-auto').check();await page.locator('#update-save').click();await page.waitForFunction(()=>document.querySelector('#toast').textContent==='检查设置已保存');
 const settings=await page.evaluate(()=>fetch('/admin-api/update').then(r=>r.json()));assert(settings.autoCheck);
 // Avoid external network access from the demo after verifying the setting.
 await page.evaluate(()=>fetch('/admin-api/update/settings',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({AutoCheck:false})}));
 let installs=0;
 await page.route('**/admin-api/update',route=>route.fulfill({json:{current:'1.0.10',arch:'arm64',signatureReady:true,release:{version:'1.0.11',notes:'<img src=x onerror=alert(1)>',url:'https://github.com/nanbeiluduo/UFI-Gate/releases/tag/v1.0.11',manifest:{}},status:{}}}));
 await page.route('**/admin-api/update/install',route=>{installs++;return route.fulfill({json:{ok:true}})});
 await page.locator('[data-nav=updates]').click();await page.locator('#update-install').waitFor();await page.waitForFunction(()=>!document.querySelector('#update-install').disabled);assert.equal(await page.locator('#update-notes img').count(),0);
 page.once('dialog',d=>d.dismiss());await page.locator('#update-install').click();assert.equal(installs,0);
 page.once('dialog',d=>d.accept());await page.locator('#update-install').click();await page.waitForFunction(()=>document.querySelector('#toast').textContent.includes('更新任务已提交'));assert.equal(installs,1);
 for(const width of [390,834,1440]){await page.setViewportSize({width,height:950});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'update overflow '+width);await page.locator('#message-float').click();await page.locator('#message-box').waitFor({state:'visible'});await page.locator('#message-close').click()}
 await page.screenshot({path:path.join(root,'dist','update-page.png'),fullPage:true});
 await page.locator('#logout').click();await page.locator('#login').waitFor();assert.equal(await page.locator('#message-float').count(),0);assert.deepEqual(errors,[]);
 console.log('Update UI passed: all pages, unread persistence, push preferences, update settings, confirmation, escaped notes, 3 viewports.');
 }finally{if(browser)await browser.close();daemon.kill()}})().catch(e=>{console.error(e);process.exitCode=1});
