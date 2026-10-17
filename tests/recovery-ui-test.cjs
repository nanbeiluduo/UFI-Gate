// Demo-only end-to-end administrator recovery and download checks.
const {chromium}=require('playwright');
const {saveRecovery}=require('./recovery-helper.cjs');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict'),{spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..');fs.mkdirSync(path.join(root,'dist'),{recursive:true});
const state=fs.mkdtempSync(path.join(root,'dist','recovery-ui-'));
const daemon=spawn(process.env.DEMO_BINARY||path.join(root,'ufi-gate-demo'),['--demo','--dir',state],{stdio:['ignore','pipe','pipe']});
let logs='';daemon.stderr.on('data',b=>logs+=b);const sleep=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{let browser;try {
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:8081/admin-api/ready')).ok)break}catch{}if(i===99)throw Error(logs);await sleep(100)}
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||undefined});
 const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('http://127.0.0.1:8081/#'+fs.readFileSync(path.join(state,'setup.token'),'utf8'));
 await page.locator('#loginbtn').waitFor();
 assert.equal(await page.locator('#admin-forgot-password').count(),0,'setup must use the initialization credential');
 await page.locator('#user').fill('admin');await page.locator('#pass').fill('AdminPass123');await page.locator('#loginbtn').click();
 const oldKey=await saveRecovery(page);await page.locator('#view').waitFor();
 await page.goto('http://127.0.0.1:8090/');await page.locator('#admin-login').click();
 for(const width of [390,834,1440]) {await page.setViewportSize({width,height:950});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'recovery page overflow '+width)}
 await page.locator('#admin-user').fill('admin');await page.locator('#admin-password').fill('wrong');await page.locator('#admin-access-submit').click();await page.locator('#admin-access-error').filter({hasText:'无效'}).waitFor();
 await page.locator('#admin-password').fill('AdminPass123');await page.locator('#admin-access-submit').click();await page.locator('#view').waitFor();
 await page.locator('#logout').click();await page.locator('#admin-forgot-password').waitFor();
 assert.equal(await page.locator('#admin-forgot-password').getAttribute('href'),'http://127.0.0.1:8090/admin-access?mode=recovery');
 await page.locator('#admin-forgot-password').click();
 await page.locator('#admin-recovery-key').waitFor({state:'visible'});
 assert(await page.locator('#forgot-password').isChecked(),'backend forgot-password link must select recovery mode');
 assert(await page.locator('#admin-user').isDisabled());
 await page.locator('#admin-recovery-key').fill(oldKey);await page.locator('#admin-new-password').fill('NewAdmin123');await page.locator('#admin-confirm-password').fill('Different123');await page.locator('#admin-access-submit').click();await page.getByText('两次新密码不一致').waitFor();
 await page.locator('#admin-confirm-password').fill('NewAdmin123');await page.locator('#admin-access-submit').click();await page.locator('#recovery-download').waitFor();
 for(const width of [390,834,1440]) {await page.setViewportSize({width,height:950});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'download page overflow '+width)}
 await page.screenshot({path:path.join(root,'dist','recovery-work','download-page.png'),fullPage:true});
 const newKey=await saveRecovery(page);assert.notEqual(oldKey,newKey);await page.locator('#view').waitFor();
 await page.goto('http://127.0.0.1:8090/admin-access');await page.locator('#forgot-password').check();await page.locator('#admin-recovery-key').fill(oldKey);await page.locator('#admin-new-password').fill('Another123');await page.locator('#admin-confirm-password').fill('Another123');await page.locator('#admin-access-submit').click();await page.locator('#admin-access-error').filter({hasText:'无效'}).waitFor();
 await page.locator('#forgot-password').uncheck();await page.locator('#admin-user').fill('admin');await page.locator('#admin-password').fill('AdminPass123');await page.locator('#admin-access-submit').click();await page.locator('#admin-access-error').filter({hasText:'无效'}).waitFor();
 await page.locator('#admin-password').fill('NewAdmin123');await page.locator('#admin-access-submit').click();await page.locator('#view').waitFor();
 assert.deepEqual(errors,[]);console.log('Recovery UI passed: mandatory TXT download, portal login, password confirmation, key rotation, old key/password rejection, 3 widths.');
 } finally {if(browser)await browser.close();daemon.kill();}
})().catch(e=>{console.error(e);process.exitCode=1});
