# 站点配置说明

## 概述

站点配置系统允许你通过修改 `site.json` 文件来动态配置网站的元数据信息无需手动修改 `index.html`。

## 配置文件

配置文件位于 `miaomiaowu/site.json`，包含以下字段：

```json
{
  "name": "妙妙屋",
  "description": "妙妙屋 Subscribe WebSite By 2ha.me",
  "url": "https://mmwdemo.2ha.me",
  "favicon": "/images/favicon.ico",
  "previewImage": "/images/preview.png",
  "twitterImage": "/safari-pinned-tab.svg",
  "themeColor": "#fff"
}
```

## 配置项说明

| 字段 | 说明 | 示例值 |
|------|------|--------|
| `name` | 网站名称，会显示在浏览器标签、SEO 元数据中 | `"妙妙屋"` |
| `description` | 网站描述，用于 SEO 和社交媒体分享 | `"妙妙屋 Subscribe WebSite By 2ha.me"` |
| `url` | 网站的完整 URL | `"https://mmwdemo.2ha.me"` |
| `favicon` | 网站图标路径 | `"/images/favicon.ico"` |
| `previewImage` | Open Graph 预览图片路径 | `"/images/preview.png"` |
| `twitterImage` | Twitter 卡片图片路径 | `"/safari-pinned-tab.svg"` |
| `themeColor` | 浏览器主题颜色（移动端） | `"#fff"` |

## 使用方法

### 1. 修改配置

编辑 `miaomiaowu/site.json` 文件，修改你需要的配置项。例如：

```json
{
  "name": "我的网站",
  "description": "这是我的个人订阅管理网站",
  "url": "https://example.com",
  "favicon": "/images/favicon.ico",
  "previewImage": "/images/preview.png",
  "twitterImage": "/safari-pinned-tab.svg",
  "themeColor": "#1a1a1a"
}
```

### 2. 构建项目

运行构建命令，配置会自动注入到 `index.html`：

```bash
cd miaomiaowu
npm run build
```

构建完成后，你会看到类似的输出：

```
✅ Site configuration injected successfully!
   Name: 我的网站
   URL: https://example.com
   Description: 这是我的个人订阅管理网站
```

### 3. 验证结果

构建后的 `internal/web/dist/index.html` 会包含你配置的所有信息：

```html
<title>我的网站</title>
<meta name="title" content="我的网站" />
<meta name="description" content="这是我的个人订阅管理网站" />
<meta property="og:url" content="https://example.com" />
<!-- ... 其他元数据 -->
```

## 配置映射

`site.json` 中的配置项会映射到 `index.html` 的以下位置：

| 配置项 | HTML 元素 |
|--------|-----------|
| `name` | `<title>`, `<meta name="title">`, `<meta property="og:title">`, `<meta property="twitter:title">` |
| `description` | `<meta name="description">`, `<meta property="og:description">`, `<meta property="twitter:description">` |
| `url` | `<meta property="og:url">`, `<meta property="twitter:url">` |
| `favicon` | `<link rel="icon">` |
| `previewImage` | `<meta property="og:image">` |
| `twitterImage` | `<meta property="twitter:image">` |
| `themeColor` | `<meta name="theme-color">` |

## 注意事项

1. **路径格式**：图片路径建议使用相对路径（如 `/images/logo.png`），这样在部署时更灵活
2. **URL 格式**：`url` 字段必须是完整的 URL（包含 `https://`）
3. **颜色格式**：`themeColor` 支持 CSS 颜色值（十六进制、RGB、颜色名称等）
4. **构建时机**：配置只在构建时注入，开发模式（`npm run dev`）不会应用配置
5. **仅构建**：如果只想构建不注入配置，可以使用 `npm run build:only`

## 工作原理

1. **构建流程**：`npm run build` 执行时，会先运行 `build:only` 进行标准构建
2. **配置注入**：构建完成后，自动执行 `scripts/inject-site-config.js` 脚本
3. **读取配置**：脚本读取 `site.json` 配置文件
4. **替换内容**：使用正则表达式替换 `index.html` 中的对应内容
5. **写入文件**：将更新后的内容写回 `index.html`

## 脚本位置

配置注入脚本位于：`miaomiaowu/scripts/inject-site-config.js`

如需自定义注入逻辑，可以直接修改此脚本。

## 示例场景

### 场景 1：更换品牌名称

```json
{
  "name": "HuskyCloud",
  "description": "Professional Proxy Service",
  "url": "https://huskycloud.example.com"
}
```

### 场景 2：使用自定义域名

```json
{
  "name": "妙妙屋",
  "description": "妙妙屋订阅管理系统",
  "url": "https://sub.yourdomain.com"
}
```

### 场景 3：深色主题

```json
{
  "name": "Dark Proxy",
  "description": "Dark theme proxy service",
  "url": "https://dark.example.com",
  "themeColor": "#1a1a1a"
}
```

## 常见问题

**Q: 修改 site.json 后需要重启开发服务器吗？**
A: 不需要。配置只在构建时生效，开发模式下直接使用源文件的 `index.html`。

**Q: 如何恢复默认配置？**
A: 将 `site.json` 恢复为初始内容即可。

**Q: 可以添加自定义配置项吗？**
A: 可以。修改 `scripts/inject-site-config.js` 脚本，添加新的替换规则。

**Q: 配置注入失败怎么办？**
A: 检查：1) `site.json` 格式是否正确 2) 文件路径是否存在 3) 是否有文件写入权限

## 相关文件

- `miaomiaowu/site.json` - 站点配置文件
- `miaomiaowu/scripts/inject-site-config.js` - 配置注入脚本
- `miaomiaowu/index.html` - 源模板文件
- `internal/web/dist/index.html` - 构建后的文件（包含注入的配置）
