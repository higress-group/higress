#  Copyright (c) 2022 Alibaba Group Holding Ltd.

#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at

#       http:www.apache.org/licenses/LICENSE-2.0

#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

#!/usr/bin/env bash

: "${BINARY_NAME:="hgctl"}"
: "${BINARY_NAME_WINDOWS:="hgctl.exe"}"
: "${hgctl_INSTALL_DIR:="/usr/local/bin"}"
: "${hgctl_INSTALL_DIR_WINDOWS:="${USERPROFILE}/hgctl/bin"}"
export VERSION

HAS_CURL="$(type "curl" &>/dev/null && echo true || echo false)"
HAS_WGET="$(type "wget" &>/dev/null && echo true || echo false)"
HAS_GIT="$(type "git" &>/dev/null && echo true || echo false)"
HAS_NODE="$(type "node" &>/dev/null && echo true || echo false)"

# the lowest node version required
REQUIRED_NODE_VERSION="20.18.1"  

# initArch discovers the architecture for this system.
initArch() {
  ARCH=$(uname -m)
  case $ARCH in
  armv5*) ARCH="armv5" ;;
  armv6*) ARCH="armv6" ;;
  armv7*) ARCH="arm" ;;
  aarch64) ARCH="arm64" ;;
  x86) ARCH="386" ;;
  x86_64) ARCH="amd64" ;;
  i686) ARCH="386" ;;
  i386) ARCH="386" ;;
  esac
}

# initOS discovers the operating system for this system.
initOS() {
  OS="$(uname | tr '[:upper:]' '[:lower:]')"

  case "$OS" in
  # Minimalist GNU for Windows
  mingw* | cygwin*) OS='windows' ;;
  esac
}

# runs the given command as root (detects if we are root already)
runAsRoot() {
  if [ $EUID -ne 0 ]; then
    sudo "${@}"
  else
    "${@}"
  fi
}

# verifySupported checks that the os/arch combination is supported for
# binary builds, as well whether or not necessary tools are present.
verifySupported() {
  local supported="darwin-amd64\ndarwin-arm64\nlinux-amd64\nlinux-arm64\nwindows-amd64\nwindows-arm64\n"
  if ! echo "${supported}" | grep -q "${OS}-${ARCH}"; then
    echo "No prebuilt binary for ${OS}-${ARCH}."
    echo "To build from source, go to https://github.com/higress-group/higress"
    exit 1
  fi

  if [ "${HAS_CURL}" != "true" ] && [ "${HAS_WGET}" != "true" ]; then
    echo "Either curl or wget is required"
    exit 1
  fi

  if [ "${HAS_GIT}" != "true" ]; then
    echo "[WARNING] Could not find git. It is required for plugin installation."
  fi

  if [ "${HAS_NODE}" != "true" ]; then
    echo "[ERROR] Could not find node. It is required for hgctl agent support."
    echo "Node.js >= ${REQUIRED_NODE_VERSION} is required."  
    echo "Start to install node..."
    installNode
  else 
    checkNodeVersion
  fi
  
}

checkNodeVersion() {  
  local current_version=$(node -v | sed 's/v//')  
    
  if ! verifyNodeVersion "$current_version" "$REQUIRED_NODE_VERSION"; then  
    echo "[ERROR] Node.js version $current_version is installed, but >= ${REQUIRED_NODE_VERSION} is required."  
    echo "Please upgrade Node.js or install a newer version."  
    echo "Visit: https://nodejs.org/ or use nvm: https://github.com/nvm-sh/nvm"  
    exit 1  
  else  
    echo "[INFO] Node.js version $current_version meets the requirement (>= ${REQUIRED_NODE_VERSION})"  
  fi  
}  

verifyNodeVersion() {  
  local current=$1  
  local required=$2  
    
  local current_major=$(echo "$current" | cut -d. -f1)  
  local current_minor=$(echo "$current" | cut -d. -f2)  
  local current_patch=$(echo "$current" | cut -d. -f3)  
    
  local required_major=$(echo "$required" | cut -d. -f1)  
  local required_minor=$(echo "$required" | cut -d. -f2)  
  local required_patch=$(echo "$required" | cut -d. -f3)  
    
  if [ "$current_major" -gt "$required_major" ]; then  
    return 0  
  elif [ "$current_major" -lt "$required_major" ]; then  
    return 1  
  fi  
    
  if [ "$current_minor" -gt "$required_minor" ]; then  
    return 0  
  elif [ "$current_minor" -lt "$required_minor" ]; then  
    return 1  
  fi  
    
  if [ "$current_patch" -ge "$required_patch" ]; then  
    return 0  
  else  
    return 1  
  fi  
}  
  
