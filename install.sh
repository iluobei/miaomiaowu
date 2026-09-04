#!/bin/bash

# 妙妙屋 - 流量监控管理系统 安装脚本
# 适用于 Debian/Ubuntu Linux 系统

set -e

# 配置
VERSION="v0.8.4"
GITHUB_REPO="iluobei/miaomiaowu"
BINARY_NAME=""  # 将根据架构自动设置
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="mmw"
DATA_DIR="/etc/mmw"
CONFIG_DIR="/etc/mmw"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

echo_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

echo_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查是否为 root 用户
check_root() {
    if [ "$EUID" -ne 0 ]; then
        echo_error "请使用 root 权限运行此脚本"
        echo_info "使用命令: sudo bash install.sh"
        exit 1
    fi
}

# 检查系统架构
check_architecture() {
    ARCH=$(uname -m)
    echo_info "检测到系统架构: $ARCH"

    case "$ARCH" in
        x86_64|amd64)
            BINARY_NAME="mmw-linux-amd64"
            echo_info "使用 AMD64 版本"
            ;;
        aarch64|arm64)
            BINARY_NAME="mmw-linux-arm64"
            echo_info "使用 ARM64 版本"
            ;;
        *)
            echo_error "不支持的架构: $ARCH"
            echo_error "支持的架构: x86_64 (amd64), aarch64 (arm64)"
            exit 1
            ;;
    esac
}

# 安装依赖
install_dependencies() {
    echo_info "检查并安装依赖..."
    apt-get update -qq
    apt-get install -y wget curl systemd >/dev/null 2>&1
}

# 下载二进制文件
download_binary() {
    echo_info "下载 $SERVICE_NAME $VERSION..."
    DOWNLOAD_URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/${BINARY_NAME}"

    cd /tmp
    if wget -q --show-progress "$DOWNLOAD_URL" -O "$BINARY_NAME"; then
        echo_info "下载完成"
    else
        echo_error "下载失败，请检查网络连接或版本号"
        exit 1
    fi
}

# 安装二进制文件
install_binary() {
    echo_info "安装二进制文件..."
    chmod +x "/tmp/$BINARY_NAME"
    mv "/tmp/$BINARY_NAME" "$INSTALL_DIR/$SERVICE_NAME"
    echo_info "已安装到 $INSTALL_DIR/$SERVICE_NAME"
}

# 创建数据目录
create_directories() {
    echo_info "创建数据目录..."
    mkdir -p "$DATA_DIR"
    mkdir -p "$CONFIG_DIR"
    chmod 755 "$DATA_DIR"
    chmod 755 "$CONFIG_DIR"
}

# 创建 systemd 服务
create_systemd_service() {
    echo_info "创建 systemd 服务..."

    # 询问端口号（支持非交互式环境）
    echo ""
    if [ -t 0 ]; then
        # 交互式环境，可以读取用户输入
        read -p "请输入端口号（默认 8080，直接回车使用默认值）: " PORT_INPUT
        if [ -z "$PORT_INPUT" ]; then
            PORT_INPUT=8080
        fi
    else
        # 非交互式环境（如管道），使用默认值
        PORT_INPUT=${PORT:-8080}
        echo_info "使用端口: $PORT_INPUT"
    fi

    cat > /etc/systemd/system/${SERVICE_NAME}.service <<EOF
[Unit]
Description=Traffic Info - 妙妙屋个人订阅管理系统
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$DATA_DIR
ExecStart=$INSTALL_DIR/$SERVICE_NAME
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=$SERVICE_NAME

# 环境变量
Environment="PORT=$PORT_INPUT"
Environment="DATABASE_PATH=$DATA_DIR/traffic.db"
Environment="LOG_LEVEL=info"

# 安全选项
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
    echo_info "systemd 服务已创建（端口: $PORT_INPUT）"
}

# 启动服务
start_service() {
    echo_info "启动服务..."
    systemctl enable ${SERVICE_NAME}.service
    systemctl start ${SERVICE_NAME}.service
    sleep 2

    if systemctl is-active --quiet ${SERVICE_NAME}.service; then
        echo_info "服务启动成功！"
        return 0
    else
        echo_error "服务启动失败"
        return 1
    fi
}

