const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { createHash } = require('node:crypto');
const out = process.argv[2] || 'dist';
const manifest = JSON.parse(fs.readFileSync(path.join(out, 'build-info.json'), 'utf8'));
async function check(build, uname, root = true) {
  const writes = new Map(), commands = [];
  const elements = { 'ug-lan': {value: 'wlan0'}, 'ug-gateway': {value: '192.168.0.1'}, 'ug-result': {}, 'ug-open': {} };
  const context = vm.createContext({
    window: {}, document: {readyState: 'loading', addEventListener() {}, getElementById: id => elements[id]},
    File, Blob, Response, DecompressionStream, Uint8Array, atob, confirm: () => true,
    saveConfig: async (file, name) => { writes.set(name, file); return true; },
    runShellWithRoot: async cmd => {
      commands.push(cmd);
      let content = '';
      if (cmd === 'id -u') content = root ? '0' : '2000';
      else if (cmd === 'uname -m') content = uname;
      else if (cmd === 'ip -4 -o addr show') content = '3: wlan0 inet 192.168.0.1/24 scope global wlan0';
      else if (cmd.includes('service.sh status')) content = 'stopped';
      else if (cmd.includes('sha256sum')) {
        const bytes = Buffer.from(await writes.get('/data/ufi-gate/ufi-gate.new').arrayBuffer());
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
  await context.window.testAPI.install();
  assert.equal(writes.size, 4);
  const bytes = Buffer.from(await writes.get('/data/ufi-gate/ufi-gate.new').arrayBuffer());
  assert.equal(bytes[4], build.arch === 'arm32' ? 1 : 2);
  assert.equal(bytes.readUInt16LE(18), build.arch === 'arm32' ? 40 : 183);
  assert.equal(createHash('sha256').update(bytes).digest('hex'), build.binary_sha256);
  assert(elements['ug-result'].textContent.includes('服务已安装'));
}
(async () => {
  let cases = 0;
  for (const build of manifest.builds) {
    for (const uname of ['armv7l','armv8l','aarch64','arm64','armv6l','x86_64']) {
      await check(build, uname); cases++;
    }
    await check(build, build.arch === 'arm32' ? 'armv8l' : 'aarch64', false); cases++;
  }
  console.log(`Installer checks passed: ${cases} cases (architecture, Root, payload, deployment)`);
})().catch(error => { console.error(error); process.exitCode = 1; });
