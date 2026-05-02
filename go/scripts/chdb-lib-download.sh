#!/usr/bin/env bash
set -euo pipefail

out_dir=".chdb-lib"
mkdir -p "$out_dir"

version="$(curl -fsSL https://api.github.com/repos/chdb-io/chdb-core/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')"
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) asset="linux-x86_64-libchdb.tar.gz" ;;
  Linux-aarch64|Linux-arm64) asset="linux-aarch64-libchdb.tar.gz" ;;
  Darwin-arm64) asset="macos-arm64-libchdb.tar.gz" ;;
  Darwin-x86_64) asset="macos-x86_64-libchdb.tar.gz" ;;
  *) echo "unsupported platform: $(uname -s)-$(uname -m)" >&2; exit 1 ;;
esac

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

curl -fsSL -o "$work_dir/libchdb.tar.gz" \
  "https://github.com/chdb-io/chdb-core/releases/download/$version/$asset"

tar -xzf "$work_dir/libchdb.tar.gz" -C "$work_dir"
cp "$work_dir/libchdb.so" "$out_dir/"
cp "$work_dir/chdb.h" "$out_dir/"

cat > "$out_dir/env.sh" <<EOF
export LD_LIBRARY_PATH="$(pwd)/$out_dir:\${LD_LIBRARY_PATH:-}"
export LIBRARY_PATH="$(pwd)/$out_dir:\${LIBRARY_PATH:-}"
EOF

echo "libchdb installed to $out_dir"
echo "To use it in your current shell: source $out_dir/env.sh"
