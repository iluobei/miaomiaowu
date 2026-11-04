package backupstore

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrCreateMasterSeed 读取(首次则生成)加密备份凭据用的密钥种子。
//
// 妙妙屋X 用主控身份密钥的种子派生;妙妙屋没有这套身份密钥,所以单独落一个
// 32 字节随机种子到 data/ 下。文件随 data/ 一起进备份,恢复后旧凭据仍能解开。
func LoadOrCreateMasterSeed(path string) ([]byte, error) {
	if raw, err := os.ReadFile(path); err == nil {
		seed, derr := hex.DecodeString(strings.TrimSpace(string(raw)))
		if derr == nil && len(seed) == 32 {
			return seed, nil
		}
		return nil, fmt.Errorf("备份密钥文件 %s 内容无效", path)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取备份密钥文件失败: %w", err)
	}
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建备份密钥目录失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(seed)+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("写入备份密钥文件失败: %w", err)
	}
	return seed, nil
}
