#!/usr/bin/env bash
set -euo pipefail

REPO="m00nk0d3/grove"
BINARY="grove"
INSTALL_DIR="${GROVE_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${GROVE_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/grove}"
NODE_HOME="$DATA_DIR/node"
RUNTIME_PREFIX="$DATA_DIR/sandcastle"

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux*)  OS="linux"; HERDR_OS="linux" ;;
  Darwin*) OS="darwin"; HERDR_OS="macos" ;;
  *)
    echo "Unsupported OS: $OS"
    echo "Please build from source: https://github.com/$REPO"
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64"; NODE_ARCH="x64"; HERDR_ARCH="x86_64" ;;
  arm64)   ARCH="arm64"; NODE_ARCH="arm64"; HERDR_ARCH="aarch64" ;;
  aarch64) ARCH="arm64"; NODE_ARCH="arm64"; HERDR_ARCH="aarch64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Fetch latest release version unless a pinned version was provided.
if [ -n "${GROVE_VERSION:-}" ]; then
  VERSION="$GROVE_VERSION"
else
  echo "Fetching latest Grove release..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"v([^"]+)".*/\1/')
fi

if [ -z "$VERSION" ]; then
  echo "Failed to fetch latest version. Check your internet connection."
  exit 1
fi

echo "Installing grove v$VERSION for $OS/$ARCH..."

ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="${GROVE_ARCHIVE_URL:-https://github.com/$REPO/releases/download/v${VERSION}/${ARCHIVE}}"
TMP_DIR="$(mktemp -d)"

cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

curl -fsSL "$URL" -o "$TMP_DIR/$ARCHIVE"
tar -xzf "$TMP_DIR/$ARCHIVE" -C "$TMP_DIR"

install_executable() {
  src="$1"
  dest="$2"
  dest_dir="$(dirname "$dest")"
  if [ ! -d "$dest_dir" ]; then
    if [ -w "$(dirname "$dest_dir")" ]; then
      mkdir -p "$dest_dir"
    else
      echo "Creating $dest_dir (may require sudo)..."
      sudo mkdir -p "$dest_dir"
    fi
  fi
  if [ -w "$dest_dir" ]; then
    install -m 755 "$src" "$dest"
  else
    echo "Installing to $dest_dir (may require sudo)..."
    sudo install -m 755 "$src" "$dest"
  fi
}

install_wrapper() {
  name="$1"
  script="$2"
  wrapper="$TMP_DIR/$name"
  cat >"$wrapper" <<EOF
#!/usr/bin/env sh
exec "$NODE_HOME/bin/node" "$RUNTIME_PREFIX/node_modules/@grove/sandcastle-runtime/dist/$script" "\$@"
EOF
  install_executable "$wrapper" "$INSTALL_DIR/$name"
}

install_private_node() {
  if [ -x "$NODE_HOME/bin/node" ]; then
    echo "Using Grove private Node.js: $NODE_HOME/bin/node"
    return
  fi

  echo "Installing private Node.js 22 runtime..."
  node_os="$OS"
  sums="$TMP_DIR/node-shasums.txt"
  curl -fsSL "https://nodejs.org/dist/latest-v22.x/SHASUMS256.txt" -o "$sums"
  node_archive=$(awk -v target="-${node_os}-${NODE_ARCH}.tar.xz" '$2 ~ target "$" { print $2; exit }' "$sums")
  node_checksum=$(awk -v archive="$node_archive" '$2 == archive { print $1; exit }' "$sums")
  if [ -z "$node_archive" ] || [ -z "$node_checksum" ]; then
    echo "Unable to resolve a Node.js runtime for $node_os/$NODE_ARCH."
    exit 1
  fi
  curl -fsSL "https://nodejs.org/dist/latest-v22.x/$node_archive" -o "$TMP_DIR/$node_archive"
  if command -v sha256sum >/dev/null 2>&1; then
    actual_checksum=$(sha256sum "$TMP_DIR/$node_archive" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual_checksum=$(shasum -a 256 "$TMP_DIR/$node_archive" | awk '{print $1}')
  else
    echo "A SHA-256 utility (sha256sum or shasum) is required."
    exit 1
  fi
  if [ "$actual_checksum" != "$node_checksum" ]; then
    echo "Node.js checksum verification failed."
    exit 1
  fi
  tar -xJf "$TMP_DIR/$node_archive" -C "$TMP_DIR"
  extracted="$TMP_DIR/${node_archive%.tar.xz}"
  mkdir -p "$DATA_DIR"
  rm -rf "$NODE_HOME.new"
  mv "$extracted" "$NODE_HOME.new"
  rm -rf "$NODE_HOME"
  mv "$NODE_HOME.new" "$NODE_HOME"
}

install_private_node

if [ ! -d "$TMP_DIR/runtime/sandcastle" ]; then
  echo "Release archive is missing the Grove Sandcastle runtime."
  exit 1
fi
echo "Installing private Grove Sandcastle runtime..."
mkdir -p "$RUNTIME_PREFIX"
# npm treats a package whose version has not changed as already satisfied, so
# an upgrade that keeps the runtime version leaves the previously installed
# files in place. Remove the installed package first, so every run delivers the
# runtime it is installing rather than reporting success over a stale copy.
rm -rf "$RUNTIME_PREFIX/node_modules/@grove/sandcastle-runtime"
PATH="$NODE_HOME/bin:$PATH" "$NODE_HOME/bin/npm" install \
  --prefix "$RUNTIME_PREFIX" \
  --omit=dev \
  --install-links \
  --no-audit \
  --no-fund \
  "$TMP_DIR/runtime/sandcastle"
if [ ! -f "$RUNTIME_PREFIX/node_modules/@grove/sandcastle-runtime/dist/sandcastle.js" ]; then
  echo "Sandcastle runtime installation is incomplete."
  exit 1
fi

if ! command -v herdr >/dev/null 2>&1; then
  echo "Installing private Herdr runtime..."
  herdr_asset="herdr-${HERDR_OS}-${HERDR_ARCH}"
  curl -fsSL "https://github.com/herdrdev/herdr/releases/latest/download/$herdr_asset" \
    -o "$TMP_DIR/herdr"
  mkdir -p "$DATA_DIR/bin"
  install -m 755 "$TMP_DIR/herdr" "$DATA_DIR/bin/herdr"
  "$DATA_DIR/bin/herdr" --version >/dev/null
  herdr_wrapper="$TMP_DIR/herdr-wrapper"
  cat >"$herdr_wrapper" <<EOF
#!/usr/bin/env sh
exec "$DATA_DIR/bin/herdr" "\$@"
EOF
  install_executable "$herdr_wrapper" "$INSTALL_DIR/herdr"
else
  echo "Using existing Herdr: $(command -v herdr)"
fi

install_executable "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
install_wrapper grove-sandcastle sandcastle.js
install_wrapper imp orchestrator.js
install_wrapper review pr-review.js
install_wrapper resolve conflict-resolver.js
install_wrapper ci ci-fix.js
install_wrapper clean cleanup.js
install_wrapper address address-review.js

echo ""
echo "✓ grove v$VERSION installed to $INSTALL_DIR/$BINARY"
echo "✓ Sandcastle runtime installed to $RUNTIME_PREFIX"
echo "✓ Herdr available"
echo ""
echo "Run: grove"
echo "Docs: https://github.com/$REPO"
