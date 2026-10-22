const {saveRecovery}=require('./recovery-helper.cjs');
const {chromium}=require('playwright');
const fs=require('node:fs'),path=require('node:path'),assert=require('node:assert/strict');
const {spawn}=require('node:child_process');
const root=path.resolve(__dirname,'..'),state=fs.mkdtempSync(path.join(root,'dist','theme-ui-'));
const daemon=spawn(process.env.DEMO_BINARY||path.join(root,'ufi-gate-demo'),['--demo','--dir',state]);
let logs='';daemon.stderr.on('data',b=>logs+=b);const sleep=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{let browser;try{
 for(let i=0;i<100;i++){try{if((await fetch('http://127.0.0.1:8081/admin-api/ready')).ok)break}catch{}if(i===99)throw Error(logs);await sleep(100)}
 browser=await chromium.launch({headless:true,executablePath:process.env.CHROME_PATH||undefined});const page=await browser.newPage(),errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto('http://127.0.0.1:8081/#'+fs.readFileSync(path.join(state,'setup.token'),'utf8'));await page.locator('#user').fill('admin');await page.locator('#pass').fill('AdminPass123');await page.locator('#loginbtn').click();await saveRecovery(page);await page.locator('#view').waitFor();
 await page.locator('[data-nav=designer]').click();await page.getByText('组件与样式',{exact:true}).waitFor();
 await page.getByRole('button',{name:'＋ 公告',exact:true}).dragTo(page.locator('.pt-canvas > div').first());assert.equal(await page.locator('.pt-item').count(),1);
 await page.getByRole('button',{name:'＋ 文本',exact:true}).click();await page.getByLabel('文字',{exact:true}).fill('<script>alert(1)</script> 欢迎');await page.getByLabel('文字',{exact:true}).blur();
 await page.getByRole('button',{name:'保存草稿',exact:true}).click();await page.locator('.pt-status').filter({hasText:'草稿已保存'}).waitFor();assert.equal(await(await fetch('http://127.0.0.1:8090/api/portal-theme')).json(),null);
 await page.getByRole('button',{name:'发布',exact:true}).click();await page.locator('#confirm-accept').click();await page.locator('.pt-status').filter({hasText:'操作成功'}).waitFor();
 const visitor=await browser.newPage();await visitor.goto('http://127.0.0.1:8090');await visitor.locator('.pt-active').waitFor();assert.equal(await visitor.locator('.pt-region script').count(),0);assert.equal(await visitor.locator('#admin-login').count(),1);
 for(const key of ['login','waiting','success','restricted']){await page.getByLabel('页面',{exact:true}).selectOption(key);assert.equal(await page.locator('.pt-protected').count(),1)}
 for(const width of [320,390,768,1440]){await page.setViewportSize({width,height:800});assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'horizontal overflow '+width+' '+JSON.stringify(await page.evaluate(()=>[...document.querySelectorAll('*')].filter(e=>e.getBoundingClientRect().right>innerWidth+1&&getComputedStyle(e).position!=='fixed').map(e=>({tag:e.tagName,cls:e.className,w:e.getBoundingClientRect().width})).slice(-15))))}
 await page.setViewportSize({width:1440,height:900});await page.getByLabel('页面',{exact:true}).selectOption('login');await page.getByRole('button',{name:'＋ 双列容器',exact:true}).click();await page.getByRole('button',{name:'编辑容器内部',exact:true}).click();await page.getByRole('button',{name:'＋ 文本',exact:true}).click();await page.getByLabel('文字',{exact:true}).fill('容器内文字');await page.getByLabel('文字',{exact:true}).blur();await page.getByRole('button',{name:'返回外层容器',exact:true}).click();
 await page.getByRole('button',{name:'撤销',exact:true}).click();await page.getByRole('button',{name:'重做',exact:true}).click();
 const downloadWait=page.waitForEvent('download');await page.getByRole('button',{name:'导出主题',exact:true}).click();const download=await downloadWait;const exported=path.join(state,'export.json');await download.saveAs(exported);await page.getByLabel('导入主题',{exact:true}).setInputFiles(exported);await page.locator('.pt-status').filter({hasText:'主题已导入'}).waitFor();
 await page.screenshot({path:path.join(root,'dist','portal-designer.png'),fullPage:true});
 await page.getByRole('button',{name:'恢复默认',exact:true}).click();await page.locator('#confirm-accept').click();await page.locator('.pt-status').filter({hasText:'操作成功'}).waitFor();assert.equal(await(await fetch('http://127.0.0.1:8090/api/portal-theme')).json(),null);
 assert.deepEqual(errors,[]);console.log('Theme UI passed: draft isolation, publish, text safety, protected controls, four states, responsive layout, containers, undo/redo, reset.');
 }finally{if(browser)await browser.close();daemon.kill()}})().catch(e=>{console.error(e);process.exitCode=1});
