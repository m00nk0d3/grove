#!/usr/bin/env bash
set -euo pipefail

ROOT="$(mktemp -d)"
trap 'rm -rf "$ROOT"' EXIT

HOME_DIR="$ROOT/home"
INSTALL_DIR="$ROOT/bin"
DATA_DIR="$ROOT/data"
FIXTURE_DIR="$ROOT/fixture"
FAKE_BIN="$ROOT/fake-bin"
STATE_DIR="$HOME_DIR/.grove"
mkdir -p "$HOME_DIR" "$INSTALL_DIR" "$DATA_DIR/node/bin" "$FIXTURE_DIR" "$FAKE_BIN" "$STATE_DIR"

cat >"$FAKE_BIN/herdr" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
chmod +x "$FAKE_BIN/herdr"

cat >"$DATA_DIR/node/bin/node" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
cat >"$DATA_DIR/node/bin/npm" <<'EOF'
#!/usr/bin/env sh
set -eu
prefix=
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--prefix" ]; then
    prefix="$2"
    shift 2
    continue
  fi
  shift
done
mkdir -p "$prefix/node_modules/@grove/sandcastle-runtime/dist"
cp "$GROVE_TEST_RUNTIME_SOURCE" "$prefix/node_modules/@grove/sandcastle-runtime/dist/sandcastle.js"
EOF
chmod +x "$DATA_DIR/node/bin/node" "$DATA_DIR/node/bin/npm"

printf 'theme = "midnight"\n' >"$STATE_DIR/config.toml"
printf 'sqlite-state\n' >"$STATE_DIR/grove.db"
printf '["workflow-1"]\n' >"$STATE_DIR/dismissed.json"
cp "$STATE_DIR/config.toml" "$ROOT/config.expected"
cp "$STATE_DIR/grove.db" "$ROOT/db.expected"
cp "$STATE_DIR/dismissed.json" "$ROOT/dismissed.expected"

case "$(uname -s)" in
  Linux*) os=linux ;;
  Darwin*) os=darwin ;;
  *) echo "Skipping unsupported installer test OS"; exit 0 ;;
esac
case "$(uname -m)" in
  x86_64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "Skipping unsupported installer test architecture"; exit 0 ;;
esac

archive="grove_1.2.3_${os}_${arch}.tar.gz"
archive_path="$FIXTURE_DIR/$archive"
checksums_path="$FIXTURE_DIR/checksums.txt"
runtime_source="$ROOT/sandcastle.js"

make_fixture() {
  runtime_marker="$1"
  package="$ROOT/package"
  rm -rf "$package"
  mkdir -p "$package/runtime/sandcastle/dist"
  cat >"$package/grove" <<'EOF'
#!/usr/bin/env sh
case "${1:-}" in
  --version|-v)
    echo "grove version 1.2.3"
    ;;
  config)
    if [ "${2:-}" = "validate" ]; then
      grep -q BROKEN "${3:-}" && exit 1
      printf 'validated %s\n' "${3:-}" >>"$GROVE_TEST_LOG"
      exit 0
    fi
    exit 1
    ;;
  *)
    exit 1
    ;;
esac
EOF
  chmod +x "$package/grove"
  printf '%s\n' "$runtime_marker" >"$package/runtime/sandcastle/dist/sandcastle.js"
  printf '{"name":"@grove/sandcastle-runtime","version":"1.2.3"}\n' >"$package/runtime/sandcastle/package.json"
  tar -czf "$archive_path" -C "$package" .
  if command -v sha256sum >/dev/null 2>&1; then
    checksum=$(sha256sum "$archive_path" | awk '{print $1}')
  else
    checksum=$(shasum -a 256 "$archive_path" | awk '{print $1}')
  fi
  printf '%s  %s\n' "$checksum" "$archive" >"$checksums_path"
  cp "$package/runtime/sandcastle/dist/sandcastle.js" "$runtime_source"
}

run_installer() {
  if ! HOME="$HOME_DIR" \
    PATH="$FAKE_BIN:$PATH" \
    GROVE_VERSION=v1.2.3 \
    GROVE_INSTALL_DIR="$INSTALL_DIR" \
    GROVE_DATA_DIR="$DATA_DIR" \
    GROVE_ARCHIVE_URL="$archive_path" \
    GROVE_CHECKSUMS_URL="$checksums_path" \
    GROVE_TEST_RUNTIME_SOURCE="$runtime_source" \
    GROVE_TEST_LOG="$ROOT/validate.log" \
    bash "$(dirname "$0")/install.sh" >"$ROOT/install.log" 2>&1; then
    cat "$ROOT/install.log"
    return 1
  fi
}

make_fixture runtime-v1
run_installer

"$INSTALL_DIR/grove" --version | grep -F "grove version 1.2.3" >/dev/null
for command in grove-sandcastle grove-lab imp agent-flow review resolve ci clean address; do
  test -x "$INSTALL_DIR/$command"
done
grep -F "runtime-v1" "$DATA_DIR/sandcastle/node_modules/@grove/sandcastle-runtime/dist/sandcastle.js" >/dev/null

make_fixture runtime-v2
run_installer
grep -F "runtime-v2" "$DATA_DIR/sandcastle/node_modules/@grove/sandcastle-runtime/dist/sandcastle.js" >/dev/null
test "$(wc -l <"$ROOT/validate.log")" -eq 2

cmp "$ROOT/config.expected" "$STATE_DIR/config.toml"
cmp "$ROOT/db.expected" "$STATE_DIR/grove.db"
cmp "$ROOT/dismissed.expected" "$STATE_DIR/dismissed.json"

printf 'BROKEN\n' >"$STATE_DIR/config.toml"
binary_checksum_before=$(cksum "$INSTALL_DIR/grove")
if run_installer; then
  echo "Installer accepted an incompatible configuration"
  exit 1
fi
test "$binary_checksum_before" = "$(cksum "$INSTALL_DIR/grove")"

echo "Unix installer behavior verified"
