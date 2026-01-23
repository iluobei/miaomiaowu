package handler

import (
	"encoding/base64"
	"strings"
)

// base64DecodeV2ray 解码 base64 内容（支持标准和 URL Safe 格式）
func base64DecodeV2ray(s string) (string, error) {
	s = strings.TrimSpace(s)
	// 替换 URL Safe 字符
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	// 补齐 padding
	if pad := len(s) % 4; pad > 0 {
		s += strings.Repeat("=", 4-pad)
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
