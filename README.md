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
- [Komari](https://github.com/komari-monitor/komari) 面板

### 体验[Demo](https://demo.miaomiaowu.net)  
账户/密码: test / test123

### [使用帮助](https://docs.miaomiaowu.net)

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
  ghcr.io/iluobei/miaomiaowu:latest
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
docker pull ghcr.io/iluobei/miaomiaowu:latest
docker stop miaomiaowu && docker rm miaomiaowu
```
然后按照上方命令重新启动服务。

#### Docker Compose 部署

创建 `docker-compose.yml` 文件：

```yaml
version: '3.8'

services:
  miaomiaowu:
    image: ghcr.io/iluobei/miaomiaowu:latest
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
curl -sL https://raw.githubusercontent.com/iluobei/miaomiaowu/main/install.sh | bash
```

安装完成后，服务将自动启动，访问 `http://服务器IP:8080` 即可。

**更新到最新版本：**
```bash
# systemd 服务更新
curl -sL https://raw.githubusercontent.com/iluobei/miaomiaowu/main/install.sh | sudo bash -s update
```

**卸载服务：**
```bash
# 卸载 systemd 服务（保留数据）
curl -sL https://raw.githubusercontent.com/iluobei/miaomiaowu/main/install.sh | sudo bash -s uninstall

# 卸载后如需完全清除数据，手动删除数据目录
sudo rm -rf /etc/mmw
```

**简易安装（手动运行）：**
```bash
# 一键下载安装
curl -sL https://raw.githubusercontent.com/iluobei/miaomiaowu/main/quick-install.sh | bash

# 运行服务
./mmw
```

**卸载服务：**
```bash
# 卸载 systemd 服务（保留数据）
curl -sL https://raw.githubusercontent.com/iluobei/miaomiaowu/main/quick-install.sh | sudo bash -s uninstall

# 卸载后如需完全清除数据，手动删除数据目录
sudo rm -rf ./data ./subscribes ./rule_templates
```

**更新简易安装版本：**
```bash
# 更新到最新版本
curl -sL https://raw.githubusercontent.com/iluobei/miaomiaowu/main/quick-install.sh | bash -s update
```

**Windows：**
```powershell
# 从 Releases 页面下载 mmw-windows-amd64.exe
# https://github.com/iluobei/miaomiaowu/releases

# 双击运行或在命令行中执行
.\mmw-windows-amd64.exe
```
<details>
<summary>页面截图</summary>

![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/traffic_info.png)  
![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/subscribe_url.png)  
![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/probe_datasource.png)  
![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/subscribe_manage.png)  
![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/generate_subscribe.png)  
![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/custom_proxy_group.png)  
![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/node_manage.png)  
![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/user_manage.png)
![image](https://github.com/iluobei/miaomiaowu/blob/main/screenshots/system_settings.png)
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

[![Star History Chart](https://star-history.dera.page/svg?repos=iluobei/miaomiaowu&type=date&legend=top-left)](https://star-history.dera.page/#iluobei/miaomiaowu&type=date&legend=top-left)


## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request！

## 联系方式

- 问题反馈：[GitHub Issues](https://github.com/iluobei/miaomiaowu/issues)
- 功能建议：[GitHub Discussions](https://github.com/iluobei/miaomiaowu/discussions)
## 更新日志
<details>
<summary>更新日志</summary>

### v0.8.3 (2026-07-30)
- perf: virtualize clash config preview on generator page
- feat: generator pagination, live aggregate subscriptions
- ci: add GHCR docker build workflow
- feat: add subscription auto-update and nodes list pagination
- 🌈 增加幻想主题
- 🌈 升级proxyparser支持pcs参数
- perf: virtualize clash config preview on generator page
- feat: generator pagination, live aggregate subscriptions
- ci: add GHCR docker build workflow
- feat: add subscription auto-update and nodes list pagination
- 🛠️ fix:多中转组导致显示溢出
### v0.8.2 (2026-06-26)
- 🛠️ fix:节点被默认禁用
### v0.8.1 (2026-06-26)
- 🛠️ fix:私有网络代理顺序错误
- 🛠️ fix:某些场景误封ip的问题
- 🛠️ fix:Aethersailor删库切换到fork仓库
- 🌈 模板v3增加默认出站配置
- 🛠️ fix:模板v3中转租重复生成
- ♻️ refactor: 节点解析/订阅转换迁移到 proxyparser 共享模块
- 🛠️ fix:中转组删除节点的逻辑处理
### v0.8.0 (2026-06-22)
- 🛠️ fix:中转租重复生成与无法解绑
### v0.7.9 (2026-06-15)
- 🌈 增加妙妙屋X入口按钮
- 🌈 节点管理的链式代理支持代理组配置
### v0.7.8 (2026-06-04)
- 🛠️ fix:stash节点过滤后没有移除代理组依赖
- 🌈 优化中转节点代理组展示
### v0.7.7 (2026-06-01)
- 🌈 订阅节点自动去重兜底
- 🛠️ fix:安全规则无法识别docker与反代环境的真实IP
### v0.7.6 (2026-05-31)
- 🌈 增加自定义安全封禁配置
- 🛠️ fix:模板dns错误配置补丁
- 🛠️ fix:模板dns错误配置补丁
### v0.7.5 (2026-05-28)
- 🌈 支持snell协议与egern reality
- 🛠️ fix:节点测速UI交互BUG
- 🌈 支持真延迟测试
### v0.7.4 (2026-05-26)
- 🌈 导入节点增加跳过证书验证开关
- 🌈 优化测速时间为8秒
- 🌈 增加节点测速功能，使用mihome
- 🛠️ fix:临时订阅被安全拦截
- 🛠️ fix:hy2缺少端口跳跃配置 #86
### v0.7.3 (2026-05-25)
- 🌈 优化短链接编辑
- 🌈 优化短链接编辑
- 🌈 增加外部订阅的节点流量信息拼接
- 🛠️ fix:获取订阅接口越权#89
- 🛠️ fix:clashtoloon规则集转换
### v0.7.2 (2026-05-21)
- 🌈 优化短链接编辑
- 🌈 优化短链接编辑
- 🌈 增加clash-to-loon配置转换#84
- 🌈 loon增加anytls节点转换#85
- 🌈 loon增加anytls节点转换#85
- 🛠️ fix:覆写脚本选择框无法选中
### v0.7.1 (2026-05-15)
- 🛠️ fix:shadowrocket 规则集格式错误
- 🛠️ fix:ss协议密码错误base64
- 🛠️ fix:链式代理丢失配置
- 🛠️ fix:singbox vless encryption去除
### v0.7.0 (2026-05-14)
- 🛠️ fix:ss协议password没有urldecode
- 🌈 更新时增加数据备份
- 🛠️ fix:模板v3全局自定义参数丢失
- 🛠️ fix:singbox自定义脚本参数丢失
- 🌈 订阅支持选择生效的覆写
- 🛠️ fix:订阅管理的节点标签比节点管理少
- 🌈 增加订阅格式JSON、YAML切换开关
- 🌈 增加扁平风格主题
### v0.6.9 (2026-05-11)
- 🛠️ fix:合并规则到覆写设置
- 🌈 增加2FA两步验证
- 🌈 增加2FA两步验证
- 🌈 每日流量推送更加详细
- 🛠️ fix:自定义脚本无法添加元素
- 🌈 优化直接创建的链式代理节点功能
### v0.6.8 (2026-05-10)
- 🌈 优化发布流程
- 🛠️ fix:shadowrocket配置导入失败
- 🌈 增加tg通知#67
- 🛠️ fix:自定义脚本不生效
### v0.6.7 (2026-05-09)
- 🌈 增加clash->shadowrocket客户端类型
- 🌈 增加覆写脚本功能[#33](https://github.com/iluobei/miaomiaowu/issues/33)
- 🛠️ fix:stash dns nameserver-policy 配置错误
### v0.6.6 (2026-05-07)
- 🌈 静态资源使用相对路径支持子路径代理[#78](https://github.com/iluobei/miaomiaowu/issues/78)
- 🌈 支持naive与mieru协议[#76](https://github.com/iluobei/miaomiaowu/issues/76)
- 🛠️ fix:编辑订阅信息溢出屏幕
- 🛠️ fix:hy2 sni和server相同时没有设置sni
- 🛠️ fix:stash dns配置优化
- 🛠️ fix:hy2 password没有urldecode[#77](https://github.com/iluobei/miaomiaowu/issues/77)
### v0.6.5 (2026-04-25)
- 🛠️ fix:节点名称过滤的默认表达式无法删除[#70](https://github.com/iluobei/miaomiaowu/issues/70)
- 🛠️ fix:订阅过期时间丢失
### v0.6.4 (2026-04-19)
- 🌈 新增订阅流量设置
- 🌈 简化探针绑定服务器流量设置
- 🌈 新增订阅流量响应头开关
### v0.6.3 (2026-04-18)
- 🛠️ fix:编辑节点应用并保存没有刷新缓存
- 🛠️ fix:节点管理批量修改名称顺序错误与排序性能优化
- 🛠️ fix:trojan security 默认为none
- 🛠️ fix:dns 修改为 doh
- 🛠️ fix:安装脚本不提示输入端口号
### v0.6.2 (2026-04-10)
- 🌈 绑定探针支持搜索与滚动
- 🌈 编辑代理组节点支持选择分列数量
- 🛠️ fix:trojan tls 总是为true
- 🛠️ fix:移除小火箭vless enc判断
- 🛠️ fix:编辑缓存重复提示
### v0.6.1 (2026-04-01)
- 🌈 增加了一个彩蛋
- 🛠️ fix:loon vless丢失参数
- 🛠️ fix:通用后端模板正则匹配支持GCX参数
- 🛠️ fix:外部订阅节点过滤失效，增加兜底
### v0.6.0 (2026-03-29)
- 🛠️ fix:v2通用后端模板正则匹配节点失效
### v0.5.9 (2026-03-28)
- 🌈 编辑节点与模板增加本地缓存
- 🌈 节点管理增加快速排序模式
- 🌈 优化自定义代理组配置
- 🛠️ fix:模板生成的订阅节点顺序错误
- 🛠️ fix:编辑模板直接关闭提示未保存更改
- 🛠️ fix:编辑模板时手机端显示问题
- 🛠️ fix:tls类型代理默认sni为空时优先取host[#65](https://github.com/iluobei/miaomiaowu/issues/65)
### v0.5.8 (2026-03-23)
- 🌈 模板v3支持yaml变量
- 🌈 节点详情支持yaml展示
- 🌈 模板v3支持icon与hidden配置
- 🛠️ fix:订阅绑定模板v3后规则同步没有禁用
- 🛠️ fix:trojan reality 参数丢失
- 🛠️ fix:外部订阅同步节点时匹配机制完善
### v0.5.7 (2026-03-18)
- 🌈 支持订阅排序
- 🌈 优化模板v3中转代理组选择
- 🌈 优化移动端编辑节点可用节点展示
- 🌈 增强节点标签管理，支持多标签
- 🛠️ fix:兼容老版链式代理处理
- 🛠️ fix:代理组选择中转节点后对代理集合未生效
- 🛠️ fix:短链接开关错误的放在了用户配置，而不是系统配置
- 🛠️ fix:选择中转节点的dialog溢出屏幕外
### v0.5.6 (2026-03-11)
- 🌈 增加上传覆盖订阅与其他非clash订阅配置校验开关
- 🛠️ fix:短链接开关错误的放在了用户配置，而不是系统配置
- 🛠️ fix:普通用户获取订阅时查询不到节点信息
- 🛠️ fix:订阅节点标签过滤不起作用
### v0.5.5 (2026-03-10)
- 🌈 增加订阅安全封禁规则
- 🌈 自定义短链接功能优化
- 🌈 优化节点管理移动端显示
- 🛠️ fix:自定义短链接被静默模式拦截导致拉取订阅失败
- 🛠️ fix:小火箭的vless enc没有根据客户端兼容模式开关处理
- 🛠️ fix:修改模板后没有刷新绑定的配置
- 🛠️ fix:中转代理组配置被错误移除
- 🛠️ fix:选择代理组类型或中转节点后气泡未关闭
- 🛠️ fix:自定义连接重复的问题
- 🛠️ fix:解析ss协议时去除ipv6地址的括号[#60](https://github.com/iluobei/miaomiaowu/issues/60)
### v0.5.4 (2026-03-05)
- 🌈 优化更新日志与debug功能显示
- 🌈 新增设置自定义订阅连接的功能(Beta)
- 🌈 订阅流量和过期信息节点展示开关
- 🌈 导入节点时支持快速选中与节点列表标签联动
- 🌈 v3模板与订阅代理组新增设置中转代理组的功能
- 🛠️ fix:大量修改节点配置时没有删除旧的配置
- 🛠️ fix:修复编辑节点与临时订阅节点顺序错误
- 🛠️ fix:切换代理组类型后气泡没有自动关闭
- 🛠️ fix:ss节点authPart为明文urlencode时解析失败
- 🛠️ fix:使用自定义规则生成订阅时手动分组和地域分组不显示
- 🛠️ fix:修复remnawave永不过期expire值为0导致过期时间错误设置为1970-01-01
### v0.5.3 (2026-02-08)
- 🌈 生成订阅支持从v3模板生成
- 🛠️ fix:订阅绑定v3模板后不显示节点标签
- 🛠️ fix:生成订阅时旧V1模板不可用
- 🛠️ fix:订阅列表显示上传按钮
### v0.5.2 (2026-02-05)
- 🌈 添加模板管理v3 [文档](https://miaomiaowu.net/docs/templatesV3)
- 🌈 增加外部订阅节点名称过滤
- 🛠️ fix:tuic协议skip-cert-verify添加默认值[#56](https://github.com/iluobei/miaomiaowu/issues/56)
- 🛠️ fix:规则引用代理节点时误判
- 🛠️ fix:shadowsocks没有解析udp-over-tcp参数与smux
- 🌈 nameserver-policy下的配置保留引号
### v0.5.1 (2026-01-30)
- 🛠️ fix:开启静默模式后无法访问页面
### v0.5.0 (2026-01-29)
- 🌈 重要安全性优化
- 🌈 导入节点新增Mihomo格式解析，大量节点展示优化
- 🛠️ fix:ip-version参数没有解析
- 🛠️ fix:订阅清除过期时间无效
### v0.4.9 (2026-01-23)
- 🌈 探针支持删除
- 🌈 支持v2ray格式外部订阅导入
- 🌈 增加外部订阅跳过证书验证
- 🛠️ fix:订阅管理的描述与实际不符
- 🛠️ fix:v2ray外部订阅导入缺失流量与同步报错
### v0.4.8 (2026-01-22)
- 🌈 节点列表支持拖动标签排序
- 🌈 增加tcping延迟探测
- 🛠️ fix:base64格式外部订阅导入错误
- 🛠️ fix:loon订阅缺少obfs参数[#51](https://github.com/iluobei/miaomiaowu/issues/51)
### v0.4.7 (2026-01-20)
- 🌈 增加两个防泄漏dns配置模板
- 🌈 支持clashtosurge配置模板转换
- 🛠️ fix:订阅过期默认配置文件错误[#48](https://github.com/iluobei/miaomiaowu/issues/48)
### v0.4.6 (2026-01-17)
- 🛠️ fix代理节点重复时自动去重
- 🛠️ fix:国内服务没有把DIRECT放在第一位
- 🛠️ fix:规则不允许修改类型[#40](https://github.com/iluobei/miaomiaowu/issues/40)
- 🛠️ fix:surge loon 协议sni参数处理
- 🛠️ fix:qx vless协议缺少tls-host参数
- 🛠️ fix:代理组配置校验缺少filter与Include-all
### v0.4.5 (2026-01-17)
- 🌈 增加简单订阅过期功能
- 🛠️ fix:代理集合的节点未更新[#49](https://github.com/iluobei/miaomiaowu/issues/49)
- 🛠️ fix:订阅流量统计错误[#50](https://github.com/iluobei/miaomiaowu/issues/50)
- 🛠️ fix:模板中负载均衡类型的代理组未解析url
- 🛠️ fix:导入外部订阅节点时保存的外部订阅名称没有使用用户输入的标签
- 🛠️ fix:使用旧模板时因为格式检查无法生成
- 🛠️ fix:使用新模板错误提示必须手动分组
### v0.4.4 (2026-01-14)
- 🌈 增加默认pt代理组
- 🛠️ fix:xhttp转换v2ray丢失mode参数
- 🛠️ fix:qx支持reality
- 🛠️ fix:外部订阅为base64编码时导入报错
### v0.4.3 (2026-01-12)
- 🌈 增加简单日志功能，用于用户提供日志
- 🌈 增加客户端兼容模式开关，自动排除客户端不支持的协议
- 🌈 增加ss协议的解析兼容代码，不忽略报错
- 🛠️ fix:生成订阅无法选择规则集
- 🛠️ 尽力修复了一些xhttp配置、订阅配置错误
### v0.4.2 (2026-01-09)
- 🛠️ fix:探针与节点绑定流量统计错误
- 🛠️ 切回旧版shadowrocket.go
### v0.4.1 (2026-01-09)
- 🌈 增加订阅配置节点排序
- 🛠️ fix:v2ray客户端ss缺少混淆参数
- 🛠️ fix:开启探针服务器绑定后丢失流量信息
- 🛠️ fix:wireguard allowed-ips解析错误
### v0.4.0 (2026-01-07)
- 🌈 节点管理支持本地缓存记忆状态
- 🌈 外部订阅地址支持修改
- 🌈 使用虚拟dom减少页面卡顿
- 🌈 合并生成订阅与添加代理组的预置代理组数据，代理组模板支持从github拉取 [proxy-groups.json](https://raw.githubusercontent.com/iluobei/miaomiaowu/refs/heads/main/proxy_groups/proxy-groups.default.json)
- 🌈 添加代理组时支持选择emoji
- 🌈 生成订阅页面节点支持排序
- 🌈 优化代理组节点拖动性能
- 🛠️ fix:wireguard协议public-key参数丢失
- 🛠️ fix:模板里的.*没有处理
- 🛠️ 更新代理组模板
- 🛠️ fix:增加xhttp的mode显示
- 🛠️ fix:深色主题按钮悬停效果不明显
- 🛠️ fix:vless splithttp改回xhttp
- 🛠️ 优化性能
- 🛠️ fix:导入的节点sni未转义
- 🛠️ fix:代理集合引入的已删除节点未从配置移除BUG
- 🛠️ fix:缓存导致节点集合重复添加前缀
- 🛠️ fix:yaml节点的值前导零丢失
- 🛠️ fix:服务器地址为ipv6时emoji添加失败
- 🛠️ fix:外部订阅流量带小数点导致同步失败
### v0.3.9 (2026-01-03)
- 🌈 更新应用时增加使用代理重试更新
- 🌈 优化批量修改节点时的订阅刷新性能
- 🛠️ fix:vless xhttp以mihomo为准,xhttp转换为splithttp
- 🛠️ fix:仅使用外部订阅节点的订阅只统计外部订阅流量
### v0.3.8 (2026-01-02)
- 🛠️ fix:vless xhttp没有解析mode参数
- 🛠️ fix:vless xhttp以mihomo为准,xhttp转换为splithttp
- 🛠️ fix:节点名称以'[开头无法手动分组
- 🛠️ fix:添加代理组时没有校验同名代理组
### v0.3.7 (2026-01-01)
- 🌈 优化代理集合的处理逻辑
- 🛠️ fix:新增节点时节点列表闪一下
- 🛠️ fix:vless xhttp导出格式缺少参数
- 🛠️ fix:手机端节点列表无法滚动
### v0.3.6 (2025-12-29)
- 🛠️ fix:生成订阅选择节点后筛选没起作用
- 🛠️ fix:生成订阅选择节点后筛选没起作用
- 🛠️ fix:妙妙屋处理代理集合时的bug
- 🛠️ fix:节点管理在特定分辨率下无法拖动
- 🛠️ fix:节点名称有特殊字符导致yamlload失败
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
- 🛠️ fix:节点名称空格编码成+号[#31](https://github.com/iluobei/miaomiaowu/issues/31)
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
- 🌈 增加手动同步外部订阅按钮[#23](https://github.com/iluobei/miaomiaowu/issues/23)
- 🌈 调整自动选择的代理组属性顺序
- 🌈 增加自定义规则同步开关[#23](https://github.com/iluobei/miaomiaowu/issues/23)
- 🛠️ fix:修复拖动节点时光标闪烁
- 🛠️ fix:修复一系列yaml操作产生的双引号、属性顺序错误问题
### v0.2.1 (2025-11-28)
- 🌈 规则引用了不存在的代理组时支持替换为任意代理组
- 🛠️ fix:节点列表快速复制节点为uri格式时缺少sni参数
- 🛠️ fix:【BUG】端口配置莫名出现双引号[#22](https://github.com/iluobei/miaomiaowu/issues/22)
- 🛠️ fix:处理yaml时没有保持原始格式[#22](https://github.com/iluobei/miaomiaowu/issues/22)
### v0.2.0 (2025-11-27)
- 🌈 可用节点支持名称与标签筛选[#21](https://github.com/iluobei/miaomiaowu/issues/21)
- 🛠️ fix:订阅管理节点操作后，负载均衡相关参数消失[#22](https://github.com/iluobei/miaomiaowu/issues/22) 
### v0.1.9 (2025-11-26)
- 🛠️ fix:调整代理组的节点顺序时不再重新加载整个代理组列表跳回顶部  
- 🛠️ fix:外部订阅节点信息变更一次后丢失外部订阅关联  
- 🛠️ fix:short-id为""时，订阅种显示为空  
- 🛠️ fix:(BUG) 代理组的属性顺序错误[#19](https://github.com/iluobei/miaomiaowu/issues/19)  
### v0.1.8 (2025-11-25)
- 🌈 节点批量重命名[#15](https://github.com/iluobei/miaomiaowu/issues/15)
- 🛠️ fix:节点删除后订阅里删不全，会留几个没有删掉[#17](https://github.com/iluobei/miaomiaowu/issues/17)
- 🛠️ fix:(BUG) 某些情况下Vless节点的Short-id到订阅里会改变成指数[#18](https://github.com/iluobei/miaomiaowu/issues/18)
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
