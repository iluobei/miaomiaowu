package web

import (
	"strings"
	"testing"
)

func served() string {
	apprMu.RLock()
	defer apprMu.RUnlock()
	return string(servedIndex)
}

// 注入的 CSS 必须原样出现,不能被 HTML 转义。
//
// 首屏注入点是 <style> 元素的内容,不是 HTML 文本 —— 一旦有人给它套上
// html.EscapeString(旁边的 title / 品牌就是那么处理的),`.a > .b` 会变成
// `.a &gt; .b`,所有子选择器和 content 里的尖括号全部静默失效。
func TestSetCustomCSSInjectsVerbatim(t *testing.T) {
	css := `.sidebar > .item { color: #fff } .b::after { content: "<x>" }`
	SetCustomCSS(css)
	t.Cleanup(func() { SetCustomCSS("") })

	out := served()
	if strings.Contains(out, customCSSPlaceholder) {
		t.Fatal("占位符没被替换 —— index.html 里可能漏了注入点")
	}
	if !strings.Contains(out, css) {
		t.Error("CSS 没原样注入(可能被转义了)")
	}
	if strings.Contains(served(), ".leftover") {
		t.Error("不该有残留")
	}
	SetCustomCSS("")
	if strings.Contains(served(), ".sidebar > .item") {
		t.Error("清空后旧 CSS 还在首屏里")
	}
}

// 标签页标题 / favicon / 左上角品牌:设了就替换并转义,清空后回到构建期的内置值。
func TestSetBrandingReplacesTitleIconAndMark(t *testing.T) {
	t.Cleanup(func() {
		SetSiteTitle("")
		SetSiteIcon("")
		SetBrandMark("", "")
	})
	before := served()
	SetSiteTitle(`我的<站>`)
	SetSiteIcon(`/api/branding/icon?v=1&x="y"`)
	SetBrandMark("品牌 & 标题", "/api/branding/logo?v=1")
	out := served()
	if !strings.Contains(out, "<title>我的&lt;站&gt;</title>") {
		t.Error("标题没替换或没转义")
	}
	if strings.Count(out, "<title>") != 1 {
		t.Error("<title> 应只有一个")
	}
	if !strings.Contains(out, `<link rel="icon" href="/api/branding/icon?v=1&amp;x=&#34;y&#34;" />`) {
		t.Error("favicon 没替换或没转义")
	}
	if !strings.Contains(out, `content="品牌 &amp; 标题"`) || !strings.Contains(out, `content="/api/branding/logo?v=1"`) {
		t.Error("左上角品牌 meta 没注入")
	}
	if strings.Contains(out, brandTitlePlaceholder) || strings.Contains(out, brandLogoPlaceholder) {
		t.Error("品牌占位符没被替换")
	}
	SetSiteTitle("")
	SetSiteIcon("")
	SetBrandMark("", "")
	if served() != before {
		t.Error("清空后应回到构建期的内置值")
	}
}
