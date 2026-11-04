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

	// 内置 Telegram 渠道(上面 BotToken / ChatID 那一组)在多渠道里的两项设置,存 system_settings。
	// TelegramChannelDisabled 用「关」而不是「开」:零值 = 照旧发,老配置升级后什么都不用动。
	TelegramChannelDisabled bool
	// TelegramEvents 内置 Telegram 渠道订阅的事件分组(见 EventGroups);空 = 全部。
	TelegramEvents []string
}

// Event holds data for a notification to be sent.
type Event struct {
	Type    EventType
	Title   string
	Message string
}

// EventGroups 事件分组,与通知设置里的开关一一对应。
// 渠道按分组订阅事件 —— 管理员看到的是开关名,不是内部事件类型。
var EventGroups = []string{
	"subscribe_fetch", "login", "ip_ban", "silent_mode", "daily_traffic", "expiry",
	"node_probe_offline", "node_probe_online",
}

// EventGroup 事件所属分组。妙妙屋的事件与开关一一对应,分组名就是事件名。
func EventGroup(t EventType) string {
	return string(t)
}

// NormalizeEventGroups 去掉未知 / 重复的分组,保持 EventGroups 的顺序。
func NormalizeEventGroups(in []string) []string {
	want := make(map[string]bool, len(in))
	for _, g := range in {
		want[g] = true
	}
	out := []string{}
	for _, g := range EventGroups {
		if want[g] {
			out = append(out, g)
		}
	}
	return out
}

// ChannelType 推送渠道类型。
type ChannelType string

const (
	ChannelTelegram   ChannelType = "telegram"
	ChannelServerChan ChannelType = "serverchan"
	ChannelFeishu     ChannelType = "feishu"
	ChannelDingTalk   ChannelType = "dingtalk"
	ChannelBark       ChannelType = "bark"
	ChannelSMTP       ChannelType = "smtp"
)

// BuiltinTelegramID 内置 Telegram 渠道的 ID —— 它的配置仍是 system_config 里的 token / chat_id,
// 不在 notify_channels 表里。
const BuiltinTelegramID int64 = 0

// ChannelConfig 一个推送渠道。
type ChannelConfig struct {
	ID      int64
	Type    ChannelType
	Name    string
	Enabled bool
	Params  map[string]string
	// Events 订阅的事件分组;空 = 通知设置里打开的全部事件。
	Events []string
}

// Subscribed 该渠道是否订阅了这个事件。
func (c ChannelConfig) Subscribed(t EventType) bool {
	if len(c.Events) == 0 {
		return true
	}
	g := EventGroup(t)
	for _, e := range c.Events {
		if e == g {
			return true
		}
	}
	return false
}

// Msg 交给渠道发送的一条消息。Body 是 Telegram legacy Markdown(*粗体*、`代码`),
// 各渠道自己转成目标格式。
type Msg struct {
	Title string
	Body  string
	Event EventType
}

// ChannelStatus 渠道最近一次发送的结果(Unix 秒,0 = 没有)。
type ChannelStatus struct {
	LastError   string
	LastErrorAt int64
	LastOKAt    int64
}
