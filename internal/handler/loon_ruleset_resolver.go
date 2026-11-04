package handler

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
	"gopkg.in/yaml.v3"
)

// Clash rule-provider → Loon 规则行。
//
// 为什么必须真的抓内容:Loon 的 [Remote Rule] 只认文本规则清单,而 clash 的
// rule-provider 可能是 YAML(payload 列表)或 MRS(二进制)。以前的做法是把
// URL 里的 .mrs 直接改成 .list —— 很多仓库根本没有同名 .list,于是 404;
// 就算地址存在,二进制 MRS 也不是 Loon 能读的格式(用户实报)。
//
// 解析器装在主控而不是转换模块里:模块是纯函数、不联网,而这边本来就有
// SSRF 安全的抓取客户端。模块通过 SetLoonRuleSetResolver 拿到它。

const (
	loonRuleSetFetchTimeout = 8 * time.Second
	loonRuleSetMaxBytes     = 4 << 20
	// 单个规则集最多内联多少行。geosite 级别的大表全塞进订阅会把配置撑到几 MB,
	// 客户端加载会很难受 —— 超过就当解析不了,让这条规则不生效而不是产出巨物。
	loonRuleSetMaxLines = 20000
	loonRuleSetCacheTTL = 30 * time.Minute
	// 抓失败也缓存一小会儿:一个挂掉的规则源不该让每次生成订阅都干等 8 秒。
	loonRuleSetFailTTL = 5 * time.Minute
)

// errRuleSetHTTP 非 200:当作抓不到,交给上层跳过这条规则集。
var errRuleSetHTTP = errors.New("规则集返回非 200")

type loonRuleSetEntry struct {
	lines   []string
	ok      bool
	expires time.Time
}

var (
	loonRuleSetMu    sync.Mutex
	loonRuleSetCache = map[string]loonRuleSetEntry{}
)

// InstallLoonRuleSetResolver 把解析器装进转换模块。main.go 启动时调一次。
func InstallLoonRuleSetResolver() {
	substore.SetLoonRuleSetResolver(resolveLoonRuleSet)
}

func resolveLoonRuleSet(_ string, provider substore.ClashRuleProvider, policy string) ([]string, bool) {
	url := strings.TrimSpace(provider.URL)
	if url == "" {
		return nil, false
	}
	behavior := strings.ToLower(strings.TrimSpace(provider.Behavior))
	if behavior == "" {
		behavior = "classical"
	}
	key := url + "\x00" + behavior + "\x00" + policy

	loonRuleSetMu.Lock()
	if e, ok := loonRuleSetCache[key]; ok && time.Now().Before(e.expires) {
		loonRuleSetMu.Unlock()
		return e.lines, e.ok
	}
	loonRuleSetMu.Unlock()

	lines, ok := fetchAndConvertLoonRuleSet(url, behavior, policy)
	ttl := loonRuleSetCacheTTL
	if !ok {
		ttl = loonRuleSetFailTTL
	}
	loonRuleSetMu.Lock()
	loonRuleSetCache[key] = loonRuleSetEntry{lines: lines, ok: ok, expires: time.Now().Add(ttl)}
	loonRuleSetMu.Unlock()
	return lines, ok
}

func fetchAndConvertLoonRuleSet(url, behavior, policy string) ([]string, bool) {
	// MRS 是二进制,读不了 —— 取同目录的 .yaml。绝大多数发布 .mrs 的仓库都同时发 .yaml,
	// 因为 mihomo 之外的客户端都要用后者。
	candidates := []string{url}
	if strings.HasSuffix(url, ".mrs") {
		candidates = []string{strings.TrimSuffix(url, ".mrs") + ".yaml"}
	}
	for _, u := range candidates {
		body, err := fetchLoonRuleSetBody(u)
		if err != nil {
			continue
		}
		if lines, ok := convertRuleSetPayload(body, behavior, policy); ok {
			return lines, true
		}
	}
	return nil, false
}

func fetchLoonRuleSetBody(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), loonRuleSetFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "miaomiaowu-ruleset/1.0")
	// 地址来自订阅模板(管理员可控),但仍走 SSRF 安全客户端 —— 模板可以被覆写脚本
	// 改写,而主控能摸到的内网不该因此暴露。
	resp, err := newSSRFSafeHTTPClient(loonRuleSetFetchTimeout).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errRuleSetHTTP
	}
	return io.ReadAll(io.LimitReader(resp.Body, loonRuleSetMaxBytes))
}

