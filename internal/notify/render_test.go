package notify

import "testing"

func TestToStandardMarkdown(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"粗体", "*标题*\n正文", "**标题**\n正文"},
		{"转义的下划线还原", `用户 a\_b 登录`, "用户 a_b 登录"},
		{"转义的星号保持字面", `x a\*b`, `x a\*b`},
		{"代码原样", "节点 `hk_1` 离线", "节点 `hk_1` 离线"},
		{"链接", "[面板](https://a.example/x)", "[面板](https://a.example/x)"},
		{"成对方括号不是链接", "节点 [GOMAMI] 恢复", "节点 [GOMAMI] 恢复"},
		{"孤立星号按字面", "流量 5* 倍", `流量 5\* 倍`},
		{"跨行不配对", "*a\nb*", "\\*a\nb\\*"},
		{"斜体", "_注意_", "*注意*"},
	}
	for _, c := range cases {
		if got := ToStandardMarkdown(c.in, false); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestToStandardMarkdownHardBreaks(t *testing.T) {
	in := "*流量告警*\n节点 `a` 已达 90%\n\n已用: 9GB"
	want := "**流量告警**  \n节点 `a` 已达 90%\n\n已用: 9GB"
	if got := ToStandardMarkdown(in, true); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestToPlainText(t *testing.T) {
	in := "*节点不可用*\n节点 `hk\\_1` 离线\n详情 [打开面板](https://p.example)"
	want := "节点不可用\n节点 hk_1 离线\n详情 打开面板 (https://p.example)"
	if got := ToPlainText(in); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSplitTitle(t *testing.T) {
	// 有标题:原样
	if title, body := splitTitle(Msg{Title: "T", Body: "B"}); title != "T" || body != "B" {
		t.Fatalf("got %q %q", title, body)
	}
	// 标题为空:首行(*📊 每日流量*)当标题
	title, body := splitTitle(Msg{Body: "*📊 每日流量*\n• a: 1GB\n• b: 2GB"})
	if title != "📊 每日流量" || body != "• a: 1GB\n• b: 2GB" {
		t.Fatalf("got %q %q", title, body)
	}
}
