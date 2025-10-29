# 妙妙屋 - 流量监控管理系统

一个轻量级、易部署的服务器流量监控与订阅管理系统，支持 Nezha、DStatus 和 Komari 探针。

## 功能特性

### 核心功能
- 📊 流量监控 - 支持探针服务器与外部订阅流量聚合统计
- 📈 历史流量 - 30 天流量使用趋势图表
- 🔗 订阅链接 - 展示通过订阅管理上传或导入和生成订阅生成的订阅
- 🔗 订阅管理 - 上传猫咪配置文件或从其他订阅url导入生成订阅
- 🎯 生成订阅 - 从导入的节点生成订阅，可视化代理组规则编辑器
- 📦 节点管理 - 导入个人节点或机场节点，支持添加、编辑、删除代理节点
- 🔧 生成订阅 - 自定义规则或使用模板快速生成订阅
- 🎨 代理分组 - 拖拽式代理节点分组配置，支持链式代理
- 👥 用户管理 - 管理员/普通用户角色区分，订阅权限管理
- 🌓 主题切换 - 支持亮色/暗色模式
- 📱 响应式设计 - 适配移动端和桌面端

### 探针支持
- [Nezha](https://github.com/naiba/nezha) 面板
- [DStatus](https://github.com/DokiDoki1103/dstatus) 监控
- [Komari](https://github.com/missuo/komari) 面板
[Demo](https://demo.miaomiaowu.com)  
### 体验demo
账户/密码: test / test123

### [使用帮助](https://mmwdemo.2ha.me/docs)

## 常见问题
### docker 启动报错
failed to initialize traffic repository: enable wal: unable to open database file: out of memory (14)

给映射的目录 chmod -R 777 比如示例的目录
```
chmod -R 777 ./data ./subscribes ./rule_templates
```

## 安装部署

### 方式 1：Docker 部署（推荐）

使用 Docker 是最简单快捷的部署方式，无需配置任何依赖环境。

#### 基础部署

```bash
docker run -d \
  --user root \
  --name miaomiaowu \
  -p 8080:8080 \
  -v $(pwd)/mmw-data:/app/data \
  -v $(pwd)/subscribes:/app/subscribes \
  -v $(pwd)/rule_templates:/app/rule_templates \
  ghcr.io/jimleerx/miaomiaowu:latest
```

参数说明：
- `-p 8080:8080` 将容器端口映射到宿主机，按需调整。
- `-v ./mmw-data:/app/data` 持久化数据库文件，防止容器重建时数据丢失。
- `-v ./subscribes:/app/subscribes` 订阅文件存放目录
- `-v ./rule_templates:/app/rule_templates` 规则模板存放目录
- `-e JWT_SECRET=your-secret` 可选参数，配置token密钥，建议改成随机字符串
- 其他环境变量（如 `LOG_LEVEL`）同下文“环境变量”章节，可通过 `-e` 继续添加。

更新镜像后可执行：
```bash
docker pull ghcr.io/jimleerx/miaomiaowu:latest
docker stop miaomiaowu && docker rm miaomiaowu
```
然后按照上方命令重新启动服务。

#### Docker Compose 部署

创建 `docker-compose.yml` 文件：

```yaml
version: '3.8'

services:
  miaomiaowu:
    image: ghcr.io/jimleerx/miaomiaowu:latest
    container_name: miaomiaowu
    restart: unless-stopped
    user: root
    environment:
      - PORT=8080
      - DATABASE_PATH=/app/data/traffic.db
      - LOG_LEVEL=info

    ports:
      - "8080:8080"

    volumes:
      - ./data:/app/data
      - ./subscribes:/app/subscribes
      - ./rule_templates:/app/rule_templates

    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://localhost:8080/"]
      interval: 30s
      timeout: 3s
      start_period: 5s
      retries: 3

```

参数说明：
- `-p 8080:8080` 将容器端口映射到宿主机，按需调整。
- `-e JWT_SECRET=your-secret` 可选参数，配置token密钥，建议改成随机字符串
- 其他环境变量（如 `LOG_LEVEL`）同下文“环境变量”章节，可通过 `-e` 继续添加。

映射目录说明:
```
volumes:     #这是挂载下面这三个目录到宿主机的，如果你不知道这三个目录是干嘛的，不需要添加
  - ./mmw-data:/app/data #持久化数据库文件，防止容器重建时数据丢失。
  - ./subscribes:/app/subscribes #订阅文件存放目录
  - ./rule_templates:/app/rule_templates #规则模板存放目录
```

启动服务：

```bash
docker-compose up -d
```

查看日志：

```bash
docker-compose logs -f
```

停止服务：

```bash
docker-compose down
```

#### 数据持久化说明

容器使用两个数据卷进行数据持久化：

- `/app/data` - 存储 SQLite 数据库文件
- `/app/subscribes` - 存储订阅配置文件
- `/app/rule_templates` - 存储规则文件模板

**重要提示**：请确保定期备份这两个目录的数据。

### 方式 2：一键安装（Linux）
#### ⚠ 注意：0.1.1版本修改了服务名称，无法通过脚本更新，只能重新安装
#### 先执行以下命令卸载及转移数据
旧服务卸载及备份转移
```
sudo systemctl stop traffic-info
sudo systemctl disable traffic-info
sudo rm -rf /etc/systemd/system/traffic-info.service
sudo rm -f /usr/local/bin/traffic-info
sudo cp -rf /var/lib/traffic-info/* /etc/mmw/
```
**自动安装为 systemd 服务（Debian/Ubuntu）：**
```bash
# 下载并运行安装脚本
curl -sL https://raw.githubusercontent.com/Jimleerx/miaomiaowu/main/install.sh | bash
```

安装完成后，服务将自动启动，访问 `http://服务器IP:8080` 即可。

**更新到最新版本：**
```bash
# systemd 服务更新
curl -sL https://raw.githubusercontent.com/Jimleerx/miaomiaowu/main/install.sh | sudo bash -s update
```

**简易安装（手动运行）：**
```bash
# 一键下载安装
curl -sL https://raw.githubusercontent.com/Jimleerx/miaomiaowu/main/quick-install.sh | bash

# 运行服务
./mmw
```

**更新简易安装版本：**
```bash
# 更新到最新版本
curl -sL https://raw.githubusercontent.com/Jimleerx/miaomiaowu/main/quick-install.sh | bash -s update
```
### 方式 3：二进制文件部署

**Linux：**
```bash
# 下载二进制文件（修改版本号为所需版本）
wget https://github.com/Jimleerx/miaomiaowu/releases/download/v0.0.2/mmw-linux-amd64

# 添加执行权限
chmod +x mmw-linux-amd64

# 运行
./mmw-linux-amd64
```

**Windows：**
```powershell
# 从 Releases 页面下载 mmw-windows-amd64.exe
# https://github.com/Jimleerx/miaomiaowu/releases

# 双击运行或在命令行中执行
.\mmw-windows-amd64.exe
```
### 页面截图
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/traffic_info.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/subscribe_url.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/probe_datasource.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/subscribe_manage.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/generate_subscribe.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/custom_proxy_group.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/node_manage.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/user_manage.png)
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/system_settings.png)
### 技术特点
- 🚀 单二进制文件部署，无需外部依赖
- 💾 使用 SQLite 数据库，免维护
- 🔒 JWT 认证，安全可靠
- 📱 响应式设计，支持移动端

## 快速开始

### 系统要求
- Linux/Windows x86_64
- 无其他依赖

### 安装部署

#### Linux
```bash
# 下载二进制文件
wget https://github.com/Jimleerx/miaomiaowu/releases/latest/download/mmw-linux-amd64

# 添加执行权限
chmod +x mmw-linux-amd64

# 运行
./mmw-linux-amd64
```

#### Windows
```powershell
# 下载 mmw-windows-amd64.exe
# 双击运行或在命令行中执行
.\mmw-windows-amd64.exe
```

### 首次配置

1. 启动程序后，访问 `http://localhost:8080`
2. 首次访问会显示初始化页面
3. 填写管理员账号信息：
   - 用户名
   - 密码
   - 昵称
   - 邮箱（可选）
   - 头像地址（可选）
4. 完成初始化后，使用管理员账号登录

### 探针配置

1. 以管理员身份登录
2. 进入「探针管理」页面
3. 选择探针类型（Nezha/DStatus/Komari）
4. 填写探针地址（如：`https://probe.example.com`）
5. 添加服务器配置：
   - **服务器 ID**：探针中的服务器标识
   - **服务器名称**：显示名称
   - **流量计算方式**：
     - `up` - 仅上行流量
     - `down` - 仅下行流量
     - `both` - 双向流量
   - **月流量限额（GB）**：服务器月流量上限
6. 保存配置

## 配置说明

### 环境变量

```bash
# 服务器端口（默认 8080）
PORT=8080

# 数据库路径（默认 ./data/traffic.db）
DATABASE_PATH=./data/traffic.db

# JWT 密钥（建议自定义）
JWT_SECRET=your-secret-key

# 日志级别（debug/info/warn/error）
LOG_LEVEL=info
```

### 数据库

程序启动时会自动创建 SQLite 数据库，默认路径为 `./data/traffic.db`。数据库包含：
- 用户表
- 探针配置表
- 流量记录表
- 订阅配置表


## 开发指南

### 技术栈

#### 后端
- Go 1.24+
- SQLite (modernc.org/sqlite)
- Gorilla WebSocket
- JWT 认证

#### 前端
- React 19
- TypeScript
- TanStack Router
- TanStack Query
- Tailwind CSS
- Recharts

### 本地开发

#### 后端开发
```bash
# 安装依赖
go mod download

# 运行开发服务器
go run cmd/server/main.go
```

#### 前端开发
```bash
# 进入前端目录
cd miaomiaowu

# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 构建生产版本
npm run build
```

### 构建

#### 构建所有平台
```bash
# 构建前端
cd miaomiaowu && npm run build && cd ..

# 构建 Linux 版本
GOOS=linux GOARCH=amd64 go build -o build/mmw-linux-amd64 ./cmd/server

# 构建 Windows 版本
GOOS=windows GOARCH=amd64 go build -o build/mmw-windows-amd64.exe ./cmd/server
```

### 项目结构

```
miaomiaowutraffic-info/
├── cmd/
│   └── server/          # 服务器入口
│       ├── main.go      # 主程序
│       └── cors.go      # CORS 配置
├── internal/
│   ├── auth/            # 认证模块
│   ├── handler/         # HTTP 处理器
│   ├── storage/         # 数据库操作
│   └── web/             # 嵌入的前端资源
├── miaomiaowu/          # 前端项目
│   ├── src/
│   │   ├── components/  # React 组件
│   │   ├── routes/      # 路由页面
│   │   ├── stores/      # Zustand 状态管理
│   │   └── lib/         # 工具函数
│   └── package.json
├── data/                # 数据目录
│   └── traffic.db       # SQLite 数据库
└── build/               # 构建输出
```

## 安全建议

1. **修改默认密钥**：部署前务必修改 JWT_SECRET
2. **使用 HTTPS**：生产环境建议配置 SSL 证书
3. **定期备份**：定期备份 `data/traffic.db` 数据库文件
4. **防火墙配置**：仅开放必要端口
5. **密码强度**：设置强密码并定期更换

## 常见问题

### 1. 探针连接失败
- 检查探针地址是否正确
- 确认探针服务正常运行
- 检查网络连通性和防火墙设置
- 检查浏览器是否跨域

### 2. 流量数据不更新
- 确认探针配置中的服务器 ID 正确
- 检查探针 WebSocket 连接状态
- 查看服务器日志排查错误

### 3. 订阅链接无法访问
- 确认已配置订阅规则
- 检查规则 YAML 格式是否正确
- 验证客户端类型匹配

### 4. 忘记管理员密码
如果忘记密码，可以删除数据库重新初始化：
```bash
# 备份数据库（可选）
cp data/traffic.db data/traffic.db.bak

# 删除数据库
rm data/traffic.db

# 重启程序，将显示初始化页面
```

### 5. Docker 启动报错 "out of memory(14)"
这个错误通常是由于 SQLite WAL 模式需要 CGO 支持。已在最新版本修复：
- Dockerfile 已启用 CGO 编译
- 添加了必要的运行时库依赖
- 建议使用 docker-compose.yml 部署并正确挂载 `/app/data` 和 `/app/subscribes` 卷

如果仍遇到问题：
```bash
# 确保数据目录有足够的权限和磁盘空间
chmod 755 ./data ./subscribes

# 使用 docker-compose 重新构建
docker-compose build --no-cache
docker-compose up -d
```

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

## 联系方式

- 问题反馈：[GitHub Issues](https://github.com/Jimleerx/miaomiaowu/issues)
- 功能建议：[GitHub Discussions](https://github.com/Jimleerx/miaomiaowu/discussions)
- 🛠️ vless节点转v2ray时servername没有转换成sni
## 更新日志
### v0.1.3 (2025-10-28)
- 🌈 添加使用帮助页面
- 🌈 节点编辑代理组支持拖动排序节点管理和生成订阅支持按标签筛选，支持批量删除节点和更新节点标签
- 🌈 导入节点时支持自定义标签，生成订阅支持标签筛选，现在筛选后默认选中
- 🌈 编辑代理组时增加一个添加到所有代理组的可释放区域
- 🛠️ 修复探针管理类型无法从接口同步
### v0.1.2 (2025-10-27)
- 🌈 添加自定义规则配置
- 🌈 节点编辑代理组支持拖动排序
- 🌈 节点管理支持配置链式代理的节点
- 🌈 使用外部订阅时支持自定义UA
- 😊 顶栏改为flex定位，始终显示在页面上方
### v0.1.1 (2025-10-25)
- 🌈 订阅管理编辑订阅时支持重新分配节点
- 😊 优化节点拖动页面，现在用节点支持整组拖动
### v0.1.0 (2025-10-24)
- 🌈 增加版本号显示与新版本提示角标
- 😊 优化链式代理配置流程，代理组现在也可拖动
### v0.0.9 (2025-10-24)
- 🌈 新增系统设置
- 🌈 增加获取订阅时同步外部订阅节点的功能
- 🌈 增加外部订阅流量汇总
- 🌈 增加节点与探针服务器绑定与开关
### v0.0.8 (2025-10-23)
- 🌗 集成substore订阅转换功能(beta)
- 🌈 readme移除docker的volume配置，防止小白没有权限启动失败
- 🌈 新增arm64架构包
- 🌈 节点分组支持链式代理
- 🌈 支持哪吒V0探针
- 🌈 节点列表支持转换为IP（v4或v6）
- 🌈 节点名称与订阅名称、说明、文件名支持修改
- 🛠️ 添加节点时vless丢失spx参数，hy2丢失sni参数
- 🛠️ 节点分组删除代理组后，rules中依然使用
- 🛠️ 修复docker启动问题

### v0.0.7 (2025-10-21)
- 🎨 新增手动分组功能，支持拖拽式节点分组
- 📦 新增节点管理功能
- 🔧 新增订阅生成器（支持自定义规则和模板）
- 📱 优化移动端响应式布局
- 🚀 前端依赖清理，减小打包体积
- ⭐ 一键安装脚本支持更新

### v0.0.6 (2025-10-20)
- 🎨 支持导入外部clash订阅与上传yaml文件
- 🐛 修复若干 UI 显示问题

### v0.0.5 (2025-10-18)
- 🔐 增强安全性，添加管理员权限控制
- 🎯 优化规则选择器UI
- 📝 改进自定义规则编辑器
- 🐛 修复数据库连接问题

### v0.0.1 (2025-10-15)
- 初始版本发布
- 支持 Nezha/DStatus/Komari 探针
- 流量监控与订阅管理
- 用户权限管理
- 首次启动初始化向导