installNode() {  
  echo "Installing Node.js ${REQUIRED_NODE_VERSION}..."  
    
  case "$OS" in  
    darwin)  
      installNodeMacOS  
      ;;  
    linux)  
      installNodeLinux  
      ;;  
    windows)  
      installNodeWindows  
      ;;  
    *)  
      echo "[ERROR] Unsupported OS: $OS"  
      echo "Please install Node.js manually from https://nodejs.org/"  
      exit 1  
      ;;  
  esac  
}  
  
installNodeMacOS() {  
  if type "brew" &>/dev/null; then  
    echo "Using Homebrew to install Node.js..."  
    brew install node@20  
  else  
    echo "[ERROR] Homebrew not found. Please install Homebrew first:"  
    echo "  /bin/bash -c \\"\\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\\""  
    echo "Or install Node.js manually from https://nodejs.org/"  
    exit 1  
  fi  
}  
  
installNodeLinux() {  
  echo "Installing Node.js via NodeSource repository..."  
    
  if [ "${HAS_CURL}" == "true" ]; then  
    curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -  
    sudo apt-get install -y nodejs  
  elif [ "${HAS_WGET}" == "true" ]; then  
    wget -qO- https://deb.nodesource.com/setup_20.x | sudo -E bash -  
    sudo apt-get install -y nodejs  
  else  
    echo "[ERROR] Neither curl nor wget found. Cannot install Node.js."  
    echo "Please install Node.js manually from https://nodejs.org/"  
    exit 1  
  fi  
}  
  
installNodeWindows() {  
  echo "[ERROR] Automatic Node.js installation on Windows is not supported."  
  echo "Please download and install Node.js manually from:"  
  echo "  https://nodejs.org/dist/v${REQUIRED_NODE_VERSION}/node-v${REQUIRED_NODE_VERSION}-x64.msi"  
  echo "Or use a package manager like Chocolatey:"  
  echo "  choco install nodejs --version=${REQUIRED_NODE_VERSION}"  
  exit 1  
}
  

# checkDesiredVersion checks if the desired version is available.
checkDesiredVersion() {
  if [ "$VERSION" == "" ]; then
    # Get tag from release URL
    local latest_release_url="https://github.com/higress-group/higress/releases"
    if [ "${HAS_CURL}" == "true" ]; then
      VERSION=$(curl -Ls $latest_release_url | grep 'href="/higress-group/higress/releases/tag/v[0-9]*.[0-9]*.[0-9]*\"' | sed -E 's/.*\/alibaba\/higress\/releases\/tag\/(v[0-9\.]+)".*/\1/g' | head -1)
    elif [ "${HAS_WGET}" == "true" ]; then
      VERSION=$(wget $latest_release_url -O - 2>&1 | grep 'href="/higress-group/higress/releases/tag/v[0-9]*.[0-9]*.[0-9]*\"' | sed -E 's/.*\/alibaba\/higress\/releases\/tag\/(v[0-9\.]+)".*/\1/g' | head -1)
    fi
    
    if [ "$VERSION" == "" ]; then 
      echo "Failed to determine latest version. Please check network or set VERSION manually."
      exit 1
    fi
  fi
}

# checkhgctlInstalledVersion checks which version of hgctl is installed and
# if it needs to be changed.
checkhgctlInstalledVersion() {
  if [[ -f "${hgctl_INSTALL_DIR}/${BINARY_NAME}" ]]; then
    version=$("${hgctl_INSTALL_DIR}/${BINARY_NAME}" version --client | grep -Eo "v[0-9]+\.[0-9]+.*")
    if [[ "$version" == "$VERSION" ]]; then
      echo "hgctl ${version} is already ${VERSION:-latest}"
      return 0
    else
      echo "hgctl ${VERSION} is available. Changing from version ${version}."
      return 1
    fi
  else
    return 1
  fi
}