# 显示状态
show_status() {
    # 从 systemd 服务文件中读取端口号
    CONFIGURED_PORT=$(grep "Environment=\"PORT=" /etc/systemd/system/${SERVICE_NAME}.service | sed 's/.*PORT=\([0-9]*\).*/\1/')
    CONFIGURED_PORT=${CONFIGURED_PORT:-8080}

    echo ""
    echo "======================================"
    echo_info "妙妙屋安装完成！"
    echo "======================================"
    echo ""
    echo "📦 安装位置: $INSTALL_DIR/$SERVICE_NAME"
    echo "💾 数据目录: $DATA_DIR"
    echo "🌐 访问地址: http://$(hostname -I | awk '{print $1}'):$CONFIGURED_PORT"
    echo ""
    echo "常用命令:"
    echo "  启动服务: systemctl start $SERVICE_NAME"
    echo "  停止服务: systemctl stop $SERVICE_NAME"
    echo "  重启服务: systemctl restart $SERVICE_NAME"
    echo "  查看状态: systemctl status $SERVICE_NAME"
    echo "  查看日志: journalctl -u $SERVICE_NAME -f"
    echo "  更新版本: curl -sL https://raw.githubusercontent.com/${GITHUB_REPO}/main/install.sh | sudo bash -s update"
    echo "  卸载服务: curl -sL https://raw.githubusercontent.com/${GITHUB_REPO}/main/install.sh | sudo bash -s uninstall"
    echo ""
    echo "⚠️  首次访问需要完成初始化配置"
    echo ""
}

# 更新服务
update_service() {
    echo_info "开始更新妙妙屋..."
    echo ""

    # 检查服务是否已安装
    if [ ! -f "$INSTALL_DIR/$SERVICE_NAME" ]; then
        echo_error "未检测到已安装的服务，请先使用安装模式"
        exit 1
    fi

    # 显示当前版本
    if [ -f "$DATA_DIR/.version" ]; then
        CURRENT_VERSION=$(cat "$DATA_DIR/.version")
        echo_info "当前版本: $CURRENT_VERSION"
    fi
    echo_info "目标版本: $VERSION"
    echo ""

    # 停止服务
    echo_info "停止服务..."
    systemctl stop ${SERVICE_NAME}.service || true

    # 备份当前二进制文件
    if [ -f "$INSTALL_DIR/$SERVICE_NAME" ]; then
        echo_info "备份当前版本..."
        cp "$INSTALL_DIR/$SERVICE_NAME" "$INSTALL_DIR/${SERVICE_NAME}.bak"
    fi

    # 下载并安装新版本
    download_binary
    install_binary

    # 保存版本信息
    echo "$VERSION" > "$DATA_DIR/.version"

    # 询问是否修改端口（支持非交互式环境）
    CURRENT_PORT=$(grep "Environment=\"PORT=" /etc/systemd/system/${SERVICE_NAME}.service 2>/dev/null | sed 's/.*PORT=\([0-9]*\).*/\1/')
    CURRENT_PORT=${CURRENT_PORT:-8080}
    echo ""
    if [ -e /dev/tty ]; then
        # 交互式环境
        read -p "请输入端口号（默认 $CURRENT_PORT，直接回车使用默认值）: " PORT_INPUT
        if [ -z "$PORT_INPUT" ]; then
            PORT_INPUT=$CURRENT_PORT
        fi
    else
        # 非交互式环境，保持当前端口或使用环境变量
        PORT_INPUT=${PORT:-$CURRENT_PORT}
        echo_info "使用端口: $PORT_INPUT"
    fi

    # 更新 systemd 服务文件中的端口
    sed -i "s/Environment=\"PORT=[0-9]*\"/Environment=\"PORT=$PORT_INPUT\"/" /etc/systemd/system/${SERVICE_NAME}.service

    # 重新加载 systemd 配置
    systemctl daemon-reload

    # 启动服务
    if start_service; then
        echo ""
        echo "======================================"
        echo_info "更新完成！"
        echo "======================================"
        echo ""
        echo "📦 版本: $VERSION"
        echo "🌐 访问地址: http://$(hostname -I | awk '{print $1}'):$PORT_INPUT"
        echo ""
        echo "如遇问题可回滚到备份版本:"
        echo "  sudo systemctl stop $SERVICE_NAME"
        echo "  sudo mv $INSTALL_DIR/${SERVICE_NAME}.bak $INSTALL_DIR/$SERVICE_NAME"
        echo "  sudo systemctl start $SERVICE_NAME"
        echo ""
    else
        echo_error "更新后服务启动失败，正在回滚..."
        mv "$INSTALL_DIR/${SERVICE_NAME}.bak" "$INSTALL_DIR/$SERVICE_NAME"
        systemctl start ${SERVICE_NAME}.service
        echo_error "已回滚到之前版本，请查看日志: journalctl -u $SERVICE_NAME -n 50"
        exit 1
    fi
}

