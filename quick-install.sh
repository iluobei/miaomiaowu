#!/bin/bash
# 妙妙屋 - 一键安装命令（简化版）

set -e

VERSION="v0.8.4"
GITHUB_REPO="iluobei/miaomiaowu"
VERSION_FILE=".version"
PORT_FILE=".port"

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
        read -p "请输入端口号（默认 8080，直接回车使用默认值）: " PORT_INPUT
        if [ -z "$PORT_INPUT" ]; then
            PORT=8080
        else
            PORT=$PORT_INPUT
        fi
    else
        # 非交互式环境，使用环境变量或默认值
        PORT=${PORT:-8080}
        echo "使用端口: $PORT"
    fi

    # 保存端口配置
    echo "$PORT" > "$PORT_FILE"

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
    echo "卸载:"
    echo "  curl -sL https://raw.githubusercontent.com/${GITHUB_REPO}/main/quick-install.sh | bash -s uninstall"
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
    # 尝试读取之前保存的端口号
    SAVED_PORT=""
    if [ -f "$PORT_FILE" ]; then
        SAVED_PORT=$(cat "$PORT_FILE")
    fi

    if [ -t 0 ]; then
        # 交互式环境
        if [ -n "$SAVED_PORT" ]; then
            read -p "请输入端口号（默认 $SAVED_PORT，直接回车使用默认值）: " PORT_INPUT
            if [ -z "$PORT_INPUT" ]; then
                PORT=$SAVED_PORT
            else
                PORT=$PORT_INPUT
            fi
        else
            read -p "请输入端口号（默认 8080，直接回车使用默认值）: " PORT_INPUT
            if [ -z "$PORT_INPUT" ]; then
                PORT=8080
            else
                PORT=$PORT_INPUT
            fi
        fi
    else
        # 非交互式环境，使用环境变量或默认值
        PORT=${PORT:-${SAVED_PORT:-8080}}
        echo "使用端口: $PORT"
    fi

    # 保存端口配置
    echo "$PORT" > "$PORT_FILE"

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

# 卸载函数
uninstall() {
    echo "正在卸载妙妙屋..."
    echo ""

    # 检查是否已安装
    if [ ! -f "mmw" ]; then
        echo "❌ 未检测到已安装的 mmw"
        exit 1
    fi

    # 显示当前版本
    if [ -f "$VERSION_FILE" ]; then
        CURRENT_VERSION=$(cat "$VERSION_FILE")
        echo "当前版本: $CURRENT_VERSION"
        echo ""
    fi

    # 查找并停止运行中的进程
    if pgrep -f "./mmw" > /dev/null; then
        echo "停止运行中的服务..."
        pkill -f "./mmw" || true
        sleep 2
        echo "✓ 服务已停止"
        echo ""
    fi

    # 询问是否保留配置和数据
    KEEP_DATA=false
    if [ -t 0 ]; then
        # 交互式环境
        echo "是否保留配置和数据？"
        echo "  1) 完全删除（删除所有文件和数据）"
        echo "  2) 保留数据（保留 data 目录和订阅文件）"
        read -p "请选择 (1/2，默认 2): " CHOICE

        if [ "$CHOICE" = "1" ]; then
            KEEP_DATA=false
        else
            KEEP_DATA=true
        fi
    else
        # 非交互式环境，检查环境变量
        if [ "$KEEP_DATA" != "false" ]; then
            KEEP_DATA=true
        fi
        if [ "$KEEP_DATA" = "true" ]; then
            echo "保留数据模式"
        else
            echo "完全删除模式"
        fi
    fi
    echo ""

    # 删除主程序和版本文件
    echo "删除程序文件..."
    rm -f mmw mmw.bak "$VERSION_FILE" "$PORT_FILE" mmw.log
    echo "✓ 程序文件已删除"
    echo ""

    # 根据选择删除或保留数据
    if [ "$KEEP_DATA" = "false" ]; then
        echo "删除数据和配置..."
        rm -rf data/ subscribes/
        echo "✓ 数据和配置已删除"
        echo ""
        echo "✅ 卸载完成！所有文件已删除"
    else
        echo "保留数据目录: data/"
        echo "保留订阅目录: subscribes/"
        echo ""
        echo "✅ 卸载完成！配置和数据已保留"
        echo ""
        echo "如需重新安装:"
        echo "  curl -sL https://raw.githubusercontent.com/${GITHUB_REPO}/main/quick-install.sh | bash"
    fi
    echo ""
}

# 主函数
main() {
    if [ "$1" = "update" ]; then
        update
    elif [ "$1" = "uninstall" ]; then
        uninstall
    else
        install
    fi
}

# 运行主函数
main "$@"
