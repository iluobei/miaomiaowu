package handler

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"time"

	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/storage"
)

// 内置 Telegram 渠道在多渠道里的设置与发送状态(system_settings)。它的 token / chat_id 仍是
// system_config 的原列,不迁移 —— 升级后零操作照旧推送,降级回老版本也不受影响。
const (
	notifyTelegramDisabledKey  = "notify_telegram_channel_disabled"
	notifyTelegramEventsKey    = "notify_telegram_events"
	notifyTelegramLastErrKey   = "notify_telegram_last_error"
	notifyTelegramLastErrAtKey = "notify_telegram_last_error_at"
	notifyTelegramLastOKAtKey  = "notify_telegram_last_ok_at"
)

// BuildNotifyConfig 由 system_config + system_settings 拼出 Notifier 配置。启动与保存设置共用这一份,
// 免得两处各列一遍字段又漏掉几个。
func BuildNotifyConfig(ctx context.Context, repo *storage.TrafficRepository, sysCfg storage.SystemConfig) notify.Config {
	cfg := notify.Config{
		Enabled:                sysCfg.NotifyEnabled,
		BotToken:               sysCfg.TelegramBotToken,
		ChatID:                 sysCfg.TelegramChatID,
		NotifySubscribeFetch:   sysCfg.NotifySubscribeFetch,
		NotifyLogin:            sysCfg.NotifyLogin,
		NotifyIPBan:            sysCfg.NotifyIPBan,
		NotifySilentMode:       sysCfg.NotifySilentMode,
		NotifyDailyTraffic:     sysCfg.NotifyDailyTraffic,
		NotifyExpiry:           sysCfg.NotifyExpiry,
		NotifyNodeProbeOffline: sysCfg.NotifyNodeProbeOffline,
		NotifyNodeProbeOnline:  sysCfg.NotifyNodeProbeOnline,
		DailyTrafficTime:       sysCfg.NotifyDailyTrafficTime,
	}
	if repo != nil {
		v, _ := repo.GetSystemSetting(ctx, notifyTelegramDisabledKey)
		cfg.TelegramChannelDisabled, _ = strconv.ParseBool(v)
		if raw, _ := repo.GetSystemSetting(ctx, notifyTelegramEventsKey); raw != "" {
			var events []string
			if json.Unmarshal([]byte(raw), &events) == nil {
				cfg.TelegramEvents = notify.NormalizeEventGroups(events)
			}
		}
	}
	return cfg
}

// toNotifyChannelConfig 表里的一行 → Notifier 的渠道配置。
func toNotifyChannelConfig(c storage.NotifyChannel) notify.ChannelConfig {
	return notify.ChannelConfig{
		ID: c.ID, Type: notify.ChannelType(c.Type), Name: c.Name,
		Enabled: c.Enabled, Params: c.Params, Events: c.Events,
	}
}

// ReloadNotifyChannels 把 notify_channels 表整份装进全局 Notifier。启动时与每次增删改渠道后调用。
func ReloadNotifyChannels(ctx context.Context, repo *storage.TrafficRepository) error {
	n := GetNotifier()
	if n == nil || repo == nil {
		return nil
	}
	rows, err := repo.ListNotifyChannels(ctx)
	if err != nil {
		return err
	}
	cfgs := make([]notify.ChannelConfig, 0, len(rows))
	for _, r := range rows {
		cfgs = append(cfgs, toNotifyChannelConfig(r))
	}
	n.SetChannels(cfgs)
	return nil
}

// AttachNotifyStatusSink 发送结果回写到库(内置 Telegram 写 system_settings,其余写 notify_channels)。
func AttachNotifyStatusSink(repo *storage.TrafficRepository) {
	n := GetNotifier()
	if n == nil || repo == nil {
		return
	}
	n.SetStatusSink(func(id int64, st notify.ChannelStatus) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		if id == notify.BuiltinTelegramID {
			err = setSystemSettings(ctx, repo, map[string]string{
				notifyTelegramLastErrKey:   st.LastError,
				notifyTelegramLastErrAtKey: strconv.FormatInt(st.LastErrorAt, 10),
				notifyTelegramLastOKAtKey:  strconv.FormatInt(st.LastOKAt, 10),
			})
		} else {
			err = repo.UpdateNotifyChannelStatus(ctx, id, st.LastError, st.LastErrorAt, st.LastOKAt)
		}
		if err != nil {
			log.Printf("[Notify] 记录渠道 %d 发送状态失败: %v", id, err)
		}
	})
}

// setSystemSettings 逐项写 system_settings。
func setSystemSettings(ctx context.Context, repo *storage.TrafficRepository, values map[string]string) error {
	for k, v := range values {
		if err := repo.SetSystemSetting(ctx, k, v); err != nil {
			return err
		}
	}
	return nil
}
