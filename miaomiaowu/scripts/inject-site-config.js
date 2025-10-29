import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

// 读取站点配置
const siteConfigPath = path.resolve(__dirname, '../site.json')
const siteConfig = JSON.parse(fs.readFileSync(siteConfigPath, 'utf-8'))

// 读取 index.html（构建输出在 ../internal/web/dist）
const indexPath = path.resolve(__dirname, '../../internal/web/dist/index.html')
let html = fs.readFileSync(indexPath, 'utf-8')

// 替换配置
html = html
  // 替换 title
  .replace(/<title>.*?<\/title>/g, `<title>${siteConfig.name}</title>`)
  // 替换 meta title
  .replace(
    /<meta name="title" content=".*?" \/>/g,
    `<meta name="title" content="${siteConfig.name}" />`
  )
  // 替换 meta description
  .replace(
    /<meta\s+name="description"\s+content=".*?"\s*\/>/g,
    `<meta name="description" content="${siteConfig.description}" />`
  )
  // 替换 favicon
  .replace(
    /<link rel="icon" type="image\/x-icon" href=".*?" \/>/g,
    `<link rel="icon" type="image/x-icon" href="${siteConfig.favicon}" />`
  )
  // 替换 Open Graph URL
  .replace(
    /<meta property="og:url" content=".*?" \/>/g,
    `<meta property="og:url" content="${siteConfig.url}" />`
  )
  // 替换 Open Graph title
  .replace(
    /<meta property="og:title" content=".*?" \/>/g,
    `<meta property="og:title" content="${siteConfig.name}" />`
  )
  // 替换 Open Graph description
  .replace(
    /<meta\s+property="og:description"\s+content=".*?"\s*\/>/g,
    `<meta property="og:description" content="${siteConfig.description}" />`
  )
  // 替换 Open Graph image
  .replace(
    /<meta\s+property="og:image"\s+content=".*?"\s*\/>/g,
    `<meta property="og:image" content="${siteConfig.previewImage}" />`
  )
  // 替换 Twitter card
  .replace(
    /<meta property="twitter:card" content=".*?" \/>/g,
    `<meta property="twitter:card" content="${siteConfig.url}${siteConfig.previewImage}" />`
  )
  // 替换 Twitter URL
  .replace(
    /<meta property="twitter:url" content=".*?" \/>/g,
    `<meta property="twitter:url" content="${siteConfig.url}" />`
  )
  // 替换 Twitter title
  .replace(
    /<meta property="twitter:title" content=".*?" \/>/g,
    `<meta property="twitter:title" content="${siteConfig.name}" />`
  )
  // 替换 Twitter description
  .replace(
    /<meta\s+property="twitter:description"\s+content=".*?"\s*\/>/g,
    `<meta property="twitter:description" content="${siteConfig.description}" />`
  )
  // 替换 Twitter image
  .replace(
    /<meta\s+property="twitter:image"\s+content=".*?"\s*\/>/g,
    `<meta property="twitter:image" content="${siteConfig.twitterImage}" />`
  )
  // 替换 theme-color
  .replace(
    /<meta name="theme-color" content=".*?" \/>/g,
    `<meta name="theme-color" content="${siteConfig.themeColor}" />`
  )

// 写回文件
fs.writeFileSync(indexPath, html, 'utf-8')

console.log('✅ Site configuration injected successfully!')
console.log(`   Name: ${siteConfig.name}`)
console.log(`   URL: ${siteConfig.url}`)
console.log(`   Description: ${siteConfig.description}`)
