#!/bin/bash

# Check if all required arguments are provided
if [ "$#" -ne 5 ]; then
    echo "Usage: $0 <APP_NAME> <BINARY_NAME> <ICON_PATH> <VERSION> <OUTPUT_DIR>"
    exit 1
fi

APP_NAME="$1"
BINARY_NAME="$2"
ICON_PATH="$3"
VERSION="$4"
OUTPUT_DIR="$5"

# Determine the OS and ARCH
OS=$(go env GOOS)
ARCH=$(go env GOARCH)

# Set the full binary name
FULL_BINARY_NAME="${BINARY_NAME}-${OS}-${ARCH}"

# Create the application bundle structure
mkdir -p "${OUTPUT_DIR}/${APP_NAME}.app/Contents/MacOS"
mkdir -p "${OUTPUT_DIR}/${APP_NAME}.app/Contents/Resources"

# Create a wrapper script to set environment variables, enable logging, and run the binary
cat > "${OUTPUT_DIR}/${APP_NAME}.app/Contents/MacOS/${BINARY_NAME}" << EOF
#!/bin/bash
DIR="\$( cd "\$( dirname "\${BASH_SOURCE[0]}" )" && pwd )"
export ARK_NODE_APP_NAME="${APP_NAME}"
export ARK_NODE_ICON_PATH="\$DIR/../Resources/icon.icns"
export ARK_NODE_PORT=7000
export AUTO_OPEN=true
export ARK_NODE_LOG_LEVEL=5

LOG_FILE="\$HOME/Library/Logs/${APP_NAME}.log"
exec "\$DIR/${FULL_BINARY_NAME}" "\$@" > "\$LOG_FILE" 2>&1
EOF

# Make the wrapper script executable
chmod +x "${OUTPUT_DIR}/${APP_NAME}.app/Contents/MacOS/${BINARY_NAME}"

# Copy the actual binary
cp "${OUTPUT_DIR}/${FULL_BINARY_NAME}" "${OUTPUT_DIR}/${APP_NAME}.app/Contents/MacOS/${FULL_BINARY_NAME}"

# Copy the icon
if [ -f "$ICON_PATH" ]; then
    cp "$ICON_PATH" "${OUTPUT_DIR}/${APP_NAME}.app/Contents/Resources/icon.icns"
else
    echo "Warning: Icon file not found at $ICON_PATH"
fi

# Create the Info.plist file
cat > "${OUTPUT_DIR}/${APP_NAME}.app/Contents/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>${APP_NAME}</string>
    <key>CFBundleExecutable</key>
    <string>${BINARY_NAME}</string>
    <key>CFBundleIconFile</key>
    <string>icon</string>
    <key>CFBundleIdentifier</key>
    <string>com.example.${APP_NAME}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleSignature</key>
    <string>????</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.8.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSSupportsAutomaticGraphicsSwitching</key>
    <true/>
</dict>
</plist>
EOF

echo "Application bundle created: ${OUTPUT_DIR}/${APP_NAME}.app"