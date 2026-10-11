#!/usr/bin/env python3
"""Build static ARM32/ARM64 installers with Go 1.24+."""
import argparse, base64, gzip, hashlib, json, os, pathlib, re, subprocess
root = pathlib.Path(__file__).resolve().parent
parser = argparse.ArgumentParser()
parser.add_argument('--go', default='go', help='Go executable')
parser.add_argument('--out', default=str(root / 'dist'))
parser.add_argument('--arch', choices=['all', 'arm32', 'arm64'], default='all')
parser.add_argument('--version', default=None, help='Version string (overrides main.go const version)')
args = parser.parse_args()
out = pathlib.Path(args.out).resolve()
out.mkdir(parents=True, exist_ok=True)
if args.version:
    version = args.version
else:
    version = re.search(r'const version = "([^"]+)"', (root / 'main.go').read_text(encoding='utf-8')).group(1)
targets = ['arm32', 'arm64'] if args.arch == 'all' else [args.arch]
manifest = {'version': version, 'go': subprocess.check_output([args.go, 'version'], text=True).strip(), 'builds': []}
for arch in targets:
    env = {**os.environ, 'CGO_ENABLED': '0', 'GOOS': 'linux', 'GOARCH': 'arm' if arch == 'arm32' else 'arm64'}
    if arch == 'arm32':
        env['GOARM'] = '7,softfloat'
    binary = out / f'ufi-gate-{arch}'
    subprocess.run([args.go, 'build', '-mod=vendor', '-buildvcs=false', '-trimpath', '-ldflags=-s -w -buildid=', '-o', str(binary), '.'], cwd=root, env=env, check=True)
    data = binary.read_bytes()
    packed = gzip.compress(data, compresslevel=9, mtime=0)
    text = (root / 'plugin.template.js').read_text(encoding='utf-8')
    substitutions = {
        '@@VERSION@@': version, '@@ARCH@@': arch,
        '@@PAYLOAD@@': base64.b64encode(packed).decode(),
        '@@SHA256@@': hashlib.sha256(data).hexdigest(),
        '@@SERVICE@@': json.dumps((root / 'service.sh').read_text(encoding='utf-8'), ensure_ascii=False),
        '@@RECOVER@@': json.dumps((root / 'emergency-recover.sh').read_text(encoding='utf-8'), ensure_ascii=False),
    }
    for key, value in substitutions.items():
        text = text.replace(key, value)
    plugin = out / f'UFI-Gate-{version}-{arch}.js'
    plugin.write_text(text, encoding='utf-8', newline='\n')
    assert len(json.dumps({'text': text}).encode()) < 5 * 1024 * 1024, 'Plugin exceeds host total storage limit'
    manifest['builds'].append({
        'arch': arch, 'goos': 'linux', 'goarch': env['GOARCH'],
        'goarm': env.get('GOARM') if arch == 'arm32' else None,
        'plugin': plugin.name, 'binary': binary.name,
        'plugin_bytes': plugin.stat().st_size, 'binary_bytes': len(data),
        'compressed_bytes': len(packed),
        'plugin_sha256': hashlib.sha256(plugin.read_bytes()).hexdigest(),
        'binary_sha256': hashlib.sha256(data).hexdigest(),
    })
(out / 'build-info.json').write_text(json.dumps(manifest, indent=2, ensure_ascii=False), encoding='utf-8')
print(json.dumps(manifest, ensure_ascii=False))
