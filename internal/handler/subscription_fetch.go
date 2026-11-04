package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
)

// 订阅获取开关(users.subscription_fetch,见 storage/subscription_fetch.go)。
//
// 拦截放在「订阅最终交给谁」那一步,而不是各个入口:
//   - SubscriptionHandler.ServeHTTP —— /api/clash/subscribe?token=、/{文件短码}{用户短码}
//   - TempSubscriptionAccessHandler —— /t/{id}(按签发人)
//   - ProxyProviderServeHandler —— /api/proxy-provider/{id}(订阅里引用的代理集合)
//
// 短码、token 长链接最后都落到上面三个 handler 之一,身份已经解析好放在 context 里
// (短码 → 用户短码对应的用户,token → token 主人)。
//
// 限时打开(#1497):打开时可带 duration_seconds,到期回到 off。
// 不跑定时任务,由 subscriptionFetchState 在读的时候判到期并写回。

// subscriptionFetchNow 判限时打开是否到期用的时钟(测试里替换)。
var subscriptionFetchNow = time.Now

// 限时打开可选时长的上下限(秒)。页面上给的是 10 分钟 / 1 小时 / 1 天 / 7 天。
const (
	subscriptionFetchMinOpenFor = 60
	subscriptionFetchMaxOpenFor = 7 * 24 * 60 * 60
)

// subscriptionFetchUntil 把请求里的 duration_seconds 换成到期时刻(unix 秒):0 / 不传 = 永久打开(until 0)。
// 只有打开时能带时长,关闭时带了算请求错误。
func subscriptionFetchUntil(enabled bool, durationSeconds int64) (int64, error) {
	if durationSeconds == 0 {
		return 0, nil
	}
	if !enabled {
		return 0, errors.New("关闭时不能带 duration_seconds")
	}
	if durationSeconds < subscriptionFetchMinOpenFor || durationSeconds > subscriptionFetchMaxOpenFor {
		return 0, errors.New("duration_seconds 须在 60 秒到 7 天之间")
	}
	return subscriptionFetchNow().Unix() + durationSeconds, nil
}

// subscriptionFetchDisabledMessage 拒绝时给客户端的一句话。客户端多半只显示状态码,这句给浏览器 / curl 看。
const subscriptionFetchDisabledMessage = "订阅获取已被管理员关闭,请联系管理员"

// subscriptionFetchState 读开关(已按限时打开是否到期折算)。读不到(用户不在 / 库出错)按开处理:
// 一时的数据库错误不该把订阅断掉,用户不存在的情况下游各自会处理。
// 限时打开已到期的按 off 处理,并顺手把库里也改掉(写失败不要紧,下次读还会这么判)。
func subscriptionFetchState(ctx context.Context, repo *storage.TrafficRepository, username string) storage.SubscriptionFetchStatus {
	on := storage.SubscriptionFetchStatus{State: storage.SubscriptionFetchOn}
	if repo == nil || strings.TrimSpace(username) == "" {
		return on
	}
	raw, err := repo.GetUserSubscriptionFetch(ctx, username)
	if err != nil {
		if !errors.Is(err, storage.ErrUserNotFound) {
			logger.Warn("[订阅获取] 读取开关失败,按开启处理", "username", username, "error", err)
		}
		return on
	}
	eff := raw.Effective(subscriptionFetchNow())
	if eff.State != raw.State {
		if err := repo.ExpireUserSubscriptionFetch(ctx, username, raw.Until); err != nil {
			logger.Warn("[订阅获取] 限时打开已到期,写回关闭失败", "username", username, "error", err)
		} else {
			logger.Info("[订阅获取] 限时打开已到期,自动关闭", "username", username)
		}
	}
	return eff
}

// rejectSubscriptionFetchDisabled 该用户关了订阅获取就回 403 并返回 true。
// 回 403 + 一句说明,不回空 200 / 空节点 —— 那样客户端会把节点清空还当作更新成功。
func rejectSubscriptionFetchDisabled(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) bool {
	if subscriptionFetchState(r.Context(), repo, username).State == storage.SubscriptionFetchOn {
		return false
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, subscriptionFetchDisabledMessage, http.StatusForbidden)
	return true
}

// NewUserSubscriptionFetchHandler POST /api/admin/users/subscription-fetch {username, enabled, duration_seconds?}
// 管理员开关某个用户的订阅获取。打开时带 duration_seconds 为限时打开,到期回到 off;不带为永久打开。
func NewUserSubscriptionFetchHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("user subscription fetch handler requires repository")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}
		var payload struct {
			Username        string `json:"username"`
			Enabled         *bool  `json:"enabled"`
			DurationSeconds int64  `json:"duration_seconds"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		username := strings.TrimSpace(payload.Username)
		if username == "" || payload.Enabled == nil {
			writeError(w, http.StatusBadRequest, errors.New("username 与 enabled 必填"))
			return
		}
		until, err := subscriptionFetchUntil(*payload.Enabled, payload.DurationSeconds)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		st, err := repo.SetUserSubscriptionFetch(r.Context(), username, *payload.Enabled, until)
		if err != nil {
			if errors.Is(err, storage.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, errors.New("user not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		logger.Info("[订阅获取] 管理员修改开关", "username", username, "state", st.State, "until", st.Until)
		respondJSON(w, http.StatusOK, map[string]any{
			"username": username, "subscription_fetch": st.State, "subscription_fetch_until": st.Until,
		})
	})
}
