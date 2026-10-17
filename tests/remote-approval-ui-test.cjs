const {saveRecovery}=require('./recovery-helper.cjs');
const {chromium}=require('playwright');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..');
const state=fs.mkdtempSync(path.join(root,'dist','remote-approval-ui-'));
const daemon=spawn(process.env.DEMO_BINARY||path.join(root,'ufi-gate-demo'),['--demo','--dir',state],{stdio:['ignore','pipe','pipe']});
let logs='';daemon.stderr.on('data',b=>logs+=b);
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{let browser;try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:8081/admin-api/ready')).ok)break}catch{}if(i===99)throw Error(logs);await sleep(100)}
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||undefined});
 const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('http://127.0.0.1:8081/#'+fs.readFileSync(path.join(state,'setup.token'),'utf8'));
 await page.locator('#user').fill('admin');await page.locator('#pass').fill('AdminPass123');await page.locator('#loginbtn').click();await saveRecovery(page);await page.locator('#view').waitFor();
 await page.locator('[data-nav=push]').click();
 assert.equal(await page.locator('#remoteEnabled').isChecked(),false);
 await page.locator('#remoteEnabled').check();await page.locator('#remotePublicURL').fill('http://approve.example.com');await page.locator('#remotePort').fill('18091');
 await page.locator('[data-action=save-settings]').click();await page.locator('#toast').filter({hasText:'HTTPS'}).waitFor();
 await page.locator('#remotePublicURL').fill('https://approve.example.com');
 await page.locator('[data-action=save-settings]').click();await page.locator('#toast').filter({hasText:'设置已保存'}).waitFor();
 await page.reload();await page.locator('[data-nav=push]').click();assert(await page.locator('#remoteEnabled').isChecked());assert.equal(await page.locator('#remotePort').inputValue(),'18091');
 assert.equal((await fetch('http://127.0.0.1:18091/admin-api/config')).status,404);
 assert.equal((await fetch('http://127.0.0.1:18091/style.css')).status,200);
 await page.locator('[data-action=remote-approval-test]').click();await page.locator('#toast').filter({hasText:'配置并保存推送渠道'}).waitFor();
 for(const width of [390,834,1440]){await page.setViewportSize({width,height:950});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'push page overflow '+width);await page.screenshot({path:path.join(root,'dist',`remote-approval-${width}.png`),fullPage:true})}
 await page.locator('#remoteEnabled').uncheck();await page.locator('[data-action=save-settings]').click();await page.locator('#toast').filter({hasText:'设置已保存'}).waitFor();
 await assert.rejects(fetch('http://127.0.0.1:18091/style.css'));
 // Reproduce a host dark-theme override, including WebKit text fill.
 await page.addStyleTag({content:'input {color:white!important;-webkit-text-fill-color:white!important;background:#222!important}'});
 await page.evaluate(()=>{window.open=(url,target,features)=>{window.tutorialOpened={url,target,features};return null}});
 const plugin=fs.readFileSync(path.join(root,'plugin.template.js'),'utf8').replace('@@SERVICE@@','""').replace('@@RECOVER@@','""');
 await page.addScriptTag({content:plugin});await page.locator('#ug-entry').click();
 for(const width of [390,834,1440]){await page.setViewportSize({width,height:950});for(const id of ['ug-lan','ug-gateway']){const css=await page.locator('#'+id).evaluate(el=>{const s=getComputedStyle(el);return [s.color,s.webkitTextFillColor,s.backgroundColor]});assert.deepEqual(css,['rgb(28, 45, 68)','rgb(28, 45, 68)','rgb(255, 255, 255)'])}await page.screenshot({path:path.join(root,'dist',`plugin-v119-${width}.png`)})}
 await page.locator('[data-ug=tutorial]').click();assert.deepEqual(await page.evaluate(()=>window.tutorialOpened),{url:'https://github.com/nanbeiluduo/UFI-Gate/blob/main/docs/INSTALL.md',target:'_blank',features:'noopener,noreferrer'});
 assert.deepEqual(errors,[]);console.log('Remote approval UI: default off, validation, save/reload, isolated listener, disable, test guidance, 3 widths, host CSS override and tutorial passed.');
}finally{if(browser)await browser.close();daemon.kill()}})().catch(e=>{console.error(e);process.exitCode=1});
