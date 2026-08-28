package notify

// EventType enumerates notification categories.
type EventType string

const (
	EventSubscribeFetch EventType = "subscribe_fetch"
	EventLogin          EventType = "login"
	EventIPBan          EventType = "ip_ban"
	EventSilentMode     EventType = "silent_mode"
	EventDailyTraffic   EventType = "daily_traffic"
	EventExpiry         EventType = "expiry"

	// EventNodeProbeOffline/Online 外部节点探测判定的不可用/恢复。
	// 与探针上报的服务器状态不是一回事:那个看的是机器活没活,这个是真拨了一次
	// 节点本身 —— 外部导入的节点根本没有探针可看。
	EventNodeProbeOffline EventType = "node_probe_offline"
	EventNodeProbeOnline  EventType = "node_probe_online"
)

// Config holds Telegram notification configuration.
// Designed to be loaded/stored externally; this package does not import storage.
type Config struct {
	Enabled                bool
	BotToken               string
	ChatID                 string
	NotifySubscribeFetch   bool
	NotifyLogin            bool
	NotifyIPBan            bool
	NotifySilentMode       bool
	NotifyDailyTraffic     bool
	NotifyExpiry           bool
	NotifyNodeProbeOffline bool
	NotifyNodeProbeOnline  bool
	DailyTrafficTime       string // "HH:MM" format, e.g. "08:00"
}

// Event holds data for a notification to be sent.
type Event struct {
	Type    EventType
	Title   string
	Message string
}
