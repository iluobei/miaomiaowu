package handler

// noAvailableNodeName 订阅里一个能用的节点都没有时塞的占位节点名(用户在客户端里看得到这句话)。
const noAvailableNodeName = "⚠️ 无可用节点,请联系管理员"

// noAvailableNodeProxy 占位节点:指向本机一个不可能有服务的端口,连不上但各客户端都能加载。
// 用于「订阅里一个可发的节点都没有」(全部禁用 / 标签没匹配到 / 配置全部残缺)等情况 ——
// 返回错误(502 / 404)的话客户端只显示更新失败,用户不知道为什么;
// 返回一份带这条提示的配置,模板分组照常生成。
func noAvailableNodeProxy() map[string]any {
	return map[string]any{
		"name":     noAvailableNodeName,
		"type":     "ss",
		"server":   "127.0.0.1",
		"port":     1,
		"cipher":   "aes-128-gcm",
		"password": "no-available-node",
		"udp":      false,
	}
}