# 卸载服务
uninstall_service() {
    echo_info "开始卸载妙妙屋..."
    echo ""

    # 检查服务是否已安装
    if [ ! -f "$INSTALL_DIR/$SERVICE_NAME" ]; then
        echo_error "未检测到已安装的服务"
        exit 1
    fi

    # 显示当前版本
    if [ -f "$DATA_DIR/.version" ]; then
        CURRENT_VERSION=$(cat "$DATA_DIR/.version")
        echo_info "当前版本: $CURRENT_VERSION"
        echo ""
    fi

    # 停止并禁用服务
    echo_info "停止并禁用服务..."
    systemctl stop ${SERVICE_NAME}.service || true
    systemctl disable ${SERVICE_NAME}.service || true
    echo_info "✓ 服务已停止"
    echo ""

    # 询问是否保留配置和数据
    KEEP_DATA=false
    if [ -t 0 ]; then
        # 交互式环境
        echo "是否保留配置和数据？"
        echo "  1) 完全删除（删除所有文件和数据）"
        echo "  2) 保留数据（保留 $DATA_DIR 和 $CONFIG_DIR 目录）"
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
            echo_info "保留数据模式"
        else
            echo_info "完全删除模式"
        fi
    fi
    echo ""

    # 删除 systemd 服务文件
    echo_info "删除 systemd 服务..."
    rm -f /etc/systemd/system/${SERVICE_NAME}.service
    systemctl daemon-reload
    echo_info "✓ systemd 服务已删除"
    echo ""

    # 删除二进制文件
    echo_info "删除程序文件..."
    rm -f "$INSTALL_DIR/$SERVICE_NAME" "$INSTALL_DIR/${SERVICE_NAME}.bak"
    echo_info "✓ 程序文件已删除"
    echo ""

    # 根据选择删除或保留数据
    if [ "$KEEP_DATA" = "false" ]; then
        echo_info "删除数据和配置..."
        rm -rf "$DATA_DIR" "$CONFIG_DIR"
        echo_info "✓ 数据和配置已删除"
        echo ""
        echo "======================================"
        echo_info "卸载完成！所有文件已删除"
        echo "======================================"
    else
        echo_info "保留数据目录: $DATA_DIR"
        echo_info "保留配置目录: $CONFIG_DIR"
        echo ""
        echo "======================================"
        echo_info "卸载完成！配置和数据已保留"
        echo "======================================"
        echo ""
        echo "如需重新安装:"
        echo "  curl -sL https://raw.githubusercontent.com/${GITHUB_REPO}/main/install.sh | sudo bash"
    fi
    echo ""
}

# 主函数
main() {
    # 检查命令行参数
    if [ "$1" = "update" ]; then
        echo_info "进入更新模式..."
        check_root
        check_architecture
        install_dependencies
        update_service
    elif [ "$1" = "uninstall" ]; then
        echo_info "进入卸载模式..."
        check_root
        uninstall_service
    else
        echo_info "开始安装妙妙屋个人Clash订阅管理系统..."
        echo ""

        check_root
        check_architecture
        install_dependencies
        download_binary
        install_binary
        create_directories
        create_systemd_service

        # 保存版本信息
        echo "$VERSION" > "$DATA_DIR/.version"

        if start_service; then
            show_status
        else
            echo_error "安装过程中出现错误，请查看日志: journalctl -u $SERVICE_NAME -n 50"
            exit 1
        fi
    fi
}

# 运行主函数
main "$@"
