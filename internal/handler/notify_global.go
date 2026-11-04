package handler

import "miaomiaowu/internal/notify"

var globalNotifier *notify.Notifier

// InitNotifier initializes the global notifier with the given config.
// 替换时先停掉旧 Notifier 的各渠道发送 goroutine。
func InitNotifier(cfg notify.Config) {
	if globalNotifier != nil {
		globalNotifier.Close()
	}
	globalNotifier = notify.New(cfg)
}

// GetNotifier returns the global notifier instance.
func GetNotifier() *notify.Notifier {
	return globalNotifier
}
