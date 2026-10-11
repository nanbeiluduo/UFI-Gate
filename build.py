#!/usr/bin/env python3
"""Build the self-contained ARM64 .js using Go 1.24+. No download during installation."""
import argparse,base64,gzip,hashlib,json,os,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parent
parser=argparse.ArgumentParser()
parser.add_argument('--go',default='go',help='Go executable')
parser.add_argument('--out',default=str(root/'dist'))
args=parser.parse_args()
out=pathlib.Path(args.out).resolve();out.mkdir(parents=True,exist_ok=True)
env={**os.environ,'CGO_ENABLED':'0','GOOS':'linux','GOARCH':'arm64'}
binary=out/'ufi-gate-arm64'
subprocess.run([args.go,'build','-mod=vendor','-buildvcs=false','-trimpath','-ldflags=-s -w -buildid=','-o',str(binary),'.'],cwd=root,env=env,check=True)
data=binary.read_bytes();packed=gzip.compress(data,compresslevel=9,mtime=0)
text=(root/'plugin.template.js').read_text()
substitutions={'@@PAYLOAD@@':base64.b64encode(packed).decode(),'@@SHA256@@':hashlib.sha256(data).hexdigest(),'@@SERVICE@@':json.dumps((root/'service.sh').read_text(),ensure_ascii=False),'@@RECOVER@@':json.dumps((root/'emergency-recover.sh').read_text(),ensure_ascii=False)}
for key,value in substitutions.items():text=text.replace(key,value)
plugin=out/'UFI-Gate-1.0.0-beta.1-arm64.js';plugin.write_text(text)
assert len(json.dumps({'text':text}).encode())<5*1024*1024,'Plugin exceeds host total storage limit'
manifest={'version':'1.0.0-beta.1','arch':'linux-arm64-static','go':subprocess.check_output([args.go,'version'],text=True).strip(),'plugin_bytes':plugin.stat().st_size,'binary_bytes':len(data),'compressed_bytes':len(packed),'plugin_sha256':hashlib.sha256(plugin.read_bytes()).hexdigest(),'binary_sha256':hashlib.sha256(data).hexdigest()}
(out/'build-info.json').write_text(json.dumps(manifest,indent=2,ensure_ascii=False))
print(json.dumps(manifest,ensure_ascii=False))
