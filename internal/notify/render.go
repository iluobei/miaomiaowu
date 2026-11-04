package notify

import "strings"

// 消息正文统一按 Telegram legacy Markdown 写(*粗体*、_斜体_、`代码`、[文字](链接),
// 动态内容可用 \ 转义)。其他渠道在这里转换:
//   - 标准 Markdown(飞书 / 钉钉 / Server酱):*x* → **x**,去掉 \_ \[ 转义
//   - 纯文本(Bark / 邮件):去掉所有标记,链接写成「文字 (链接)」

// mdSpan 词法切分后的一段。
type mdSpan struct {
	kind string // text / bold / italic / code / pre / link
	text string // 已去转义的文字(pre 为原文)
	url  string // kind=link
}

// tokenizeTelegramMarkdown 按 Telegram legacy Markdown 的规则切分。不成对的标记按普通字符处理
// (Telegram 那边会直接 400,但我们这边不该因为一个孤立的 * 丢掉整条消息)。
func tokenizeTelegramMarkdown(s string) []mdSpan {
	var out []mdSpan
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, mdSpan{kind: "text", text: buf.String()})
			buf.Reset()
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && strings.IndexByte("_*`[", s[i+1]) >= 0:
			buf.WriteByte(s[i+1])
			i += 2
		case strings.HasPrefix(s[i:], "```"):
			end := strings.Index(s[i+3:], "```")
			if end < 0 {
				buf.WriteString("```")
				i += 3
				continue
			}
			flush()
			out = append(out, mdSpan{kind: "pre", text: s[i+3 : i+3+end]})
			i += 3 + end + 3
		case c == '`':
			end := strings.IndexByte(s[i+1:], '`')
			if end < 0 {
				buf.WriteByte(c)
				i++
				continue
			}
			flush()
			// 调用方常把转义过的名字放进 `…` 里;代码段里反斜杠是字面字符,还原掉
			out = append(out, mdSpan{kind: "code", text: unescapeTelegram(s[i+1 : i+1+end])})
			i += 1 + end + 1
		case c == '*' || c == '_':
			end := closingMarker(s, i+1, c)
			if end < 0 {
				buf.WriteByte(c)
				i++
				continue
			}
			flush()
			kind := "bold"
			if c == '_' {
				kind = "italic"
			}
			out = append(out, mdSpan{kind: kind, text: unescapeTelegram(s[i+1 : end])})
			i = end + 1
		case c == '[':
			closeText := strings.Index(s[i:], "](")
			if closeText < 0 {
				buf.WriteByte(c)
				i++
				continue
			}
			closeURL := strings.IndexByte(s[i+closeText+2:], ')')
			if closeURL < 0 || strings.ContainsAny(s[i+1:i+closeText], "\n") {
				buf.WriteByte(c)
				i++
				continue
			}
			flush()
			out = append(out, mdSpan{
				kind: "link",
				text: unescapeTelegram(s[i+1 : i+closeText]),
				url:  s[i+closeText+2 : i+closeText+2+closeURL],
			})
			i += closeText + 2 + closeURL + 1
		default:
			buf.WriteByte(c)
			i++
		}
	}
	flush()
	return out
}

// closingMarker 找与 from-1 处的 marker 配对的未转义 marker;跨行不配对(Telegram 的粗体也是行内的,
// 跨行配对容易把两个孤立的 * 之间整段变粗)。
func closingMarker(s string, from int, marker byte) int {
	for j := from; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '\n':
			return -1
		case marker:
			if j == from {
				return -1 // ** 之间没有内容
			}
			return j
		}
	}
	return -1
}

func unescapeTelegram(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(`\_`, "_", `\*`, "*", "\\`", "`", `\[`, "[").Replace(s)
}

// escapeStandardMarkdown 标准 Markdown 里会改变样式的少数字符。下划线不转义:CommonMark 对词内的
// _ 不做强调,钉钉等简化渲染器又可能把 \_ 原样显示 —— 用户名 a_b 最常见,原样输出最稳。
var standardMarkdownEscaper = strings.NewReplacer("*", `\*`, "`", "\\`")

// ToStandardMarkdown 转成标准 Markdown。hardBreaks=true 时每个换行前补两个空格(钉钉 / Server酱
// 的渲染器把单个换行当空格,不补的话多行告警会挤成一段)。
func ToStandardMarkdown(s string, hardBreaks bool) string {
	var b strings.Builder
	for _, sp := range tokenizeTelegramMarkdown(s) {
		switch sp.kind {
		case "text":
			b.WriteString(standardMarkdownEscaper.Replace(sp.text))
		case "bold":
			b.WriteString("**" + standardMarkdownEscaper.Replace(sp.text) + "**")
		case "italic":
			b.WriteString("*" + standardMarkdownEscaper.Replace(sp.text) + "*")
		case "code":
			b.WriteString("`" + sp.text + "`")
		case "pre":
			b.WriteString("\n```\n" + strings.Trim(sp.text, "\n") + "\n```\n")
		case "link":
			b.WriteString("[" + sp.text + "](" + sp.url + ")")
		}
	}
	out := b.String()
	if hardBreaks {
		out = addHardBreaks(out)
	}
	return out
}

// addHardBreaks 代码块外的换行改成 Markdown 硬换行(行尾两个空格);空行保持空行。
func addHardBreaks(s string) string {
	lines := strings.Split(s, "\n")
	inFence := false
	for i := 0; i < len(lines)-1; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			inFence = !inFence
			continue
		}
		if inFence || strings.TrimSpace(lines[i]) == "" || strings.TrimSpace(lines[i+1]) == "" {
			continue
		}
		lines[i] = strings.TrimRight(lines[i], " ") + "  "
	}
	return strings.Join(lines, "\n")
}

// ToPlainText 去掉全部标记,链接写成「文字 (链接)」。
func ToPlainText(s string) string {
	var b strings.Builder
	for _, sp := range tokenizeTelegramMarkdown(s) {
		switch sp.kind {
		case "link":
			if sp.text == sp.url || sp.text == "" {
				b.WriteString(sp.url)
			} else {
				b.WriteString(sp.text + " (" + sp.url + ")")
			}
		default:
			b.WriteString(sp.text)
		}
	}
	return b.String()
}

// splitTitle 标题为空的消息取正文首行当标题,其余当正文,
// 给需要独立标题字段的渠道用(飞书卡片标题、Server酱 / Bark 的 title、邮件主题)。
func splitTitle(m Msg) (title, body string) {
	if strings.TrimSpace(m.Title) != "" {
		return m.Title, m.Body
	}
	first, rest, _ := strings.Cut(strings.TrimLeft(m.Body, "\n"), "\n")
	return strings.TrimSpace(ToPlainText(first)), strings.TrimLeft(rest, "\n")
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
