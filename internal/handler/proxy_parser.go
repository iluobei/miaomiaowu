package handler

import "github.com/MMWOrg/mmwX-plugins/proxyparser"

// 节点 URI 解析已统一抽取到共享 module github.com/MMWOrg/mmwX-plugins/proxyparser,
// 供 miaomiaowu 与 miaomiaowux 共用。本文件仅保留薄包装,维持原有调用方签名不变。

// ParseProxyURL 解析单个代理 URI,返回 clash 风格字段的 map。
func ParseProxyURL(uri string) (map[string]any, error) {
	return proxyparser.Parse(normalizeMieruShareURI(uri))
}

// ParseV2raySubscription 解析 v2ray 订阅内容(base64 或明文多行 URI)。
func ParseV2raySubscription(content string) ([]map[string]any, error) {
	return proxyparser.ParseSubscription(rewriteMierusInContent(content))
}
