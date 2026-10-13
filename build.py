#!/usr/bin/env python3
"""Build static ARM32/ARM64 installers with Go 1.24+."""
import argparse, base64, gzip, hashlib, io, json, os, pathlib, re, subprocess, tarfile
root = pathlib.Path(__file__).resolve().parent
parser = argparse.ArgumentParser()
parser.add_argument('--go', default='go', help='Go executable')
parser.add_argument('--out', default=str(root / 'dist'))
parser.add_argument('--arch', choices=['all', 'arm32', 'arm64'], default='all')
parser.add_argument('--version', default=None, help='Version string (overrides main.go const version)')
parser.add_argument('--require-signature', action='store_true', help='Refuse unsigned production releases')
args = parser.parse_args()
out = pathlib.Path(args.out).resolve()
out.mkdir(parents=True, exist_ok=True)
if args.version:
    version = args.version
else:
    version = re.search(r'const version = "([^"]+)"', (root / 'main.go').read_text(encoding='utf-8')).group(1)
if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', version):
    raise SystemExit('Version must be X.Y.Z')
public_key = ''
if os.environ.get('UPDATE_SIGNING_KEY'):
    public_key = subprocess.check_output([args.go, 'run', '-mod=vendor', './tools/update-sign', '--public'], cwd=root, text=True).strip()
if args.require_signature and (not public_key or args.arch != 'all'):
    raise SystemExit('Production release needs UPDATE_SIGNING_KEY and both architectures')
updates = {'version': version, 'protocol': 1, 'min_version': '1.0.9', 'assets': []}
targets = ['arm32', 'arm64'] if args.arch == 'all' else [args.arch]
manifest = {'version': version, 'go': subprocess.check_output([args.go, 'version'], text=True).strip(), 'builds': []}
for arch in targets:
    env = {**os.environ, 'CGO_ENABLED': '0', 'GOOS': 'linux', 'GOARCH': 'arm' if arch == 'arm32' else 'arm64'}
    if arch == 'arm32':
        env['GOARM'] = '7,softfloat'
    binary = out / f'ufi-gate-{arch}'
    subprocess.run([args.go, 'build', '-mod=vendor', '-buildvcs=false', '-trimpath', f'-ldflags=-s -w -buildid= -X main.releaseVersion={version} -X main.updatePublicKey={public_key}', '-o', str(binary), '.'], cwd=root, env=env, check=True)
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
    update_name = f'UFI-Gate-{version}-{arch}-update.tar.gz'
    tar_bytes = io.BytesIO()
    with tarfile.open(fileobj=tar_bytes, mode='w', format=tarfile.USTAR_FORMAT) as archive:
        for name, content in [('ufi-gate', data), ('service.sh', (root / 'service.sh').read_bytes().replace(b'\r\n', b'\n')), ('emergency-recover.sh', (root / 'emergency-recover.sh').read_bytes().replace(b'\r\n', b'\n'))]:
            entry = tarfile.TarInfo(name)
            entry.size, entry.mode, entry.mtime = len(content), 0o700, 0
            archive.addfile(entry, io.BytesIO(content))
    update_data = gzip.compress(tar_bytes.getvalue(), compresslevel=9, mtime=0)
    (out / update_name).write_bytes(update_data)
    updates['assets'].append({'arch': arch, 'name': update_name, 'sha256': hashlib.sha256(update_data).hexdigest(), 'size': len(update_data)})
    manifest['builds'].append({
        'arch': arch, 'goos': 'linux', 'goarch': env['GOARCH'],
        'goarm': env.get('GOARM') if arch == 'arm32' else None,
        'plugin': plugin.name, 'binary': binary.name,
        'plugin_bytes': plugin.stat().st_size, 'binary_bytes': len(data),
        'compressed_bytes': len(packed),
        'plugin_sha256': hashlib.sha256(plugin.read_bytes()).hexdigest(),
        'binary_sha256': hashlib.sha256(data).hexdigest(),
    })
manifest['update_public_key'] = public_key
manifest['signed_updates'] = bool(public_key)
(out / 'update.json').write_text(json.dumps(updates, indent=2), encoding='utf-8', newline='\n')
if public_key:
    subprocess.run([args.go, 'run', '-mod=vendor', './tools/update-sign', '--manifest', str(out / 'update.json')], cwd=root, check=True)
else:
    (out / 'update.json.sig').unlink(missing_ok=True)
(out / 'build-info.json').write_text(json.dumps(manifest, indent=2, ensure_ascii=False), encoding='utf-8')
print(json.dumps(manifest, ensure_ascii=False))
