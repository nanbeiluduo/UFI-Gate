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

root = pathlib.Path(__file__).resolve().parent.parent
parser = argparse.ArgumentParser()
parser.add_argument('--out', default=str(root / 'dist'))
args = parser.parse_args()
out = pathlib.Path(args.out).resolve()
manifest = json.loads((out / 'build-info.json').read_text())
version = manifest['version']
plugin = out / f'UFI-Gate-{version}-arm64.js'
binary = (out / 'ufi-gate-arm64').read_bytes()
assert hashlib.sha256(plugin.read_bytes()).hexdigest() == manifest['plugin_sha256']
assert hashlib.sha256(binary).hexdigest() == manifest['binary_sha256']
assert binary[:4] == b'\x7fELF' and binary[4] == 2 and binary[5] == 1
assert int.from_bytes(binary[18:20], 'little') == 183, 'Not ARM64'
assert plugin.stat().st_size < 5 * 1024 * 1024

# Installer embeds one gzip/base64 payload; verify it matches this build.
payloads = re.findall(r'[A-Za-z0-9+/]{1000,}={0,2}', plugin.read_text())
assert any(gzip.decompress(base64.b64decode(p)) == binary for p in payloads)

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
assets = [plugin, source, out / 'build-info.json']
(out / 'SHA256SUMS').write_text(''.join(
    f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in assets
))
print(json.dumps({'version': version, 'plugin_bytes': plugin.stat().st_size,
                  'source_files': len([p for p in tracked if p]),
                  'assets': [p.name for p in assets] + ['SHA256SUMS']}))
