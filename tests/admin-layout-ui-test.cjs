// DEMO_BINARY=/path/to/demo CHROME_PATH=/path/to/browser node tests/admin-layout-ui-test.cjs
const {saveRecovery}=require('./recovery-helper.cjs');
const {chromium}=require('playwright');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..');fs.mkdirSync(path.join(root,'dist'),{recursive:true});
const state=fs.mkdtempSync(path.join(root,'dist','layout-ui-'));
const daemon=spawn(process.env.DEMO_BINARY||path.join(root,'ufi-gate-demo'),['--demo','--dir',state],{stdio:['ignore','pipe','pipe']});
let logs='';daemon.stderr.on('data',b=>logs+=b);const sleep=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{let browser;try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:8081/admin-api/ready')).ok)break}catch{}if(i===99)throw Error(logs);await sleep(100)}
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||undefined});
 const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 page.on('dialog',d=>{errors.push('Unexpected browser dialog: '+d.message());d.dismiss()});
 await page.goto('http://127.0.0.1:8081/#'+fs.readFileSync(path.join(state,'setup.token'),'utf8'));
 await page.locator('#user').fill('admin');await page.locator('#pass').fill('AdminPass123');await page.locator('#loginbtn').click();await saveRecovery(page);await page.locator('#view').waitFor();
 let deletes=0;await page.route('**/admin-api/notifications/delete',r=>{deletes++;return r.fulfill({json:{ok:true}})});
 await page.locator('#message-float').click();await page.locator('#message-clear').click();
 assert(await page.locator('#confirm-accept').isDisabled());
 await page.locator('#confirm-accept').dispatchEvent('click');await page.keyboard.press('Enter');assert.equal(deletes,0);
 // Enter on autofocus Cancel may cancel; reopen to verify an actual three-second gate.
 if(!await page.locator('#confirm-dialog').count())await page.locator('#message-clear').click();
 const started=Date.now();await page.locator('#confirm-dialog').evaluate(e=>{e.tabIndex=-1;e.focus()});await page.keyboard.press('Enter');assert.equal(deletes,0);
 await page.locator('#confirm-accept').click();assert(Date.now()-started>=2800,'confirmation became available too early');
 await page.waitForFunction(()=>!document.querySelector('#confirm-dialog'));assert.equal(deletes,1);assert(await page.locator('#message-box').isVisible());
 await page.locator('#message-clear').click();assert(await page.locator('#confirm-accept').isDisabled());await page.keyboard.press('Escape');assert.equal(deletes,1);
 assert.equal(await page.evaluate(()=>document.activeElement.id),'message-clear');
 await page.locator('#message-clear').click();await page.waitForFunction(()=>!document.querySelector('#confirm-accept').disabled);assert.equal(deletes,1,'countdown must not auto-submit');await page.locator('#confirm-cancel').click();
 await page.locator('#message-delete-read').click();await page.screenshot({path:path.join(root,'dist','admin-122-confirm.png')});await page.locator('#confirm-cancel').click();assert.equal(deletes,1);
 await page.locator('#message-close').click();
 const sizes=[[320,568],[360,640],[390,844],[600,700],[768,1024],[834,390],[1024,600],[1366,600],[1440,900],[1920,1080],[2560,1440],[720,450]];
 let checks=0;
 for(const [width,height] of sizes){
  await page.setViewportSize({width,height});
  for(const section of ['dashboard','devices','approvals','accounts','auth','policy','appearance','push','system','updates','logs','about']){
   if(width<768){await page.locator('#menu').click();assert(await page.locator('#sidebar').evaluate(e=>e.classList.contains('open')))}
   assert(await page.locator('#logout').evaluate(e=>{const r=e.getBoundingClientRect();return r.top>=0&&r.bottom<=innerHeight+1}),'logout outside viewport '+width+'x'+height);
   await page.locator('[data-nav='+section+']').click();await page.waitForFunction(s=>document.querySelector('#heading').textContent===s,({dashboard:'网络概览',devices:'设备管理',approvals:'审批中心',accounts:'上网账号',auth:'认证方式',policy:'网址规则',appearance:'门户外观',push:'消息推送',system:'服务设置',updates:'更新管理',logs:'运行日志',about:'关于'})[section]);
   await sleep(40);assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'overflow '+section+' '+width+'x'+height);checks++;
  }
  if([320,390,1366,2560].includes(width)){
   if(width<768)await page.locator('#menu').click();
   await sleep(250);await page.screenshot({path:path.join(root,'dist',`admin-122-${width}.png`)});
   if(width<768)await page.locator('#nav-close').click();
  }
 }
 await page.setViewportSize({width:390,height:500});await page.locator('#menu').click();await page.keyboard.press('Escape');assert.equal(await page.locator('#menu').getAttribute('aria-expanded'),'false');
 await page.locator('#menu').click();await page.locator('#nav-backdrop').click({position:{x:370,y:200}});assert.equal(await page.locator('#menu').getAttribute('aria-expanded'),'false');
 await page.locator('#menu').click();await page.locator('[data-nav=devices]').click();await page.locator('[data-action=device-edit]').first().click();
 assert(await page.locator('#closeDialog').evaluate(e=>e.getBoundingClientRect().bottom<=innerHeight));
 assert(await page.locator('#dialog .dialog-body').evaluate(e=>e.scrollHeight>e.clientHeight),'long form should scroll internally');await page.locator('#closeDialog').click();
 // Long untrusted text stays text, even inside nested confirmations.
 await page.evaluate(()=>{window.ufiAdminUI.confirm('<img src=x onerror=alert(1)>\n'+'很长的提示'.repeat(400))});
 assert.equal(await page.locator('#confirm-dialog img').count(),0);assert(await page.locator('#confirm-cancel').evaluate(e=>e.getBoundingClientRect().bottom<=innerHeight));await page.locator('#confirm-cancel').click();
 await page.emulateMedia({colorScheme:'dark',reducedMotion:'reduce'});await page.locator('[data-action=toggle]').click();await page.locator('#verified').waitFor();assert.equal(await page.locator('#verified').isChecked(),false);
 await page.screenshot({path:path.join(root,'dist','admin-122-dark.png')});
 await page.locator('#menu').click();await page.locator('#logout').click();await page.locator('#login').waitFor();assert(!await page.locator('body').evaluate(e=>e.classList.contains('nav-open')));
 assert.deepEqual(errors,[]);console.log(`Admin UI passed: ${checks} page/viewport checks; real 3-second gate, early click/Enter, cancel/Escape, no auto-submit, nested dialog, focus return, scrolling forms, text escaping, mobile navigation/logout, dark/reduced-motion.`);
 }finally{if(browser)await browser.close();daemon.kill()}})().catch(e=>{console.error(e);process.exitCode=1});
