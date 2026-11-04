package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"miaomiaowu/internal/logger"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
)

// sing-box 模板。
//
// 与 Surge / Loon / QX 模板同一个思路(模板决定分组和规则,后端只管把节点放进去),
// 但载体不同:模板是一份**完整的 sing-box 配置(JSON)**,扩展名 .json。
//
// 模板按 **sing-box 1.14** 的配置格式来(用户拍板以最新版为准):内置模板用了 1.14 才有的
// http_clients / route.default_http_client,1.13 及以下加载不了。
//
// 没有模板时,sing-box 订阅只有 {"outbounds": [...], "endpoints": [...]} 两段 —— 没有入站、
// 没有路由,官方客户端拿它当远程配置是用不了的。套上模板后下发的就是一份能直接跑的配置。
//
// 模板里的分组(selector / urltest)用占位符表示「这里放节点」,写法沿用社区里用得最多的
// sing-box-subscribe 模板,现成的模板拿来就能用:
//
//	{ "type": "selector", "tag": "🇭🇰 香港", "outbounds": ["{all}"],
//	  "filter": [ { "action": "include", "keywords": ["🇭🇰|HK|香港"] } ] }
//
//   - "{all}":全部节点(别的 {xxx} 占位符在那个工具里指某一个订阅,这里只有一个节点池,一律当 {all});
//     分组里没写占位符时,等于在成员末尾写了一个 {all} —— 每个分组都会放进节点;
//     不想要的分组写 "include_all": false(写了 {all} 的照样展开);
//   - filter:按顺序执行,include 只留名字匹配的,exclude 去掉匹配的;keywords 是正则,多条之间是「或」。
//     filter、include_all 都不是 sing-box 的字段,输出时去掉。
//
// 筛完一个节点都不剩的分组不删(路由规则还引用着它,删了整份配置加载不了),改成只含直连。

// isSingboxTemplateFile 按扩展名识别 sing-box 模板。
func isSingboxTemplateFile(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), ".json")
}

// isSingboxTemplateClientType 判断这次请求能不能套 sing-box 模板。
func isSingboxTemplateClientType(clientType string) bool {
	return strings.EqualFold(strings.TrimSpace(clientType), "sing-box")
}

// looksLikeSingboxTemplate 按内容识别(预览接口只拿得到内容,拿不到文件名)。
// JSON 写的 Clash 配置也以 { 开头,所以还要看有没有 outbounds、没有 proxy-groups。
func looksLikeSingboxTemplate(content string) bool {
	if !strings.HasPrefix(strings.TrimSpace(string(stripJSONComments([]byte(content)))), "{") {
		return false
	}
	root, err := parseSingboxTemplate(content)
	if err != nil {
		return false
	}
	_, hasOutbounds := root.vals["outbounds"]
	_, hasGroups := root.vals["proxy-groups"]
	return hasOutbounds && !hasGroups
}

// stripJSONComments 去掉 // 与 /* */ 注释,以及对象 / 数组末尾多出来的逗号。
// sing-box 自己读配置时两样都容忍,社区模板里也常见;标准库的解析器不认。
func stripJSONComments(src []byte) []byte {
	out := make([]byte, 0, len(src))
	inString := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			out = append(out, c)
			if c == '\\' && i+1 < len(src) {
				i++
				out = append(out, src[i])
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			if i < len(src) {
				out = append(out, '\n')
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
				i++
			}
			i++
		default:
			out = append(out, c)
		}
	}

	// 末尾逗号:逗号后面(跳过空白)紧跟 } 或 ] 就丢掉。
	cleaned := make([]byte, 0, len(out))
	inString = false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inString {
			cleaned = append(cleaned, c)
			if c == '\\' && i+1 < len(out) {
				i++
				cleaned = append(cleaned, out[i])
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
		}
		if c == ',' {
			j := i + 1
			for j < len(out) && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				continue
			}
		}
		cleaned = append(cleaned, c)
	}
	return cleaned
}

// sbObject 是保留键顺序的 JSON 对象。
//
// 模板是人写的:type / tag 在前、分组成员在后。用 map 过一遍会按字母重排,下发的配置
// 和用户在面板里看到的模板对不上,出了问题也难比对。所以只动要动的键,其余原样带过去。
type sbObject struct {
	keys []string
	vals map[string]json.RawMessage
}

