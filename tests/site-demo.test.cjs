const assert = require('assert');
const engine = require('../site/js/engine.js');
const github = require('../site/js/github.js');

engine.reset();
let state = engine.load();
assert.strictEqual(state.config.Enabled, true);
assert.strictEqual(state.devices.phone.connected, false);
assert.strictEqual(engine.deviceState(state.devices.pc), 'online');
assert.strictEqual(engine.portalKind(state, state.devices.tablet), 'login');

const bad = engine.authAccount('phone', 'guest', 'nope');
assert.strictEqual(bad.ok, false);

const offline = engine.connect('phone');
assert.strictEqual(offline.ok, true);
const authed = engine.authAccount('phone', 'guest', 'wifi2026');
assert.strictEqual(authed.ok, true);
assert.strictEqual(engine.deviceState(engine.load().devices.phone), 'online');

engine.reset();
engine.connect('phone');
const asked = engine.requestApproval('phone', '小陈', '临时查资料');
assert.strictEqual(asked.ok, true);
state = engine.load();
assert.strictEqual(state.requests[0].Status, 'pending');
assert.strictEqual(state.messages.length, 1);
assert.strictEqual(state.config.Push.Kind, 'wecom');
const allowed = engine.decide(state.requests[0].ID, 'allow', '可以');
assert.strictEqual(allowed.ok, true);
assert.strictEqual(engine.deviceState(engine.load().devices.phone), 'online');

engine.reset();
engine.connect('tablet');
engine.authAccount('tablet', 'guest', 'wifi2026');
const dropped = engine.addTraffic('phone', 30 * engine.MiB);
assert.strictEqual(dropped.ok, false);
const used = engine.addTraffic('tablet', 30 * engine.MiB);
assert.strictEqual(used.ok, true);
assert.ok(engine.load().devices.tablet.Down >= 30 * engine.MiB);
const filled = engine.fillQuota('tablet');
assert.strictEqual(filled.ok, true);
assert.strictEqual(engine.deviceState(engine.load().devices.tablet), 'quota');

engine.reset();
engine.connect('phone');
const black = engine.visit('phone', 'https://ads.example.com/home');
assert.strictEqual(black.detail.kind, 'black');
const white = engine.visit('phone', 'captive.apple.com');
assert.strictEqual(white.detail.kind, 'white');
const portal = engine.visit('phone', 'example.com');
assert.strictEqual(portal.detail.kind, 'portal');
assert.strictEqual(engine.load().devices.phone.ui, 'portal');

const summary = github.summarize(
  { stargazers_count: 34 },
  [{
    tag_name: 'v1.2.3',
    draft: false,
    prerelease: false,
    assets: [
      { name: 'UFI-Gate-1.2.3-arm64.js', browser_download_url: 'https://example/arm64', size: 10, download_count: 14 },
      { name: 'UFI-Gate-1.2.3-arm64-update.tar.gz', browser_download_url: 'https://example/tar', size: 10, download_count: 4 },
      { name: 'UFI-Gate-1.2.3-arm32.js', browser_download_url: 'https://example/arm32', size: 12, download_count: 12 }
    ]
  }, {
    tag_name: 'v1.2.2',
    assets: [{ name: 'UFI-Gate-1.2.2-arm64.js', browser_download_url: 'https://example/old', size: 8, download_count: 9 }]
  }],
  null
);
assert.strictEqual(summary.stars, 34);
assert.strictEqual(summary.downloads, 14 + 4 + 12 + 9);
assert.strictEqual(summary.release.arm64.name, 'UFI-Gate-1.2.3-arm64.js');
assert.strictEqual(summary.release.arm32.name, 'UFI-Gate-1.2.3-arm32.js');
assert.strictEqual(github.archOf('UFI-Gate-1.2.3-arm64-update.tar.gz'), '');

const empty = github.summarize({ stargazers_count: 0 }, [], null);
assert.strictEqual(empty.stars, 0);
assert.strictEqual(empty.downloads, null);
assert.strictEqual(empty.release, null);

console.log('site demo tests passed');
