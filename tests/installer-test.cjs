const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { createHash } = require('node:crypto');
const out = process.argv[2] || 'dist';
const manifest = JSON.parse(fs.readFileSync(path.join(out, 'build-info.json'), 'utf8'));
async function check(build, uname, root = true, local = 'NONE', approve = true) {
  const writes = new Map(), commands = [];
  const elements = { 'ug-lan': {value: 'wlan0'}, 'ug-gateway': {value: '192.168.0.1'}, 'ug-result': {}, 'ug-open': {} };
  const context = vm.createContext({
    window: {}, document: {readyState: 'loading', addEventListener() {}, getElementById: id => elements[id]},
    File, Blob, Response, DecompressionStream, Uint8Array, atob, confirm: () => approve,
    saveConfig: async (file, name) => { writes.set(name, file); return true; },
    runShellWithRoot: async cmd => {
      commands.push(cmd);
      let content = '';
      if (cmd === 'id -u') content = root ? '0' : '2000';
      else if (cmd === 'uname -m') content = uname;
      else if (cmd.includes('--network-inspect')) content = JSON.stringify({Protocol:1,Running:null,Saved:null,Bootstrap:null,Recommended:{LAN:'wlan0',Gateway:'192.168.0.1'},Candidates:[],Interfaces:[],Warnings:[],Validation:{Status:'valid',Message:'ok'}});
      else if (cmd.includes('--installed-info')) {if(local==='{}')return {success:false,content:'无法识别已安装版本，拒绝覆盖'};let v=local==='NONE'?{}:JSON.parse(local);if(v.Logs)v={version:'1.0.9',legacy:true};content=JSON.stringify(v);}
      else if (cmd.includes('sha256sum')) {
        const bytes = Buffer.from(await [...writes.entries()].find(([name])=>name.endsWith('/ufi-gate'))[1].arrayBuffer());
        assert(cmd.includes(createHash('sha256').update(bytes).digest('hex')));
        content = 'VERIFIED';
      } else if (cmd.includes('service.sh start')) content = 'ok';
      return {success: true, content};
    }, location: {hostname: '192.168.0.1'},
  });
  const source = fs.readFileSync(path.join(out, build.plugin), 'utf8');
  vm.runInContext(source.replace('})();', 'window.testAPI={inspect,install};})();'), context);
  const supported = root && (build.arch === 'arm32' ? ['armv7l','armv8l'] : ['aarch64','arm64']).includes(uname);
  if (!supported) {
    for (const method of ['inspect', 'install']) {
      await assert.rejects(context.window.testAPI[method](), root ? /此安装包仅内嵌/ : /Root 权限不可用/);
    }
    assert.equal(writes.size, 0);
    assert(commands.every(cmd => ['id -u','uname -m'].includes(cmd)), 'Mismatch must not change device');
    return;
  }
  await context.window.testAPI.inspect();
  assert(elements['ug-result'].textContent.includes(build.arch.toUpperCase()));
  if(local!=='NONE' && (local==='{}'||JSON.parse(local).version==='99.0.0')) {
    await assert.rejects(context.window.testAPI.install(), /拒绝覆盖|阻止降级/);
    assert([...writes.keys()].every(n=>n.includes('/network-probe-'))); return;
  }
  await context.window.testAPI.install();
  if(!approve){assert([...writes.keys()].every(n=>n.includes('/network-probe-')));assert(!commands.some(c=>c.includes('--apply-update')));return}
  for(const name of [...writes.keys()])if(name.includes('/network-probe-'))writes.delete(name);
  assert.equal(writes.size, 4);
  assert(!commands.some(c=>c.includes('pkill')||c.includes('cat /data/ufi-gate/state.json')),'Unsafe process matching or secret state output');
  assert([...writes.keys()].every(n=>/^\/data\/ufi-gate\/update-stage-[a-z0-9-]+\/(ufi-gate|service.sh|emergency-recover.sh|bootstrap.json)$/.test(n)), 'Only stage writes allowed');
  assert(commands.some(c=>c.includes('nohup')&&c.includes('--apply-update')),'Detached installer missing');
  if(local!=='NONE'&&JSON.parse(local).version===manifest.version)assert(commands.some(c=>c.includes('--repair')));
  const bytes = Buffer.from(await [...writes.entries()].find(([name])=>name.endsWith('/ufi-gate'))[1].arrayBuffer());
  assert.equal(bytes[4], build.arch === 'arm32' ? 1 : 2);
  assert.equal(bytes.readUInt16LE(18), build.arch === 'arm32' ? 40 : 183);
  assert.equal(createHash('sha256').update(bytes).digest('hex'), build.binary_sha256);
  assert(elements['ug-result'].textContent.includes('安装任务已提交'));
}
(async () => {
  let cases = 0;
  for (const build of manifest.builds) {
    const supported = build.arch === 'arm32' ? 'armv7l' : 'aarch64';
    for (const local of ['{}', JSON.stringify({version:'99.0.0'}), JSON.stringify({version:manifest.version}), JSON.stringify({Logs:[{Kind:'service',Message:'服务启动 1.0.9'}]})]) {await check(build,supported,true,local);cases++}
    await check(build,supported,true,'NONE',false);cases++;
    for (const uname of ['armv7l','armv8l','aarch64','arm64','armv6l','x86_64']) {
      await check(build, uname); cases++;
    }
    await check(build, build.arch === 'arm32' ? 'armv8l' : 'aarch64', false); cases++;
  }
  console.log(`Installer checks passed: ${cases} cases (architecture, Root, payload, deployment)`);
})().catch(error => { console.error(error); process.exitCode = 1; });