func parseSBObject(raw []byte) (*sbObject, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("不是 JSON 对象")
	}
	obj := &sbObject{vals: make(map[string]json.RawMessage)}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("JSON 对象的键不是字符串")
		}
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, err
		}
		if _, dup := obj.vals[key]; !dup {
			obj.keys = append(obj.keys, key)
		}
		obj.vals[key] = val
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("JSON 对象后面还有多余内容")
	}
	return obj, nil
}

// sbObjectFromMap 把产出的节点(map)转成有序对象:type、tag 在前,其余按字母序。
func sbObjectFromMap(m map[string]any) (*sbObject, error) {
	obj := &sbObject{vals: make(map[string]json.RawMessage, len(m))}
	rest := make([]string, 0, len(m))
	for k := range m {
		if k != "type" && k != "tag" {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range append([]string{"type", "tag"}, rest...) {
		v, ok := m[k]
		if !ok {
			continue
		}
		if err := obj.set(k, v); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

func sbMarshal(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // 规则集地址里的 & 不要变成 &
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func (o *sbObject) set(key string, v any) error {
	var raw json.RawMessage
	switch t := v.(type) {
	case json.RawMessage:
		raw = t
	case []*sbObject:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			item.writeTo(&buf)
		}
		buf.WriteByte(']')
		raw = buf.Bytes()
	default:
		m, err := sbMarshal(v)
		if err != nil {
			return err
		}
		raw = m
	}
	if _, exists := o.vals[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.vals[key] = raw
	return nil
}

func (o *sbObject) del(key string) {
	if _, ok := o.vals[key]; !ok {
		return
	}
	delete(o.vals, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

// str 取字符串字段;不存在或不是字符串返回 ""。
func (o *sbObject) str(key string) string {
	var s string
	if raw, ok := o.vals[key]; ok {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

func (o *sbObject) writeTo(buf *bytes.Buffer) {
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		name, _ := sbMarshal(k)
		buf.Write(name)
		buf.WriteByte(':')
		buf.Write(o.vals[k])
	}
	buf.WriteByte('}')
}

func (o *sbObject) raw() json.RawMessage {
	var buf bytes.Buffer
	o.writeTo(&buf)
	return buf.Bytes()
}

// objectList 读 key 下的对象数组。键不存在返回空;存在但不是「对象的数组」报错。
func (o *sbObject) objectList(key string) ([]*sbObject, error) {
	raw, ok := o.vals[key]
	if !ok || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("%s 必须是数组", key)
	}
	list := make([]*sbObject, 0, len(items))
	for i, item := range items {
		obj, err := parseSBObject(item)
		if err != nil {
			return nil, fmt.Errorf("%s 第 %d 项不是对象", key, i+1)
		}
		list = append(list, obj)
	}
	return list, nil
}

// parseSingboxTemplate 把模板读成有序对象(容忍注释和末尾逗号)。
func parseSingboxTemplate(content string) (*sbObject, error) {
	cleaned := bytes.TrimSpace(stripJSONComments([]byte(content)))
	if len(cleaned) == 0 {
		return nil, errors.New("模板内容为空")
	}
	root, err := parseSBObject(cleaned)
	if err != nil {
		var syn *json.SyntaxError
		if errors.As(err, &syn) {
			line, col := jsonOffsetToLineCol(cleaned, syn.Offset)
			return nil, fmt.Errorf("JSON 语法错误(约第 %d 行第 %d 列): %v", line, col, err)
		}
		return nil, fmt.Errorf("JSON 解析失败: %v", err)
	}
	return root, nil
}

func jsonOffsetToLineCol(data []byte, offset int64) (line, col int) {
	line, col = 1, 1
	for i := int64(0); i < offset && i < int64(len(data)); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// validateSingboxTemplate 保存模板前的检查。
//
// 只查「这份东西套上节点后一定出不来」的结构问题:不是合法 JSON、没有 outbounds、出站没有 tag、
// tag 重复。路由规则引用了哪个出站不查 —— 规则可以直接指向某个节点名,那要到生成订阅时才知道在不在。
func validateSingboxTemplate(content string) error {
	root, err := parseSingboxTemplate(content)
	if err != nil {
		return err
	}
	if _, ok := root.vals["outbounds"]; !ok {
		return errors.New("缺少 outbounds:sing-box 模板要是一份完整配置,分组(selector / urltest)写在 outbounds 里")
	}
	seen := make(map[string]string)
	for _, key := range []string{"outbounds", "endpoints"} {
		list, err := root.objectList(key)
		if err != nil {
			return err
		}
		for i, ob := range list {
			tag := ob.str("tag")
			if tag == "" {
				return fmt.Errorf("%s 第 %d 项缺少 tag", key, i+1)
			}
			if isSingboxPlaceholder(tag) {
				return fmt.Errorf("%s 第 %d 项的 tag 不能是占位符 %s", key, i+1, tag)
			}
			if prev, dup := seen[tag]; dup {
				return fmt.Errorf("tag %q 重复(%s 与 %s 里各有一个)", tag, prev, key)
			}
			seen[tag] = key
			if raw, has := ob.vals["outbounds"]; has {
				var members []string
				if err := json.Unmarshal(raw, &members); err != nil {
					return fmt.Errorf("分组 %q 的 outbounds 必须是字符串数组", tag)
				}
			}
		}
	}
	return nil
}

var singboxPlaceholderRe = regexp.MustCompile(`^\{[^{}]*\}$`)

func isSingboxPlaceholder(s string) bool {
	return singboxPlaceholderRe.MatchString(strings.TrimSpace(s))
}

// singboxIncludeAllDisabled 分组上明确写了 "include_all": false。
func singboxIncludeAllDisabled(raw json.RawMessage) bool {
	var v bool
	return len(raw) > 0 && json.Unmarshal(raw, &v) == nil && !v
}

// applySingboxFilters 按分组的 filter 依次筛节点名。写法不认识的条目跳过(等于没写)。
func applySingboxFilters(tags []string, raw json.RawMessage) []string {
	if len(raw) == 0 {
		return tags
	}
	var filters []struct {
		Action   string `json:"action"`
		Keywords any    `json:"keywords"`
	}
	if err := json.Unmarshal(raw, &filters); err != nil {
		return tags
	}
	for _, f := range filters {
		var keywords []string
		switch k := f.Keywords.(type) {
		case string:
			keywords = []string{k}
		case []any:
			for _, item := range k {
				if s, ok := item.(string); ok {
					keywords = append(keywords, s)
				}
			}
		}
		pattern := strings.Join(keywords, "|")
		if strings.TrimSpace(pattern) == "" {
			continue
		}
		include := !strings.EqualFold(strings.TrimSpace(f.Action), "exclude")
		match := singboxKeywordMatcher(pattern)
		kept := make([]string, 0, len(tags))
		for _, tag := range tags {
			if match(tag) == include {
				kept = append(kept, tag)
			}
		}
		tags = kept
	}
	return tags
}

// singboxKeywordMatcher 关键字按正则匹配。社区模板是照 Python 的正则写的,个别写法
// (如前瞻 (?!...))Go 不支持 —— 编译不过时退回「按 | 拆开、逐个做子串匹配」,不让整组失效。
func singboxKeywordMatcher(pattern string) func(string) bool {
	if re, err := regexp.Compile(pattern); err == nil {
		return re.MatchString
	}
	logger.Info("[sing-box模板] 筛选关键字不是 Go 支持的正则,按子串匹配", "keywords", pattern)
	parts := strings.Split(pattern, "|")
	return func(s string) bool {
		for _, p := range parts {
			if p != "" && strings.Contains(s, p) {
				return true
			}
		}
		return false
	}
}

// pruneSingboxDanglingRefs 清掉模板里指向不存在出站的引用。
//
// 在面板里删掉一个分组、或者规则直接指向的节点这次没下发,模板的路由规则就会指向一个不存在的出站,
// sing-box 遇到这种配置是整份拒载。与其让用户拿到一份打不开的配置,不如让这部分流量落到默认出站:
//   - 路由规则的 outbound 不存在 → 去掉这条规则;
//   - route.final 不存在 → 去掉(sing-box 用第一个出站);
//   - DNS 服务器、HTTP 客户端(http_clients,规则集靠它下载)的 detour 不存在 → 去掉(改为直连)。
func pruneSingboxDanglingRefs(root *sbObject, alive map[string]bool) error {
	if clients, err := root.objectList("http_clients"); err != nil {
		return fmt.Errorf("sing-box 模板的 %w", err)
	} else if len(clients) > 0 {
		changed := false
		for _, client := range clients {
			if d := client.str("detour"); d != "" && !alive[d] {
				logger.Warn("[sing-box模板] HTTP 客户端的 detour 指向不存在的出站,已去掉", "client", client.str("tag"), "detour", d)
				client.del("detour")
				changed = true
			}
		}
		if changed {
			if err := root.set("http_clients", clients); err != nil {
				return err
			}
		}
	}
	if raw, ok := root.vals["route"]; ok {
		route, err := parseSBObject(raw)
		if err != nil {
			return errors.New("sing-box 模板的 route 必须是对象")
		}
		changed := false
		if final := route.str("final"); final != "" && !alive[final] {
			logger.Warn("[sing-box模板] route.final 指向不存在的出站,已去掉", "final", final)
			route.del("final")
			changed = true
		}
		rules, err := route.objectList("rules")
		if err != nil {
			return fmt.Errorf("sing-box 模板的 route.%w", err)
		}
		kept := make([]*sbObject, 0, len(rules))
		for _, rule := range rules {
			if ob := rule.str("outbound"); ob != "" && !alive[ob] {
				logger.Warn("[sing-box模板] 路由规则指向不存在的出站,已去掉这条规则", "outbound", ob)
				changed = true
				continue
			}
			kept = append(kept, rule)
		}
		if changed {
			if _, has := route.vals["rules"]; has {
				if err := route.set("rules", kept); err != nil {
					return err
				}
			}
			if err := root.set("route", route.raw()); err != nil {
				return err
			}
		}
	}
	if raw, ok := root.vals["dns"]; ok {
		dns, err := parseSBObject(raw)
		if err != nil {
			return errors.New("sing-box 模板的 dns 必须是对象")
		}
		servers, err := dns.objectList("servers")
		if err != nil {
			return fmt.Errorf("sing-box 模板的 dns.%w", err)
		}
		changed := false
		for _, srv := range servers {
			if d := srv.str("detour"); d != "" && !alive[d] {
				logger.Warn("[sing-box模板] DNS 服务器的 detour 指向不存在的出站,已去掉", "server", srv.str("tag"), "detour", d)
				srv.del("detour")
				changed = true
			}
		}
		if changed {
			if err := dns.set("servers", servers); err != nil {
				return err
			}
			if err := root.set("dns", dns.raw()); err != nil {
				return err
			}
		}
	}
	return nil
}

// singboxUnloadableReason 这个出站会不会让官方 sing-box 拒绝加载**整份**配置;不会返回 ""。
//
// 套了模板的订阅是要直接当配置跑的,sing-box 解析配置时任何一个出站不合格都是整份拒载 ——
// 一个坏节点就让用户所有节点都用不了。所以这里把已知会触发拒载的挑出来不发:
//   - snell:sing-box 的客户端现在还不支持(用户确认)。内核 1.14 的源码里有 snell 出站,
//     1.13 及以下没有、遇到就报 unknown outbound type;等客户端跟上了再放开。
//     转换器照样会产出它,那是给不套模板的那条路用的,所以只在套模板时不发。
//   - REALITY 公钥不是 32 字节:节点数据不全(公钥为空)时 sing-box 报 invalid public_key。
func singboxUnloadableReason(item map[string]any) string {
	if typ, _ := item["type"].(string); typ == "snell" {
		return "sing-box 客户端还不支持 snell"
	}
	tls, _ := item["tls"].(map[string]any)
	reality, _ := tls["reality"].(map[string]any)
	if enabled, _ := reality["enabled"].(bool); enabled {
		publicKey, _ := reality["public_key"].(string)
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(strings.TrimSpace(publicKey), "="))
		if err != nil || len(raw) != 32 {
			return "REALITY 公钥无效"
		}
	}
	return ""
}

// relayGroupMembers 取中转组成员名。生成路径给的是 []string,
// 这里同时容忍 []any(比如从 YAML/JSON 反解出来的组),免得静默得到一个空组。
func relayGroupMembers(group map[string]any) []string {
	switch v := group["proxies"].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// injectProxiesIntoSingboxTemplate 把节点放进 sing-box 模板,产出完整配置。
//
// relayGroups 是动态生成的中转组(Clash 的 url-test 组),模板里没有;节点的 detour 指向它们,
// 所以要一起变成 urltest 出站,否则引用悬空、sing-box 整份拒载。
func injectProxiesIntoSingboxTemplate(templateContent string, proxies []map[string]any, relayGroups []map[string]any) (string, error) {
	root, err := parseSingboxTemplate(templateContent)
	if err != nil {
		return "", fmt.Errorf("sing-box 模板无法解析: %w", err)
	}
	tplOutbounds, err := root.objectList("outbounds")
	if err != nil {
		return "", fmt.Errorf("sing-box 模板无法解析: %w", err)
	}
	tplEndpoints, err := root.objectList("endpoints")
	if err != nil {
		return "", fmt.Errorf("sing-box 模板无法解析: %w", err)
	}

	// alive:此刻能被引用的全部 tag(模板自带的 + 节点 + 中转组)。
	alive := make(map[string]bool)
	directTag := ""
	for _, ob := range append(append([]*sbObject{}, tplOutbounds...), tplEndpoints...) {
		alive[ob.str("tag")] = true
	}
	for _, ob := range tplOutbounds {
		if ob.str("type") == "direct" {
			directTag = ob.str("tag")
			break
		}
	}

	// 节点。shadow-tls 一个节点会产出两个出站(本体 + <名>_shadowtls),后者只是前者的 detour,
	// 不该出现在分组里 —— 所以「可选节点」只认名字与输入节点对得上的。
	nodeNames := make(map[string]bool, len(proxies))
	sbProxies := make([]substore.Proxy, 0, len(proxies))
	for _, p := range proxies {
		sbProxies = append(sbProxies, substore.Proxy(p))
		if name, _ := p["name"].(string); name != "" {
			nodeNames[name] = true
		}
	}
	produced, err := substore.NewSingboxProducer().Produce(sbProxies, "internal", &substore.ProduceOptions{})
	if err != nil {
		return "", err
	}
	rawItems, ok := produced.([]map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected sing-box producer result type: %T", produced)
	}
	type nodeItem struct {
		tag     string
		data    map[string]any
		dropped bool
	}
	items := make([]*nodeItem, 0, len(rawItems))
	for _, raw := range rawItems {
		tag, _ := raw["tag"].(string)
		if tag == "" {
			continue
		}
		if reason := singboxUnloadableReason(raw); reason != "" {
			logger.Warn("[sing-box模板] 节点会让 sing-box 拒绝加载整份配置,本次不下发", "tag", tag, "reason", reason)
			continue
		}
		if alive[tag] {
			// 与模板里的出站(或前面的节点)重名:sing-box 不允许 tag 重复,后来的让路。
			logger.Warn("[sing-box模板] 节点名与已有出站重名,本次不下发", "tag", tag)
			continue
		}
		alive[tag] = true
		items = append(items, &nodeItem{tag: tag, data: raw})
	}
	if len(items) < len(proxies) {
		logger.Info("[sing-box模板] 部分节点 sing-box 不支持、加载不了或重名,已跳过",
			"total", len(proxies), "outbounds", len(items))
	}

	// 中转组。
	type relayGroup struct {
		name    string
		members []string
		url     string
		secs    int
		dead    bool
	}
	var relays []*relayGroup
	for _, g := range relayGroups {
		name, _ := g["name"].(string)
		if name == "" || alive[name] {
			continue
		}
		rg := &relayGroup{name: name, members: relayGroupMembers(g), url: "https://www.gstatic.com/generate_204", secs: 300}
		if u, _ := g["url"].(string); u != "" {
			rg.url = u
		}
		switch n := g["interval"].(type) {
		case int:
			rg.secs = n
		case int64:
			rg.secs = int(n)
		case float64:
			rg.secs = int(n)
		}
		alive[name] = true
		relays = append(relays, rg)
	}

	// 去悬空引用,反复做到不再变化:中转组的成员没了 → 组没了 → 走这个组的节点也不能发
	// (detour 指向不存在的出站,sing-box 整份拒载;把 detour 摘掉让它直连落地更糟,绕过了入口)。
	for changed := true; changed; {
		changed = false
		for _, rg := range relays {
			if rg.dead {
				continue
			}
			kept := rg.members[:0:0]
			for _, m := range rg.members {
				if alive[m] && m != rg.name {
					kept = append(kept, m)
				}
			}
			rg.members = kept
			if len(kept) == 0 {
				rg.dead = true
				delete(alive, rg.name)
				changed = true
			}
		}
		for _, it := range items {
			if it.dropped {
				continue
			}
			if detour, _ := it.data["detour"].(string); detour != "" && !alive[detour] {
				logger.Warn("[sing-box模板] 节点的前置出站不在本次订阅里,本次不下发", "tag", it.tag, "detour", detour)
				it.dropped = true
				delete(alive, it.tag)
				changed = true
			}
		}
	}

	var selectable []string
	for _, it := range items {
		if !it.dropped && nodeNames[it.tag] {
			selectable = append(selectable, it.tag)
		}
	}

	// 展开模板里的分组。
	needDirect := false
	for _, ob := range tplOutbounds {
		raw, isGroup := ob.vals["outbounds"]
		if !isGroup {
			ob.del("filter")
			ob.del("include_all")
			continue
		}
		var members []string
		if err := json.Unmarshal(raw, &members); err != nil {
			return "", fmt.Errorf("sing-box 模板分组 %q 的 outbounds 必须是字符串数组", ob.str("tag"))
		}
		self := ob.str("tag")
		filtered := applySingboxFilters(selectable, ob.vals["filter"])
		seen := make(map[string]bool)
		expanded := make([]string, 0, len(members)+len(filtered))
		add := func(tag string) {
			if tag == self || seen[tag] || !alive[tag] {
				return
			}
			seen[tag] = true
			expanded = append(expanded, tag)
		}
		hasPlaceholder := false
		for _, m := range members {
			if isSingboxPlaceholder(m) {
				hasPlaceholder = true
				for _, tag := range filtered {
					add(tag)
				}
				continue
			}
			add(m)
		}
		// 没写占位符的分组也放进全部节点(接在模板写的成员后面,filter 照样生效);
		// 模板里给分组写了 "include_all": false 的不放 —— 只想选别的分组时用。
		if !hasPlaceholder && !singboxIncludeAllDisabled(ob.vals["include_all"]) {
			for _, tag := range filtered {
				add(tag)
			}
		}
		if len(expanded) == 0 {
			if directTag == "" {
				needDirect = true
				directTag = "direct"
				for alive[directTag] {
					directTag += "-out"
				}
				alive[directTag] = true
			}
			expanded = []string{directTag}
		}
		if err := ob.set("outbounds", expanded); err != nil {
			return "", err
		}
		if def := ob.str("default"); def != "" {
			found := false
			for _, tag := range expanded {
				if tag == def {
					found = true
					break
				}
			}
			if !found {
				ob.del("default")
			}
		}
		ob.del("filter")
		ob.del("include_all")
	}

	outbounds := append([]*sbObject{}, tplOutbounds...)
	if needDirect {
		direct, _ := sbObjectFromMap(map[string]any{"type": "direct", "tag": directTag})
		outbounds = append(outbounds, direct)
	}
	for _, rg := range relays {
		if rg.dead {
			continue
		}
		group := &sbObject{vals: make(map[string]json.RawMessage)}
		_ = group.set("type", "urltest")
		_ = group.set("tag", rg.name)
		_ = group.set("outbounds", rg.members)
		_ = group.set("url", rg.url)
		_ = group.set("interval", fmt.Sprintf("%ds", rg.secs))
		_ = group.set("tolerance", 50)
		outbounds = append(outbounds, group)
	}
	endpoints := append([]*sbObject{}, tplEndpoints...)
	for _, it := range items {
		if it.dropped {
			continue
		}
		obj, err := sbObjectFromMap(it.data)
		if err != nil {
			return "", err
		}
		// WireGuard / Tailscale 在 sing-box 里是 endpoint,不是出站。
		if typ, _ := it.data["type"].(string); typ == "wireguard" || typ == "tailscale" {
			endpoints = append(endpoints, obj)
		} else {
			outbounds = append(outbounds, obj)
		}
	}

	if err := root.set("outbounds", outbounds); err != nil {
		return "", err
	}
	if err := pruneSingboxDanglingRefs(root, alive); err != nil {
		return "", err
	}
	if _, had := root.vals["endpoints"]; had || len(endpoints) > 0 {
		if err := root.set("endpoints", endpoints); err != nil {
			return "", err
		}
	}

	var compact bytes.Buffer
	root.writeTo(&compact)
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact.Bytes(), "", "  "); err != nil {
		return "", err
	}
	pretty.WriteByte('\n')
	return pretty.String(), nil
}
