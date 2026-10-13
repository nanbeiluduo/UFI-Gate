#!/usr/bin/env python3
"""Verify the built installer and package only tracked project source."""
import argparse
import base64
import gzip
import hashlib
import json
import pathlib
import re
import subprocess
import zipfile
import tarfile

root = pathlib.Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser()
parser.add_argument('--out', default=str(root / 'dist'))
args = parser.parse_args()
out = pathlib.Path(args.out).resolve()
manifest = json.loads((out / 'build-info.json').read_text(encoding='utf-8'))
version = manifest['version']
plugins = []
for build in manifest['builds']:
    arch = build['arch']
    plugin = out / build['plugin']
    binary = (out / build['binary']).read_bytes()
    assert hashlib.sha256(plugin.read_bytes()).hexdigest() == build['plugin_sha256']
    assert hashlib.sha256(binary).hexdigest() == build['binary_sha256']
    elf_class, machine = {'arm32': (1, 40), 'arm64': (2, 183)}[arch]
    assert binary[:4] == b'\x7fELF' and binary[4] == elf_class and binary[5] == 1
    assert int.from_bytes(binary[18:20], 'little') == machine, f'Not {arch}'
    # Static deployment must not depend on Android's dynamic linker.
    phoff = int.from_bytes(binary[28:32] if elf_class == 1 else binary[32:40], 'little')
    offset = 42 if elf_class == 1 else 54
    phsize = int.from_bytes(binary[offset:offset+2], 'little')
    phnum = int.from_bytes(binary[offset+2:offset+4], 'little')
    assert all(int.from_bytes(binary[phoff+i*phsize:phoff+i*phsize+4], 'little') != 3
               for i in range(phnum)), 'Unexpected ELF interpreter'
    assert plugin.stat().st_size < 5 * 1024 * 1024
    text = plugin.read_text(encoding='utf-8')
    assert f"TARGET_ARCH='{arch}'" in text
    assert not re.search(r'@@[A-Z0-9_]+@@', text), 'Unresolved template placeholder'
    payloads = re.findall(r'[A-Za-z0-9+/]{1000,}={0,2}', text)
    assert any(gzip.decompress(base64.b64decode(p)) == binary for p in payloads)
    plugins.append(plugin)
assert plugins, 'No installers built'

tracked = subprocess.check_output(['git', 'ls-files', '-z'], cwd=root).split(b'\0')
source = out / f'UFI-Gate-{version}-source.zip'
with zipfile.ZipFile(source, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
    for raw in tracked:
        if not raw:
            continue
        rel = pathlib.Path(raw.decode())
        if rel.parts[0] in {'.git', 'dist', 'node_modules', 'demo-state'}:
            raise RuntimeError(f'Unexpected generated or private path: {rel}')
        archive.write(root / rel, str(pathlib.Path('UFI-Gate') / rel))
    archive.write(out / 'build-info.json', 'UFI-Gate/build-info.json')
update_manifest = json.loads((out / 'update.json').read_text(encoding='utf-8'))
assert update_manifest['version'] == version
update_assets = []
for a in update_manifest['assets']:
    item = out / a['name']
    assert item.stat().st_size == a['size']
    assert hashlib.sha256(item.read_bytes()).hexdigest() == a['sha256']
    build = next(b for b in manifest['builds'] if b['arch'] == a['arch'])
    with tarfile.open(item, 'r:gz') as archive:
        assert set(archive.getnames()) == {'ufi-gate', 'service.sh', 'emergency-recover.sh'}
        assert all(m.isfile() for m in archive.getmembers())
        assert archive.extractfile('ufi-gate').read() == (out / build['binary']).read_bytes()
    update_assets.append(item)
assets = plugins + update_assets + [source, out / 'build-info.json', out / 'update.json']
if manifest.get('signed_updates'):
    assert (out / 'update.json.sig').is_file()
    assets.append(out / 'update.json.sig')
(out / 'SHA256SUMS').write_text(''.join(
    f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in assets
))
print(json.dumps({'version': version, 'plugin_bytes': {p.name: p.stat().st_size for p in plugins},
                  'source_files': len([p for p in tracked if p]),
                  'assets': [p.name for p in assets] + ['SHA256SUMS']}))
