package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// telegramAPIBase 测试里替换成 httptest 地址。
var telegramAPIBase = "https://api.telegram.org/bot"

var httpClient = &http.Client{Timeout: 10 * time.Second}

// telegramChannel 内置 Telegram 渠道与额外添加的 Telegram 渠道共用。
// 正文原样按 Telegram legacy Markdown 发,行为与多渠道之前完全一致。
type telegramChannel struct {
	token  string
	chatID string
}

func (c *telegramChannel) Send(ctx context.Context, m Msg) error {
	return sendTelegram(ctx, c.token, c.chatID, composeText(m.Title, m.Body))
}

// composeText 拼最终发给 TG 的正文(与多渠道之前的格式逐字相同)。
func composeText(title, message string) string {
	return fmt.Sprintf("*%s*\n%s", title, message)
}

func sendTelegram(ctx context.Context, botToken, chatID, text string) error {
	if botToken == "" || chatID == "" {
		return fmt.Errorf("bot token or chat ID is empty")
	}

	endpoint := telegramAPIBase + botToken + "/sendMessage"
	params := url.Values{
		"chat_id":    {chatID},
		"text":       {text},
		"parse_mode": {"Markdown"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.URL.RawQuery = params.Encode()

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send telegram: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram API error (status %d): %s", resp.StatusCode, result.Description)
	}

	return nil
}
