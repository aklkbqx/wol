#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
host=$(uname -s)
version=$(sed -n 's/.*Version = "\([^"]*\)".*/\1/p' internal/buildinfo/version.go)
[ -n "$version" ] || { echo 'Cannot determine version' >&2; exit 1; }
out="dist/packages/$version"
mkdir -p "$out"
export WOL_PACKAGE_DIR="$PWD/$out" WOL_PACKAGE_VERSION="$version"
for target in linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64; do
 os=${target%/*}; arch=${target#*/}
 if [ "$os" = darwin ] && [ "$host" != Darwin ]; then
  echo 'macOS artifacts require a macOS build host' >&2
  continue
 fi
 make build GOOS="$os" GOARCH="$arch"
 export WOL_PACKAGE_OS="$os" WOL_PACKAGE_ARCH="$arch"
 python3 - <<'PY'
import os,pathlib,tarfile,zipfile
root=pathlib.Path(os.environ['WOL_PACKAGE_DIR'])
platform=os.environ['WOL_PACKAGE_OS']; arch=os.environ['WOL_PACKAGE_ARCH']; version=os.environ['WOL_PACKAGE_VERSION']
name=f'wol_{version}_{platform}_{arch}'
if platform=='windows':
 with zipfile.ZipFile(root/(name+'.zip'),'w',zipfile.ZIP_DEFLATED) as z:
  z.write('dist/wol','wol.exe');z.write('LICENSE','LICENSE');z.write('README.md','README.md')
else:
 with tarfile.open(root/(name+'.tar.gz'),'w:gz') as t:
  t.add('dist/wol',arcname='wol');t.add('LICENSE',arcname='LICENSE');t.add('README.md',arcname='README.md')
PY
done
python3 - <<'PY'
import hashlib,os,pathlib
root=pathlib.Path(os.environ['WOL_PACKAGE_DIR'])
files=sorted([*root.glob('*.tar.gz'),*root.glob('*.zip')])
(root/'SHA256SUMS').write_text(''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in files))
print(root)
PY
make build

