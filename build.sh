#!/bin/bash
# 前后端统一打包脚本 (Linux/Mac)
set -e

echo "========================================"
echo "开始构建前后端项目"
echo "========================================"

# 设置变量
BUILD_DIR="build"
FRONTEND_DIR="miaomiaowu"
OUTPUT_DIR="${BUILD_DIR}/release"

# 0. 同步版本号
echo ""
echo "[0/3] 同步版本号..."
bash scripts/sync-version.sh
echo "版本号同步完成 ✓"

# 清理旧的构建目录
if [ -d "$BUILD_DIR" ]; then
    echo "清理旧的构建文件..."
    rm -rf "$BUILD_DIR"
fi

# 1. 构建前端
echo ""
echo "[1/3] 构建前端项目..."
cd "$FRONTEND_DIR"
if [ ! -d "node_modules" ]; then
    echo "安装前端依赖..."
    npm install
fi

echo "编译前端代码..."
npm run build
cd ..
echo "前端构建完成 ✓"

# 2. 构建 Go 后端 (Linux)
echo ""
echo "[2/3] 构建 Linux 版本后端..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o "${BUILD_DIR}/mmw-linux-amd64" cmd/server/main.go cmd/server/cors.go
echo "Linux 后端构建完成 ✓"

# 3. 构建 Go 后端 (Windows)
echo ""
echo "[3/3] 构建 Windows 版本后端..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o "${BUILD_DIR}/mmw-windows-amd64.exe" cmd/server/main.go cmd/server/cors.go
echo "Windows 后端构建完成 ✓"

# 4. 准备发布文件
echo ""
echo "准备发布文件..."
mkdir -p "${OUTPUT_DIR}/linux"
mkdir -p "${OUTPUT_DIR}/windows"
mkdir -p "${BUILD_DIR}/data"
mkdir -p "${BUILD_DIR}/subscribes"

# 复制 Linux 版本到 release 目录
cp "${BUILD_DIR}/mmw-linux-amd64" "${OUTPUT_DIR}/linux/"
chmod +x "${OUTPUT_DIR}/linux/mmw-linux-amd64"
if [ -d "data" ]; then
    cp -r "data" "${OUTPUT_DIR}/linux/"
fi
if [ -d "subscribes" ]; then
    cp -r "subscribes" "${OUTPUT_DIR}/linux/"
fi
if [ -d "config" ]; then
    cp -r "config" "${OUTPUT_DIR}/linux/"
fi

# 复制 Windows 版本到 release 目录
cp "${BUILD_DIR}/mmw-windows-amd64.exe" "${OUTPUT_DIR}/windows/"
if [ -d "data" ]; then
    cp -r "data" "${OUTPUT_DIR}/windows/"
fi
if [ -d "subscribes" ]; then
    cp -r "subscribes" "${OUTPUT_DIR}/windows/"
fi
if [ -d "config" ]; then
    cp -r "config" "${OUTPUT_DIR}/windows/"
fi

# 复制必要的配置文件到 build 根目录
if [ -d "data" ]; then
    cp -r "data" "${BUILD_DIR}/"
fi
if [ -d "subscribes" ]; then
    cp -r "subscribes" "${BUILD_DIR}/"
fi

echo ""
echo "========================================"
echo "构建完成！"
echo "========================================"
echo ""
echo "输出文件:"
echo "  - Linux:   ${BUILD_DIR}/mmw-linux-amd64"
echo "  - Windows: ${BUILD_DIR}/mmw-windows-amd64.exe"
echo "  - Release: ${OUTPUT_DIR}/"
echo ""
