#!/bin/bash

# Bandwidth Monitor Quick Start Installer
# Quick install: bash <(curl -fsSL https://raw.githubusercontent.com/ebinon/monitor-bandwidth/main/install.sh)

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print colored output
print_success() {
    echo -e "${GREEN}[OK] $1${NC}"
}

print_error() {
    echo -e "${RED}[ERROR] $1${NC}"
}

print_info() {
    echo -e "${YELLOW}[INFO] $1${NC}"
}

# Print banner
echo ""
echo "========================================"
echo "  Bandwidth Monitor - Quick Installer"
echo "========================================"
echo ""

# Detect OS
OS="$(uname -s)"
case "${OS}" in
    Linux*)     MACHINE=Linux;;
    Darwin*)    MACHINE=Mac;;
    *)          MACHINE="UNKNOWN:${OS}"
esac

if [ "$MACHINE" = "UNKNOWN:${OS}" ]; then
    print_error "Unsupported operating system: $OS"
    print_info "Only Linux and macOS are supported."
    exit 1
fi

print_info "Detected OS: $OS"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    print_info "Go is not installed. Installing Go..."
    
    if [ "$MACHINE" = "Linux" ]; then
        GO_VERSION="1.24.0"
        ARCH=$(uname -m)
        case "$ARCH" in
            x86_64) GO_ARCH="amd64";;
            aarch64) GO_ARCH="arm64";;
            armv7l) GO_ARCH="arm";;
            *) 
                print_error "Unsupported architecture: $ARCH"
                exit 1
                ;;
        esac
        
        GO_TARBALL="go${GO_VERSION}.${OS}-${GO_ARCH}.tar.gz"
        GO_URL="https://golang.org/dl/${GO_TARBALL}"
        
        print_info "Downloading Go ${GO_VERSION} for ${GO_ARCH}..."
        cd /tmp
        if ! curl -fsSL "$GO_URL" -o "$GO_TARBALL"; then
            print_error "Failed to download Go"
            exit 1
        fi
        
        print_info "Extracting Go..."
        sudo tar -C /usr/local -xzf "$GO_TARBALL"
        
        # Add Go to PATH
        if ! grep -q '/usr/local/go/bin' ~/.bashrc 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
            print_info "Go added to PATH in ~/.bashrc"
        fi
        if ! grep -q '/usr/local/go/bin' ~/.zshrc 2>/dev/null; then
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.zshrc
            print_info "Go added to PATH in ~/.zshrc"
        fi
        
        export PATH=$PATH:/usr/local/go/bin
        rm -f "$GO_TARBALL"
        
        print_success "Go ${GO_VERSION} installed successfully!"
    else
        print_error "Please install Go manually on macOS:"
        print_info "  brew install go"
        exit 1
    fi
else
    GO_VERSION=$(go version | awk '{print $3}')
    print_success "Go is installed: $GO_VERSION"
fi

# Check git
if ! command -v git &> /dev/null; then
    print_error "Git is not installed. Please install git first."
    print_info "  Ubuntu/Debian: sudo apt install git"
    print_info "  CentOS/RHEL:   sudo yum install git"
    print_info "  macOS:         brew install git"
    exit 1
fi

# Create temporary directory
TEMP_DIR=$(mktemp -d)
cd "$TEMP_DIR"

print_info "Cloning repository..."
if ! git clone https://github.com/ebinon/monitor-bandwidth.git > /dev/null 2>&1; then
    print_error "Failed to clone repository. Please check your internet connection."
    exit 1
fi

cd monitor-bandwidth

print_info "Building binary..."
if ! go build -o bandwidth-monitor . 2>/dev/null; then
    print_error "Failed to build binary"
    exit 1
fi

# Install binary
if [ -w /usr/local/bin ]; then
    BIN_DIR="/usr/local/bin"
    cp bandwidth-monitor "$BIN_DIR/"
else
    BIN_DIR="$HOME/.local/bin"
    mkdir -p "$BIN_DIR"
    cp bandwidth-monitor "$BIN_DIR/"
    
    # Add to PATH if not already there
    if ! grep -q "$HOME/.local/bin" ~/.bashrc 2>/dev/null; then
        echo 'export PATH=$PATH:$HOME/.local/bin' >> ~/.bashrc
        print_info "Added $BIN_DIR to PATH in ~/.bashrc"
    fi
fi

chmod +x "$BIN_DIR/bandwidth-monitor"

# Cleanup
cd - > /dev/null
rm -rf "$TEMP_DIR"

# Reload shell
export PATH=$PATH:$BIN_DIR

print_success "Installation completed successfully!"
echo ""
echo "========================================"
echo "  Bandwidth Monitor is now installed!"
echo "========================================"
echo ""
echo "Quick Start:"
echo ""
echo "  Add a server:"
echo "    $ bandwidth-monitor add"
echo ""
echo "  Start dashboard:"
echo "    $ bandwidth-monitor web"
echo ""
echo "  View help:"
echo "    $ bandwidth-monitor"
echo ""
echo "Documentation:"
echo "  https://github.com/ebinon/monitor-bandwidth"
echo ""
echo "========================================"