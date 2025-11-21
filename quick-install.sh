#!/bin/bash
# 妙妙屋 - 一键安装命令（简化版）

set -e

VERSION="v6"
GITHUB_REPO="Jimleerx/miaomiaowu"
VERSION_FILE=".version"

# 检测系统架构
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64)
        BINARY_NAME="mmw-linux-amd64"
        ;;
    aarch64|arm64)
        BINARY_NAME="mmw-linux-arm64"
        ;;
    *)
        echo "❌ 不支持的架构: $ARCH"
        echo "支持的架构: x86_64 (amd64), aarch64 (arm64)"
        exit 1
        ;;
esac

DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${BINARY_NAME}"

# 安装函数
install() {
    echo "正在下载并安装妙妙屋 $VERSION ($ARCH)..."

    # 下载
    wget -q --show-progress "$DOWNLOAD_URL" -O mmw

    # 赋予执行权限
    chmod +x mmw

    # 创建数据目录
    mkdir -p data

    # 保存版本信息
    echo "$VERSION" > "$VERSION_FILE"

    # 询问端口号（支持非交互式环境）
    echo ""
    if [ -t 0 ]; then
        # 交互式环境
        read -p "请输入端口（默认8080）: " PORT_INPUT
        PORT=${PORT_INPUT:-8080}
    else
        # 非交互式环境，使用环境变量或默认值
        PORT=${PORT:-8080}
        echo "使用端口: $PORT"
    fi

    # 设置环境变量并运行
    export PORT=$PORT
    nohup ./mmw > mmw.log 2>&1 &

    # 显示完成信息
    echo ""
    echo "✅ 安装完成！"
    echo ""
    echo "访问地址: http://localhost:$PORT"
    echo ""
    echo "更新版本:"
    echo "  curl -sL https://raw.githubusercontent.com/${GITHUB_REPO}/main/quick-install.sh | bash -s update"
    echo ""
}

# 更新函数
update() {
    echo "正在更新妙妙屋 ($ARCH)..."
    echo ""

    # 检查是否已安装
    if [ ! -f "mmw" ]; then
        echo "❌ 未检测到已安装的 mmw，请先运行安装"
        exit 1
    fi

    # 显示当前版本
    if [ -f "$VERSION_FILE" ]; then
        CURRENT_VERSION=$(cat "$VERSION_FILE")
        echo "当前版本: $CURRENT_VERSION"
    fi
    echo "目标版本: $VERSION ($ARCH)"
    echo ""

    # 查找并停止运行中的进程
    if pgrep -f "./mmw" > /dev/null; then
        echo "停止运行中的服务..."
        pkill -f "./mmw" || true
        sleep 2
    fi

    # 备份当前版本
    if [ -f "mmw" ]; then
        echo "备份当前版本..."
        cp mmw mmw.bak
    fi

    # 下载新版本
    echo "下载新版本..."
    wget -q --show-progress "$DOWNLOAD_URL" -O mmw

    # 赋予执行权限
    chmod +x mmw

    # 保存版本信息
    echo "$VERSION" > "$VERSION_FILE"

    # 询问端口号（支持非交互式环境）
    echo ""
    if [ -t 0 ]; then
        # 交互式环境
        read -p "请输入端口（默认8080）: " PORT_INPUT
        PORT=${PORT_INPUT:-8080}
    else
        # 非交互式环境，使用环境变量或默认值
        PORT=${PORT:-8080}
        echo "使用端口: $PORT"
    fi

    # 设置环境变量并运行
    export PORT=$PORT
    nohup ./mmw > mmw.log 2>&1 &

    echo ""
    echo "✅ 更新完成！"
    echo ""
    echo "📦 版本: $VERSION"
    echo "🌐 访问地址: http://localhost:$PORT"
    echo ""
    echo "运行服务:"
    echo "  PORT=$PORT ./mmw"
    echo ""
    echo "后台运行:"
    echo "  PORT=$PORT nohup ./mmw > mmw.log 2>&1 &"
    echo ""
    echo "如遇问题可回滚到备份版本:"
    echo "  mv mmw.bak mmw"
    echo ""
}

# 主函数
main() {
    if [ "$1" = "update" ]; then
        update
    else
        install
    fi
}

# 运行主函数
main "$@"
