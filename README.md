# 妙妙屋 - 个人Clash订阅管理系统

一个轻量级、易部署的Clash订阅管理系统，支持 Nezha、DStatus 和 Komari 探针获取流量信息，导入外部机场节点等功能。

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
- 📱 响应式设计 - 不完全适配移动端和桌面端

### 探针支持
- [Nezha](https://github.com/naiba/nezha) 面板
- [DStatus](https://github.com/DokiDoki1103/dstatus) 监控
- [Komari](https://github.com/missuo/komari) 面板

### 体验[Demo](https://mmwdemo.2ha.me)  
账户/密码: test / test123

### [使用帮助](https://mmwdemo.2ha.me/docs)

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
#### ⚠⚠⚠ 注意：0.1.1版本修改了服务名称，无法通过脚本更新，只能重新安装
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

**卸载服务：**
```bash
# 卸载 systemd 服务（保留数据）
curl -sL https://raw.githubusercontent.com/Jimleerx/miaomiaowu/main/install.sh | sudo bash -s uninstall

# 卸载后如需完全清除数据，手动删除数据目录
sudo rm -rf /etc/mmw
```

**简易安装（手动运行）：**
```bash
# 一键下载安装
curl -sL https://raw.githubusercontent.com/Jimleerx/miaomiaowu/main/quick-install.sh | bash

# 运行服务
./mmw
```

**卸载服务：**
```bash
# 卸载 systemd 服务（保留数据）
curl -sL https://raw.githubusercontent.com/Jimleerx/miaomiaowu/main/quick-install.sh | sudo bash -s uninstall

# 卸载后如需完全清除数据，手动删除数据目录
sudo rm -rf ./data ./subscribes ./rule_templates
```

**更新简易安装版本：**
```bash
# 更新到最新版本
curl -sL https://raw.githubusercontent.com/Jimleerx/miaomiaowu/main/quick-install.sh | bash -s update
```

**Windows：**
```powershell
# 从 Releases 页面下载 mmw-windows-amd64.exe
# https://github.com/Jimleerx/miaomiaowu/releases

# 双击运行或在命令行中执行
.\mmw-windows-amd64.exe
```
<details>
<summary>页面截图</summary>

![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/traffic_info.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/subscribe_url.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/probe_datasource.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/subscribe_manage.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/generate_subscribe.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/custom_proxy_group.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/node_manage.png)  
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/user_manage.png)
![image](https://github.com/Jimleerx/miaomiaowu/blob/main/screenshots/system_settings.png)
</details>

### 技术特点
- 🚀 单二进制文件部署，无需外部依赖
- 💾 使用 SQLite 数据库，免维护
- 🔒 JWT 认证，安全可靠
- 📱 响应式设计，支持移动端

## ⚠️ 免责声明

- 本程序仅供学习交流使用，请勿用于非法用途
- 使用本程序需遵守当地法律法规
- 作者不对使用者的任何行为承担责任

## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=Jimleerx/miaomiaowu&type=date&legend=top-left)](https://www.star-history.com/#Jimleerx/miaomiaowu&type=date&legend=top-left)


## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

## 联系方式

- 问题反馈：[GitHub Issues](https://github.com/Jimleerx/miaomiaowu/issues)
- 功能建议：[GitHub Discussions](https://github.com/Jimleerx/miaomiaowu/discussions)
## 更新日志
<details>
<summary>更新日志</summary>

### v0.3.5 (2025-12-26)
- 🌈 增加代理集合支持，须在系统设置开启
- 🌈 增加节点管理的节点排序
- 🌈 增加用户备注
### v0.3.4 (2025-12-24)
- 🌈 增加新旧模板生成订阅开关
- 🛠️ fix:修复tuicv5节点解析错误
- 🛠️ fix:代理节点中有转义字符未转换
### v0.3.3 (2025-12-24)
- 🌈 增加删除重复节点功能
### v0.3.2 (2025-12-23)
- 🌈 增加数据备份及恢复功能
- 🌈 增加图标按钮的提示
- 🌈 增加代理组类型切换功能
- 🌈 生成订阅模板修改为各种转换后端通用模板
- 🌈 优化生成订阅页面节点快速选择逻辑
- 🌈 支持从网页自动升级版本
### v0.3.1 (2025-12-19)
- 🌈 增加从所有代理组移除操作区
- 🌈 增加删除用户的功能
- 🛠️ fix:删除订阅后用户无法绑定新订阅
### v0.3.0 (2025-12-18)
- 🌈 stash订阅不再跳过任何节点，不兼容的格式由stash报错
- 🛠️ fix:订阅链接选择客户端类型后二维码显示错误
- 🛠️ fix:stash不支持mrs格式规则集，替换为yaml格式
- 🛠️ fix:修改订阅时落地节点链式代理失效
### v0.2.9 (2025-12-17)
- 🛠️ fix:hysteria2协议缺少obfs-password参数
- 🛠️ fix:手机端不显示临时订阅按钮
- 🛠️ fix:节点名称空格编码成+号[#31](https://github.com/Jimleerx/miaomiaowu/issues/31)
### v0.2.8 (2025-12-14)
- 🌈 支持导出带规则的stash配置
- 🛠️ fix:ss plugin参数没有解析
### v0.2.7 (2025-12-11)
- 🌈 调整节点列表的分辨率自适应
- 🌈 支持给节点名称添加地区emoji
- 🌈 增加按地区分组节点
- 🌈 统一页面上除Clash文本配置外的emoji图标样式
- 🛠️ fix:节点绑定探针按钮在手机端不显示
### v0.2.6 (2025-12-10)
- 🌈 节点管理-节点列表支持点击任意位置选中
- 🌈 支持外部订阅同步保留name和仅同步已存在节点
- 🌈 增加同步单个外部订阅的功能
- 🌈 增加外部订阅流量显示
- 🌈 同步外部订阅节点支持保留节点与部分更新节点
- 🌈 增加定时同步外部订阅流量信息
- 🛠️ fix:探针报错时获取订阅报错502
### v0.2.5 (2025-12-08)
- 🌈 增加telegram群组链接
- 🌈 增加临时订阅功能，用于机器人测速
- 🛠️ fix:编辑订阅配置里的按钮左对齐还原右对齐
- 🛠️ fix:short-id为空时导出订阅错误
### v0.2.4 (2025-12-05)
- 🌈 支持wireguard协议
- 🌈 获取探针流量增加重试
- 🌈 增加一个DNS类型模板，统一节点选择名称
- 🌈 生成订阅页面节点未被任何代理组使用时自动移除
- 🛠️ fix:解析节点时没有解析udp参数
- 🛠️ fix:开启短链接后还是会请求获取token
### v0.2.3 (2025-12-03)
- 🌈 脚本增加端口号选择与卸载
- 🌈 自定义规则和系统管理移动到菜单栏
- 🌈 增加自定义规则模板，自定义规则操作优化
- 🌈 生成订阅时如果有自定义规则集，保留原规则集而不替换
- 🌈 手机端与平板端适配
- 🌈 移除html5拖动，使用dndkit实现
- 🌈 拖动时增加释放位置指示器
- 🌈 增加外部订阅管理卡片
### v0.2.2 (2025-11-29)
- 🌈 增加短链接功能，防止token泄露
- 🌈 模板增加默认dns配置
- 🌈 重置token后再次获取定义返回假的配置，通过节点name提示token过期
- 🌈 增加手动同步外部订阅按钮[#23](https://github.com/Jimleerx/miaomiaowu/issues/23)
- 🌈 调整自动选择的代理组属性顺序
- 🌈 增加自定义规则同步开关[#23](https://github.com/Jimleerx/miaomiaowu/issues/23)
- 🛠️ fix:修复拖动节点时光标闪烁
- 🛠️ fix:修复一系列yaml操作产生的双引号、属性顺序错误问题
### v0.2.1 (2025-11-28)
- 🌈 规则引用了不存在的代理组时支持替换为任意代理组
- 🛠️ fix:节点列表快速复制节点为uri格式时缺少sni参数
- 🛠️ fix:【BUG】端口配置莫名出现双引号[#22](https://github.com/Jimleerx/miaomiaowu/issues/22)
- 🛠️ fix:处理yaml时没有保持原始格式[#22](https://github.com/Jimleerx/miaomiaowu/issues/22)
### v0.2.0 (2025-11-27)
- 🌈 可用节点支持名称与标签筛选[#21](https://github.com/Jimleerx/miaomiaowu/issues/21)
- 🛠️ fix:订阅管理节点操作后，负载均衡相关参数消失[#22](https://github.com/Jimleerx/miaomiaowu/issues/22) 
### v0.1.9 (2025-11-26)
- 🛠️ fix:调整代理组的节点顺序时不再重新加载整个代理组列表跳回顶部  
- 🛠️ fix:外部订阅节点信息变更一次后丢失外部订阅关联  
- 🛠️ fix:short-id为""时，订阅种显示为空  
- 🛠️ fix:(BUG) 代理组的属性顺序错误[#19](https://github.com/Jimleerx/miaomiaowu/issues/19)  
### v0.1.8 (2025-11-25)
- 🌈 节点批量重命名[#15](https://github.com/Jimleerx/miaomiaowu/issues/15)
- 🛠️ fix:节点删除后订阅里删不全，会留几个没有删掉[#17](https://github.com/Jimleerx/miaomiaowu/issues/17)
- 🛠️ fix:(BUG) 某些情况下Vless节点的Short-id到订阅里会改变成指数[#18](https://github.com/Jimleerx/miaomiaowu/issues/18)
### v0.1.7 (2025-11-24)
- 🛠️ fix:哪吒V0不同版本服务器地址兼容
- 🛠️ fix:节点管理无法解析ssr类型
- 🛠️ fix:导入节点未保存时无法查看配置
### v0.1.6 (2025-11-22)
- 🌈 节点配置支持编辑
- 🌈 节点支持复制为URI格式
- 🌈 支持AnyTls代理
- 🛠️ fix:拖动节点时没有添加到鼠标释放的位置
- 🛠️ fix:转换loon类型时sni取值错误
### v0.1.5 (2025-11-05)
- 🛠️ 修复short-id为数字时getString返回空值
### v0.1.4 (2025-10-30)
- 🌈 代理组支持新增和修改名称
- 🌈 生成订阅支持上传自定义模板
- 🛠️ surge订阅支持dialer-proxy转换underlying-proxy
- 🛠️ 复制订阅失败时更新地址框的地址
- 🛠️ 修复ss的password带:号时解析错误
- 🛠️ 下载订阅文件时仅更新使用到的节点的外部订阅
- 🛠️ 修复编辑节点后配置文件节点属性乱序
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

</details>