# downloadFile downloads the latest binary package
# for that binary.
downloadFile() {
  hgctl_DIST="hgctl_${VERSION}_${OS}_${ARCH}.tar.gz"
  if [ "${OS}" == "windows" ]; then
    hgctl_DIST="hgctl_${VERSION}_${OS}_${ARCH}.zip"
  fi
  DOWNLOAD_URL="https://github.com/higress-group/higress/releases/download/$VERSION/$hgctl_DIST"
  hgctl_TMP_ROOT="$(mktemp -dt hgctl-installer-XXXXXX)"
  hgctl_TMP_FILE="$hgctl_TMP_ROOT/$hgctl_DIST"
  echo "Downloading $DOWNLOAD_URL"
  if [ "${HAS_CURL}" == "true" ]; then
    curl -SsL "$DOWNLOAD_URL" -o "$hgctl_TMP_FILE"
  elif [ "${HAS_WGET}" == "true" ]; then
    wget -q -O "$hgctl_TMP_FILE" "$DOWNLOAD_URL"
  fi
}

# installFile installs the hgctl binary.
installFile() {
  hgctl_TMP="$hgctl_TMP_ROOT/$BINARY_NAME"
  mkdir -p "$hgctl_TMP"
  tar xf "$hgctl_TMP_FILE" -C "$hgctl_TMP"
  hgctl_TMP_BIN="$hgctl_TMP/out/${OS}_${ARCH}/hgctl"
  echo "Preparing to install $BINARY_NAME into ${hgctl_INSTALL_DIR}"
  runAsRoot cp "$hgctl_TMP_BIN" "$hgctl_INSTALL_DIR/$BINARY_NAME"
  echo "$BINARY_NAME installed into $hgctl_INSTALL_DIR/$BINARY_NAME"
}

# installFileWindows installs the hgctl binary for windows.
installFileWindows() {
  hgctl_TMP="$hgctl_TMP_ROOT/$BINARY_NAME"
  mkdir -p "$hgctl_TMP"
  unzip "$hgctl_TMP_FILE" -d "$hgctl_TMP"
  hgctl_TMP_BIN="$hgctl_TMP/out/${OS}_${ARCH}/hgctl.exe"
  echo "Preparing to install ${BINARY_NAME} into ${hgctl_INSTALL_DIR_WINDOWS}"
  mkdir -p ${hgctl_INSTALL_DIR_WINDOWS}
  cp "$hgctl_TMP_BIN" "$hgctl_INSTALL_DIR_WINDOWS/$BINARY_NAME_WINDOWS"
  echo "$BINARY_NAME installed into $hgctl_INSTALL_DIR_WINDOWS/$BINARY_NAME_WINDOWS"
}

# fail_trap is executed if an error occurs.
fail_trap() {
  result=$?
  if [ "$result" != "0" ]; then
    if [[ -n "$INPUT_ARGUMENTS" ]]; then
      echo "Failed to install $BINARY_NAME with the arguments provided: $INPUT_ARGUMENTS"
    else
      echo "Failed to install $BINARY_NAME"
    fi
    echo -e "\tFor support, go to https://github.com/higress-group/higress."
  fi
  cleanup
  exit $result
}

# testVersion tests the installed client to make sure it is working.
testVersion() {
  dir="$hgctl_INSTALL_DIR"
  if [ "${OS}" == "windows" ]; then
    dir="$hgctl_INSTALL_DIR_WINDOWS"
  fi
  set +e
  if ! [ "$(command -v $BINARY_NAME)" ]; then
    echo "$BINARY_NAME not found. Is ${dir} on your PATH?"
    exit 1
  fi
  set -e
}

# cleanup temporary files.
cleanup() {
  if [[ -d "${hgctl_TMP_ROOT:-}" ]]; then
    rm -rf "$hgctl_TMP_ROOT"
  fi
}

# Execution

#Stop execution on any error
trap "fail_trap" EXIT
set -e

initArch
initOS
verifySupported
checkDesiredVersion
if ! checkhgctlInstalledVersion; then
  downloadFile
  if [ "${OS}" == "windows" ]; then
    installFileWindows
  else
    installFile
  fi
fi
testVersion
cleanup