// convertRuleSetPayload 把 clash 规则集内容转成 Loon 规则行。
//
// 两种载体:YAML(payload 列表)和纯文本清单(每行一条)。行的写法由 behavior 决定。
func convertRuleSetPayload(body []byte, behavior, policy string) ([]string, bool) {
	var entries []string
	// 用指针区分「有 payload 键但为空」和「压根不是 payload 式 YAML」。
	// 用 len()>0 判断的话,`payload: []` 会掉进下面的纯文本分支,把 "payload: []"
	// 这一行当成域名,产出一条垃圾规则(单测抓到的)。
	var doc struct {
		Payload *[]string `yaml:"payload"`
	}
	if err := yaml.Unmarshal(body, &doc); err == nil && doc.Payload != nil {
		entries = *doc.Payload
	} else if looksLikePayloadYAML(body) {
		// 长得像 payload 式 YAML 却解析不了 —— 被截断了,或者上游根本没返回规则
		// (错误页、重定向页)。此时**绝不能**退到纯文本分支:那会把 "payload:"、
		// "  - '+.example.com'" 这些 YAML 骨架当成域名条目,产出
		// `DOMAIN,payload:,策略` 这种垃圾规则塞进用户配置 —— 比这条规则不生效糟得多。
		log.Printf("[Loon规则集] 内容像 YAML 却解析失败(可能被截断或不是规则集),已整条跳过")
		return nil, false
	} else {
		// 纯文本清单:每行一条,# 开头是注释。
		for _, ln := range strings.Split(string(body), "\n") {
			ln = strings.TrimSpace(ln)
			if ln == "" || strings.HasPrefix(ln, "#") {
				continue
			}
			if !looksLikeRuleEntry(ln) {
				// 同理:HTML 错误页 / JSON 报错体逐行喂进来也会产出垃圾规则。
				// 出现一行不像条目就整条放弃,不做「挑能用的」—— 半份规则集比没有更危险。
				log.Printf("[Loon规则集] 纯文本清单里出现不像规则条目的行,已整条跳过: %q", truncateForLog(ln))
				return nil, false
			}
			entries = append(entries, ln)
		}
	}
	if len(entries) == 0 || len(entries) > loonRuleSetMaxLines {
		return nil, false
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if line := ruleEntryToLoon(strings.TrimSpace(e), behavior, policy); line != "" {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// looksLikePayloadYAML 判断响应体是不是 clash 的 payload 式规则集。
// 只看有没有行首的 `payload:` —— 规则集的骨架就这一个键,足够把「截断的 YAML」
// 与「本来就是纯文本清单」区分开。
func looksLikePayloadYAML(body []byte) bool {
	for _, ln := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "payload:") {
			return true
		}
	}
	return false
}

// looksLikeRuleEntry 粗筛纯文本清单的一行是不是规则条目。
//
// 三种合法形态都不含空白与引号:classical 是 `DOMAIN-SUFFIX,example.com`、
// domain 是 `+.example.com`、ipcidr 是 `1.2.3.0/24`。而 YAML 残骸(`- 'x'`)、
// HTML(`<html>`)、JSON(`{"error":…}`)必然踩中这几条。
func looksLikeRuleEntry(ln string) bool {
	if strings.ContainsAny(ln, " \t'\"") {
		return false
	}
	switch ln[0] {
	case '-', '<', '{', '[', '*':
		return false
	}
	return true
}

// truncateForLog 日志里只留前 80 字节,免得一整页 HTML 灌进日志文件。
func truncateForLog(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:80] + "…"
}

// loonRuleOptions 允许跟在策略后面的选项。clash 的 classical 条目里没有策略,
// 所以策略要**插在选项之前**:IP-CIDR,1.2.3.0/24,no-resolve → IP-CIDR,1.2.3.0/24,策略,no-resolve
var loonRuleOptions = map[string]bool{"no-resolve": true, "src": true, "force-remote-dns": true}

func ruleEntryToLoon(entry, behavior, policy string) string {
	if entry == "" || strings.HasPrefix(entry, "#") {
		return ""
	}
	switch behavior {
	case "domain":
		// clash 的 domain 行为:`+.x` 与 `.x` 表示包含子域,`*.x` 是通配。
		// 三者都映射成 DOMAIN-SUFFIX —— 对 `*.x` 略宽(也会命中 x 本身),
		// 但比整条丢掉更接近原意。
		d := entry
		for _, p := range []string{"+.", "*.", "."} {
			if strings.HasPrefix(d, p) {
				return "DOMAIN-SUFFIX," + strings.TrimPrefix(d, p) + "," + policy
			}
		}
		if strings.Contains(d, "*") {
			return "" // 中间带通配的没有对等写法,丢掉好过写错
		}
		return "DOMAIN," + d + "," + policy
	case "ipcidr":
		t := "IP-CIDR"
		if strings.Contains(entry, ":") {
			t = "IP-CIDR6"
		}
		return t + "," + entry + "," + policy
	default: // classical:条目本身就是完整规则,只差策略
		parts := strings.Split(entry, ",")
		if len(parts) < 2 {
			return ""
		}
		last := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
		if loonRuleOptions[last] {
			head := parts[:len(parts)-1]
			return strings.Join(head, ",") + "," + policy + "," + parts[len(parts)-1]
		}
		return entry + "," + policy
	}
}
