# 订阅客户端类型支持

## 概述

订阅接口 `/api/clash/subscribe` 现在支持通过参数 `t` 指定客户端类型，自动将订阅转换为对应客户端的格式。

## 使用方法

### API 参数

- `filename`: 订阅文件名（必需）
- `token`: 用户令牌（必需）
- `t`: 客户端类型（可选），不指定时返回原始 YAML 文件

### 支持的客户端类型

| 参数值 | 客户端名称 | 说明 |
|--------|-----------|------|
| `clash` | Clash | Clash 标准格式（YAML） |
| `clashmeta` | Clash.Meta | Clash.Meta 扩展格式（YAML），支持更多代理类型 |
| `stash` | Stash | Stash 专用格式（YAML） |
| `shadowrocket` | Shadowrocket | Shadowrocket 格式（YAML） |
| `surfboard` | Surfboard | Surfboard 文本格式 |
| `surge` | Surge | Surge 文本格式 |
| `surgemac` | Surge Mac | Surge for macOS 格式（支持 mihomo） |
| `loon` | Loon | Loon 文本格式，支持 9 种代理类型 |
| `qx` | QuantumultX | QuantumultX 文本格式 |
| `egern` | Egern | Egern YAML 格式，支持 Shadow-TLS 和 Reality |
| `sing-box` | sing-box | sing-box JSON 格式，支持 13 种代理类型 |
| `v2ray` | V2Ray | V2Ray 订阅格式（Base64） |
| `uri` | URI | 标准 URI 格式 |

### 示例

```bash
# 获取 Clash 格式订阅
https://your-domain.com/api/clash/subscribe?filename=subscribe.yaml&token=YOUR_TOKEN&t=clash

# 获取 Surge 格式订阅
https://your-domain.com/api/clash/subscribe?filename=subscribe.yaml&token=YOUR_TOKEN&t=surge

# 获取 V2Ray 格式订阅（Base64）
https://your-domain.com/api/clash/subscribe?filename=subscribe.yaml&token=YOUR_TOKEN&t=v2ray
```

## 前端界面

订阅页面的"复制"按钮已改为下拉菜单，支持选择不同的客户端类型：

1. 点击"复制"按钮旁边的下拉箭头
2. 选择目标客户端类型
3. 对应格式的订阅链接将被复制到剪贴板

每个客户端类型都有对应的图标和名称显示，方便识别。

## 技术实现

### 后端

- **文件**: `internal/handler/subscription.go`
- **核心函数**: `convertSubscription()`
- **工作流程**:
  1. 读取原始 YAML 订阅文件
  2. 解析 `proxies` 节点
  3. 调用对应客户端的 Producer
  4. 返回转换后的格式

### 前端

- **文件**: `miaomiaowu/src/routes/subscription.index.tsx`
- **关键组件**:
  - `CLIENT_TYPES`: 客户端类型配置（图标、名称）
  - `DropdownMenu`: 客户端选择下拉菜单
  - `buildSubscriptionURL()`: 构建带客户端类型的订阅链接

## 转换库

使用了从 TypeScript 转换的 Go 实现订阅转换库：

- **位置**: `internal/substore/`
- **已转换的 Producer**:
  - clash.go - Clash 标准格式
  - clashmeta.go - Clash.Meta 扩展格式
  - stash.go - Stash 格式
  - shadowrocket.go - Shadowrocket 格式
  - surfboard.go - Surfboard 文本格式
  - surge.go - Surge 文本格式
  - surgemac.go - Surge for macOS 格式
  - loon.go - Loon 文本格式
  - qx.go - QuantumultX 文本格式
  - egern.go - Egern YAML 格式
  - singbox.go - sing-box JSON 格式
  - v2ray.go - V2Ray Base64 格式
  - uri.go - URI 格式

## 测试

运行测试验证转换功能：

```bash
# 测试订阅转换
go test ./internal/handler -v -run TestConvertSubscription

# 测试特定 Producer
go test ./internal/substore -v -run TestClashProducer
```

## 注意事项

1. **兼容性**: 不同客户端支持的代理类型和参数可能不同，转换时会自动过滤或调整不支持的选项
2. **性能**: 首次转换可能需要几毫秒，建议客户端缓存订阅结果
3. **错误处理**: 如果指定的客户端类型不存在，API 会返回 400 错误
4. **默认行为**: 不指定 `t` 参数时，返回原始 YAML 文件（向后兼容）
