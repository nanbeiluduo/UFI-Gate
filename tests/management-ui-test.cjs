const {saveRecovery}=require('./recovery-helper.cjs');
// Run against a demo binary: DEMO_BINARY=/path/to/binary node tests/management-ui-test.cjs
const {chromium}=require('playwright');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn,execFileSync}=require('node:child_process');
const root=path.resolve(__dirname,'..');
fs.mkdirSync(path.join(root,'dist'),{recursive:true});
const state=fs.mkdtempSync(path.join(root,'dist','management-ui-'));
const binary=process.env.DEMO_BINARY||path.join(root,'ufi-gate-demo');
const daemon=spawn(binary,['--demo','--dir',state],{stdio:['ignore','pipe','pipe']});
let logs='';daemon.stderr.on('data',b=>logs+=b);
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{let browser;try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:8081/admin-api/ready')).ok)break}catch{}if(i===99)throw Error(logs);await sleep(100)}
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||undefined});
 const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 const token=fs.readFileSync(path.join(state,'setup.token'),'utf8');
 await page.goto('http://127.0.0.1:8081/#'+token);
 await page.locator('#user').fill('admin');await page.locator('#pass').fill('AdminPass123');await page.locator('#loginbtn').click();await saveRecovery(page);await page.locator('#view').waitFor();
 await page.locator('[data-nav=devices]').click();
 await page.locator('[data-action=device-rate]').click();await page.locator('#manualRate').fill('256');await page.locator('#dialogForm .primary').click();await page.getByText('独立限速 256 KiB/s',{exact:false}).waitFor();
 await page.reload();await page.locator('[data-nav=devices]').click();await page.getByText('独立限速 256 KiB/s',{exact:false}).waitFor();
 await page.locator('[data-action=device-rate-clear]').click();await page.locator('[data-action=device-rate-clear]').waitFor({state:'detached'});
 // Adding management permission does not grant Internet access.
 await page.evaluate(async()=>{await fetch('/admin-api/device',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({MAC:'02:00:00:00:00:33',Action:'block'})})});
 await page.locator('[data-action=refresh]').click();
 const card=page.locator('.device').filter({hasText:'02:00:00:00:00:33'});
 page.once('dialog',d=>d.accept());await card.locator('[data-action=device-admin-add]').click();await card.getByText('管理员设备',{exact:false}).first().waitFor();
 assert(await card.getByText('已封禁',{exact:true}).isVisible());
 page.once('dialog',d=>d.accept());await card.locator('[data-action=device-admin-remove]').click();await card.locator('[data-action=device-admin-add]').waitFor();
 for(const width of [390,834,1440]){await page.setViewportSize({width,height:950});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'device page overflow '+width)}
 // Load the actual plugin UI with a mocked host Root bridge.
 const commands=[];
 await page.exposeFunction('runShellWithRoot',async command=>{commands.push(command);if(command.endsWith('--reset-admin'))return {success:true,content:execFileSync(binary,['--dir',state,'--reset-admin'],{encoding:'utf8'})};if(command.includes('setup.token'))return {success:true,content:fs.readFileSync(path.join(state,'setup.token'),'utf8')};return {success:true,content:''}});
 const plugin=fs.readFileSync(path.join(root,'plugin.template.js'),'utf8').replace('@@SERVICE@@','""').replace('@@RECOVER@@','""');
 await page.addScriptTag({content:plugin});await page.locator('#ug-entry').click();
 page.once('dialog',d=>d.dismiss());await page.locator('[data-ug=token]').click();assert.equal(commands.length,0,'cancel must not execute reset');
 page.once('dialog',d=>d.accept());await page.locator('[data-ug=token]').click();await page.locator('#ug-result').filter({hasText:'初始化凭证（20 分钟有效）'}).waitFor();
 assert.equal(commands.length,2);assert(!commands[0].includes('service.sh'));
 const newToken=fs.readFileSync(path.join(state,'setup.token'),'utf8');assert.notEqual(newToken,token);
 const saved=JSON.parse(fs.readFileSync(path.join(state,'state.json'),'utf8'));assert.equal(saved.Config.AdminHash,'');assert.equal(saved.Devices['02:00:00:00:00:02'].Trusted,true);assert.equal(saved.Devices['02:00:00:00:00:33'].Banned,true);
 await page.goto('http://127.0.0.1:8081/?reinitialize=1#'+newToken);await page.locator('#user').fill('new-admin');await page.locator('#pass').fill('NewAdmin123');await page.locator('#loginbtn').click();await saveRecovery(page);await page.locator('#view').waitFor();
 assert.deepEqual(errors,[]);console.log('Management UI: rate, persistence, admin grants/removal, 3 viewports, reset confirmation and reinitialization passed.');
}finally{if(browser)await browser.close();daemon.kill()}})().catch(e=>{console.error(e);process.exitCode=1});
