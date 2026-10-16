const {saveRecovery}=require('./recovery-helper.cjs');
/* Local integration/viewport test. npm install playwright; supply CHROME_PATH if needed.
 * Starts a --demo service as a child so it never alters system firewall/network settings.
 */
const {chromium}=require('playwright');
const fs=require('fs'),os=require('os'),path=require('path'),{spawn}=require('child_process');
const root=path.resolve(__dirname,'..'),state=fs.mkdtempSync(path.join(os.tmpdir(),'ufi-gate-ui-'));
const daemon=spawn(process.env.DEMO_BINARY||path.join(root,'ufi-gate-demo'),['--demo','--dir',state],{stdio:['ignore','pipe','pipe']});
let log='';daemon.stderr.on('data',b=>log+=b);const sleep=ms=>new Promise(r=>setTimeout(r,ms));
const assert=(v,msg)=>{if(!v)throw Error(msg)};
(async()=>{let browser;try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:8081/admin-api/ready')).ok)break}catch{}await sleep(50);if(i===99)throw Error('service did not start: '+log)}
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||undefined,args:['--no-sandbox']});
 const context=await browser.newContext({viewport:{width:1440,height:1000}}),page=await context.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 const token=fs.readFileSync(path.join(state,'setup.token'),'utf8').trim();
 await page.goto('http://127.0.0.1:8081/#'+token);await page.locator('#user').fill('admin');await page.locator('#pass').fill('AdminPass123');await page.locator('#loginbtn').click();await saveRecovery(page);await page.locator('#view').waitFor();
 async function api(p,v){const res=await context.request.post('http://127.0.0.1:8081/admin-api/'+p,{data:v,headers:{Origin:'http://127.0.0.1:8081'}});assert(res.ok(),p+': '+await res.text());return res.json()}
 await api('account',{Name:'guest',Password:'GuestPass123',Action:'create'});
 await api('device',{MAC:'02:00:00:00:00:22',Action:'trust'});await api('device',{MAC:'02:00:00:00:00:33',Action:'block'});
 await api('toggle',{Enabled:true});
 fs.mkdirSync(path.join(root,'tests','screenshots'),{recursive:true});let checked=0;
 for(const width of [390,834,1440]){await page.setViewportSize({width,height:1000});for(const panel of ['dashboard','devices','approvals','accounts','auth','policy','appearance','push','system','logs']){if(width<768)await page.locator('#menu').click();await page.locator('[data-nav='+panel+']').click();await sleep(150);const dimensions=await page.evaluate(()=>({vw:innerWidth,sw:document.documentElement.scrollWidth}));assert(dimensions.sw<=dimensions.vw+1,`overflow ${panel} @ ${width}: ${dimensions.sw}`);checked++}if(width<768)await page.locator('#menu').click();await page.locator('[data-nav=dashboard]').click();await sleep(250);await page.screenshot({path:path.join(root,'tests','screenshots','admin-'+width+'.png'),fullPage:true})}
 await api('device',{MAC:'02:00:00:00:00:02',Action:'revoke'});
 const portal=await context.newPage();portal.on('pageerror',e=>errors.push(e.message));await portal.setViewportSize({width:390,height:844});await portal.goto('http://127.0.0.1:8090/');await portal.locator('#username').fill('guest');await portal.locator('#password').fill('WrongPass123');await portal.locator('#submit').click();await portal.getByText('认证未通过，请检查后重试').waitFor();await portal.locator('#password').fill('GuestPass123');await portal.locator('#submit').click();await portal.getByRole('heading',{name:'连接成功'}).waitFor();
 await api('device',{MAC:'02:00:00:00:00:02',Action:'revoke'});await portal.reload();await portal.locator('[data-method=captcha]').click();await portal.waitForFunction(()=>document.getElementById('captcha')?.getAttribute('src')?.startsWith('data:'));
 const src=await portal.locator('#captcha').getAttribute('src'),svg=decodeURIComponent(src.split(',').slice(1).join(',')),answer=[...svg.matchAll(/>([A-Z0-9])<\/text>/g)].map(m=>m[1]).join('');assert(answer.length===5,'captcha SVG not rendered');await portal.locator('#answer').fill(answer);await portal.locator('#submit').click();await portal.getByRole('heading',{name:'连接成功'}).waitFor();
 await api('device',{MAC:'02:00:00:00:00:02',Action:'revoke'});await portal.reload();await portal.locator('[data-method=approval]').click();await portal.locator('#name').fill('测试访客');await portal.locator('#reason').fill('集成测试');await portal.locator('#submit').click();await portal.getByRole('heading',{name:'正在等待管理员'}).waitFor();
 const d=await(await context.request.get('http://127.0.0.1:8081/admin-api/dashboard')).json();const req=d.requests.find(r=>r.Name==='测试访客');assert(req,'request not found');await api('decision',{ID:req.ID,Action:'allow',Reason:'测试通过'});await portal.getByRole('heading',{name:'连接成功'}).waitFor({timeout:12000});
 await api('device',{MAC:'02:00:00:00:00:02',Action:'block'});await portal.reload();await portal.getByRole('heading',{name:'设备暂不可上网'}).waitFor();await api('device',{MAC:'02:00:00:00:00:02',Action:'release'});await portal.reload();await portal.getByRole('heading',{name:'连接成功'}).waitFor();
 await api('device',{MAC:'02:00:00:00:00:02',Action:'revoke'});await portal.reload();for(const width of [390,834,1440]){await portal.setViewportSize({width,height:844});assert(await portal.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'portal overflow');await portal.screenshot({path:path.join(root,'tests','screenshots','portal-'+width+'.png'),fullPage:true})}
 await api('toggle',{Enabled:false});
 // Validate the actual single-file installer without executing any Root commands.
 if(process.env.PLUGIN_PATH){
  await page.evaluate((arch)=>{window.mockWrites={};window.confirm=()=>true;window.saveConfig=async(file,p)=>{window.mockWrites[p]=file;return true};window.runShellWithRoot=async(cmd)=>{let text='';if(cmd==='id -u')text='0';else if(cmd==='uname -m')text=arch==='arm32'?'armv8l':'aarch64';else if(cmd==='ip -4 -o addr show')text='3: wlan0 inet 192.168.0.1/24 brd 192.168.0.255 scope global wlan0';else if(cmd.includes('sha256sum')){const b=await window.mockWrites['/data/ufi-gate/ufi-gate.new'].arrayBuffer();const digest=[...new Uint8Array(await crypto.subtle.digest('SHA-256',b))].map(v=>v.toString(16).padStart(2,'0')).join('');if(!cmd.includes(digest))throw Error('payload checksum mismatch');text='VERIFIED'}else if(cmd.includes('service.sh status'))text='stopped';else if(cmd.includes('service.sh start'))text='ok';return {success:true,content:text}}},process.env.PLUGIN_ARCH||'arm64');
  await page.addScriptTag({content:fs.readFileSync(process.env.PLUGIN_PATH,'utf8')});await page.locator('#ug-entry').click();await page.locator('[data-ug=inspect]').click();await page.getByText('Root 与 '+(process.env.PLUGIN_ARCH||'arm64').toUpperCase()+' 检测通过。',{exact:false}).waitFor();await page.locator('[data-ug=install]').click();await page.getByText('服务已安装。下一步打开独立后台',{exact:false}).waitFor({timeout:30000});const payload=await page.evaluate(async()=>{const b=new Uint8Array(await window.mockWrites['/data/ufi-gate/ufi-gate.new'].arrayBuffer());return {magic:Array.from(b.slice(0,4)),machine:b[18]+256*b[19],size:b.length,files:Object.keys(window.mockWrites)}});assert(payload.magic.join(',')==='127,69,76,70'&&payload.machine===(process.env.PLUGIN_ARCH==='arm32'?40:183)&&payload.size>1000000,'ARM payload validation');console.log('installer payload verified: '+JSON.stringify(payload))
 }
 assert(errors.length===0,'browser errors: '+errors.join('; '));console.log(JSON.stringify({panels_checked:checked,viewports:[390,834,1440],flows:['setup','account-error','account-success','captcha','request','admin-approve','ban','release','disable'],browser_errors:errors},null,2));
 }finally{if(browser)await browser.close();daemon.kill('SIGTERM');await sleep(300);fs.rmSync(state,{recursive:true,force:true})}
})().catch(e=>{console.error(e);process.exitCode=1});
