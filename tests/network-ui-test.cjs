const {saveRecovery}=require('./recovery-helper.cjs');
const {chromium}=require('playwright');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),state=fs.mkdtempSync(path.join(root,'dist','network-ui-'));
const daemon=spawn(process.env.DEMO_BINARY||path.join(root,'ufi-gate-demo'),['--demo','--dir',state],{stdio:['ignore','pipe','pipe']});
let logs='';daemon.stderr.on('data',b=>logs+=b);
const sleep=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{let browser;try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:8081/admin-api/ready')).ok)break}catch{}if(i===99)throw Error(logs);await sleep(100)}
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||undefined});
 const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('http://127.0.0.1:8081/#'+fs.readFileSync(path.join(state,'setup.token'),'utf8'));
 await page.locator('#user').fill('admin');await page.locator('#pass').fill('AdminPass123');await page.locator('#loginbtn').click();await saveRecovery(page);await page.locator('#view').waitFor();
 await page.locator('[data-nav=system]').click();await page.locator('#verified').uncheck();await page.locator('[data-action=save-settings]').click();await page.locator('#toast').filter({hasText:'设置已保存'}).waitFor();
 await page.locator('[data-nav=dashboard]').click();await page.locator('[data-action=toggle]').click();await page.locator('#verified').waitFor();assert.equal(await page.locator('#verified').isChecked(),false);assert.equal(await page.locator('#verified').evaluate(e=>document.activeElement===e),true);assert.equal(await page.evaluate(async()=> (await(await fetch('/admin-api/dashboard')).json()).enabled),false);
 assert(await page.locator('#verified').evaluate(e=>e.closest('label').getAnimations().length)>0);
 // This guided jump never auto-checks or auto-saves the user's confirmation.
 await page.evaluate(()=>{window.commands=[];window.badDetector=false;window.invalidDetector=false;window.runShellWithRoot=async cmd=>{commands.push(cmd);let content='';if(cmd==='id -u')content='0';else if(cmd==='uname -m')content='aarch64';else if(cmd.includes('[ -x '))content='YES';else if(cmd.includes('--network-inspect')){if(window.badDetector)return {success:false,content:'flag provided but not defined'};content=JSON.stringify({Protocol:1,Running:{LAN:'wlan0',Gateway:'192.168.0.1',Enabled:false},Saved:{LAN:'wlan0',Gateway:'192.168.0.1',Enabled:false},Bootstrap:{LAN:'br0',Gateway:'192.168.0.1'},Recommended:{LAN:'br0',Gateway:'192.168.0.1',Reason:'本机管理地址已核对'},Candidates:[{LAN:'br0',Gateway:'192.168.0.1',Reason:'本机地址'}],Interfaces:[{Name:'br0',Up:true,Addresses:['192.168.0.1/24']}],Warnings:['保存配置与安装引导不一致'],SavedValidation:{Status:'invalid',Message:'192.168.0.1 当前配置在 br0，而不是 wlan0。请检查热点接口。'},Validation:window.invalidDetector?{Status:'invalid',Message:'网关不匹配'}:{Status:'valid',Message:'接口匹配'}})}return {success:true,content}}});
 const plugin=fs.readFileSync(path.join(root,'plugin.template.js'),'utf8').replace('@@SERVICE@@','""').replace('@@RECOVER@@','""').replaceAll('@@ARCH@@','arm64');await page.addScriptTag({content:plugin});await page.locator('#ug-entry').click();
 await page.locator('[data-ug=network-repair]').click();await page.locator('#ug-repair-lan').waitFor();assert.equal(await page.locator('#ug-repair-lan').inputValue(),'br0');assert((await page.locator('#ug-repair-result').textContent()).includes('当前保存：wlan0'));
 const onlyReads=async()=>assert(!(await page.evaluate(()=>commands)).some(c=>c.includes('--network-repair ')||c.includes('nohup')||c.includes('sed ')));
 await page.locator('[data-ug=network-cancel]').click();await onlyReads();assert(await page.locator('#ug-repair').isHidden());
 await page.locator('[data-ug=network-repair]').click();page.once('dialog',d=>d.dismiss());await page.locator('[data-ug=network-confirm]').click();await onlyReads();
 await page.locator('#ug-repair-lan').fill('br0;reboot');await page.locator('[data-ug=network-confirm]').click();await page.locator('#ug-result').filter({hasText:'无效'}).waitFor();await onlyReads();
 await page.locator('[data-ug=network-redetect]').click();await page.evaluate(()=>window.invalidDetector=true);await page.locator('[data-ug=network-confirm]').click();await page.locator('#ug-result').filter({hasText:'网关不匹配'}).waitFor();await onlyReads();await page.evaluate(()=>window.invalidDetector=false);
 for(const width of [390,834,1440]){await page.setViewportSize({width,height:950});assert(await page.locator('#ug-install').evaluate(e=>e.scrollWidth<=e.clientWidth+1),'repair dialog overflow');await page.screenshot({path:path.join(root,'dist',`network-repair-${width}.png`)})}
 page.once('dialog',d=>{assert(d.message().includes('恢复密钥'));assert(d.message().includes('认证保持关闭'));d.accept()});await page.locator('[data-ug=network-confirm]').click();await page.locator('#ug-result').filter({hasText:'任务已提交'}).waitFor();assert((await page.evaluate(()=>commands)).some(c=>c.includes("--network-lan 'br0'")));
 await page.evaluate(()=>window.badDetector=true);await page.locator('[data-ug=network-redetect]').click();await page.locator('#ug-result').filter({hasText:'缺少专用检测命令'}).waitFor();assert.deepEqual(errors,[]);
 console.log('Network UI passed: confirmation jump/focus/highlight; actual/saved/bootstrap differences; recommend; redetect; cancel without writes; invalid input; explicit submit; old service guidance; 3 widths.');
 }finally{if(browser)await browser.close();daemon.kill()}})().catch(e=>{console.error(e);process.exitCode=1});
