#!/bin/bash

input_file="$1"
output_dir="$2"

# Check if input file is provided
if [ -z "$input_file" ] || [ -z "$output_dir" ]; then
    echo "Usage: $0 <input_file> <output_dir>"
    exit 1
fi

output_name="${output_dir}/icon.icns"

# Create iconset directory
mkdir -p "${output_dir}/icon.iconset"

# Generate icns files
sips -z 16 16     "${input_file}" --out "${output_dir}/icon.iconset/icon_16x16.png"
sips -z 32 32     "${input_file}" --out "${output_dir}/icon.iconset/icon_16x16@2x.png"
sips -z 32 32     "${input_file}" --out "${output_dir}/icon.iconset/icon_32x32.png"
sips -z 64 64     "${input_file}" --out "${output_dir}/icon.iconset/icon_32x32@2x.png"
sips -z 128 128   "${input_file}" --out "${output_dir}/icon.iconset/icon_128x128.png"
sips -z 256 256   "${input_file}" --out "${output_dir}/icon.iconset/icon_128x128@2x.png"
sips -z 256 256   "${input_file}" --out "${output_dir}/icon.iconset/icon_256x256.png"
sips -z 512 512   "${input_file}" --out "${output_dir}/icon.iconset/icon_256x256@2x.png"
sips -z 512 512   "${input_file}" --out "${output_dir}/icon.iconset/icon_512x512.png"

# If the input file is big enough, use it for the @2x version, otherwise scale it up
if [ $(sips -g pixelHeight "${input_file}" | awk '/pixelHeight:/ {print $2}') -ge 1024 ]; then
    cp "${input_file}" "${output_dir}/icon.iconset/icon_512x512@2x.png"
else
    sips -z 1024 1024  "${input_file}" --out "${output_dir}/icon.iconset/icon_512x512@2x.png"
fi

# Create icns file
iconutil -c icns "${output_dir}/icon.iconset" -o "${output_name}"

# Clean up
rm -R "${output_dir}/icon.iconset"

echo "ICNS file created: ${output_name}"