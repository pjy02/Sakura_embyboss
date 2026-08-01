package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	APIBase, InternalAPIURL, InternalAPIToken, BotToken string
	RequestTimeout                                      time.Duration
}

type Bot struct {
	config Config
	client *http.Client
	logger *slog.Logger
	offset int64
}

func New(config Config, logger *slog.Logger) *Bot {
	timeout := config.RequestTimeout
	if timeout < 35*time.Second {
		timeout = 35 * time.Second
	}
	return &Bot{config: config, client: &http.Client{Timeout: timeout}, logger: logger}
}

func (b *Bot) Run(ctx context.Context) error {
	token := b.config.BotToken
	for token == "" {
		var err error
		token, err = b.fetchCredential(ctx, "telegram.bot_token")
		if err != nil {
			b.logger.Warn("Telegram credential is not ready; retrying", "error", err)
			if !wait(ctx, 10*time.Second) {
				return nil
			}
		}
	}
	b.logger.Info("Telegram adapter started")
	for ctx.Err() == nil {
		updates, err := b.getUpdates(ctx, token)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			b.logger.Warn("Telegram update polling failed", "error", err)
			if !wait(ctx, 3*time.Second) {
				return nil
			}
			continue
		}
		for _, update := range updates {
			if update.ID >= b.offset {
				b.offset = update.ID + 1
			}
			b.handleUpdate(ctx, token, update)
		}
	}
	return nil
}

type update struct {
	ID      int64    `json:"update_id"`
	Message *message `json:"message"`
}
type message struct {
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	From struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
	Text string `json:"text"`
}

func (b *Bot) getUpdates(ctx context.Context, token string) ([]update, error) {
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?timeout=25&offset=%d&allowed_updates=%%5B%%22message%%22%%5D", b.config.APIBase, token, b.offset)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := b.client.Do(request)
	if err != nil {
		return nil, safeNetworkError("Telegram getUpdates", err)
	}
	defer response.Body.Close()
	var body struct {
		OK          bool     `json:"ok"`
		Description string   `json:"description"`
		Result      []update `json:"result"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&body); err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK || !body.OK {
		return nil, fmt.Errorf("Telegram API rejected getUpdates: status=%d description=%s", response.StatusCode, body.Description)
	}
	return body.Result, nil
}

func (b *Bot) handleUpdate(ctx context.Context, token string, item update) {
	if item.Message == nil || item.Message.Chat.Type != "private" {
		return
	}
	fields := strings.Fields(item.Message.Text)
	if len(fields) == 0 {
		return
	}
	command := strings.Split(fields[0], "@")[0]
	var reply string
	switch command {
	case "/bind":
		if len(fields) != 2 {
			reply = "用法：/bind Web 页面显示的一次性绑定码"
			break
		}
		if err := b.confirmLink(ctx, fields[1], item.Message.From.ID, item.Message.From.Username); err != nil {
			b.logger.Warn("Telegram identity binding failed", "telegram_user_id", item.Message.From.ID, "error", err)
			reply = "绑定失败：绑定码无效、已过期，或该 Telegram 已绑定其他账号。"
		} else {
			reply = "绑定成功，Web 账号与 Telegram 身份已关联。"
		}
	case "/register":
		if len(fields) < 3 || len(fields) > 4 {
			reply = "用法：/register <邀请码或-> <Emby用户名> [实例ID]"
			break
		}
		invite := fields[1]
		if invite == "-" {
			invite = ""
		}
		instance := ""
		if len(fields) == 4 {
			instance = fields[3]
		}
		result, err := b.requestProvision(ctx, item.Message.From.ID, item.Message.From.Username, fields[2], invite, instance, fmt.Sprintf("tg-update-%d", item.ID))
		if err != nil {
			b.logger.Warn("Emby provisioning request failed", "telegram_user_id", item.Message.From.ID, "error", err)
			reply = "注册申请失败。请先用 /bind 绑定 Web 账号，并确认邀请码、会员和用户名有效。"
		} else {
			reply = formatProvision(result)
		}
	case "/register-status":
		if len(fields) != 2 {
			reply = "用法：/register-status <任务ID>"
			break
		}
		result, err := b.provisionStatus(ctx, item.Message.From.ID, fields[1])
		if err != nil {
			reply = "查询失败：任务不存在或不属于当前账号。"
		} else {
			reply = formatProvision(result)
		}
	default:
		return
	}
	_ = b.sendMessage(ctx, token, item.Message.Chat.ID, reply)
}

type provisionResponse struct {
	Task struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		LastError string `json:"last_error"`
	} `json:"task"`
	Username          string `json:"username"`
	RemoteUserID      string `json:"remote_user_id"`
	GeneratedPassword string `json:"generated_password"`
}

func formatProvision(result provisionResponse) string {
	switch result.Task.Status {
	case "succeeded":
		return fmt.Sprintf("Emby 注册成功\n用户名：%s\n密码：%s\n任务：%s", result.Username, result.GeneratedPassword, result.Task.ID)
	case "failed", "dead":
		return fmt.Sprintf("Emby 注册失败：%s\n任务：%s", result.Task.LastError, result.Task.ID)
	default:
		return fmt.Sprintf("注册任务已提交，状态：%s\n任务：%s\n稍后使用 /register-status %s 查询。", result.Task.Status, result.Task.ID, result.Task.ID)
	}
}

func (b *Bot) requestProvision(ctx context.Context, telegramID int64, telegramUsername, username, invite, instanceID, key string) (provisionResponse, error) {
	body := map[string]any{"telegram_user_id": telegramID, "telegram_username": telegramUsername, "username": username, "invitation_code": invite, "idempotency_key": key}
	if instanceID != "" {
		body["instance_id"] = instanceID
	}
	return b.platformRequest(ctx, http.MethodPost, "/api/v3/internal/emby/provision-requests", body)
}

func (b *Bot) provisionStatus(ctx context.Context, telegramID int64, taskID string) (provisionResponse, error) {
	return b.platformRequest(ctx, http.MethodGet, "/api/v3/internal/emby/provision-requests/"+url.PathEscape(taskID)+"?telegram_user_id="+fmt.Sprint(telegramID), nil)
}

func (b *Bot) platformRequest(ctx context.Context, method, endpoint string, body any) (provisionResponse, error) {
	var reader io.Reader
	if body != nil {
		encoded, _ := json.Marshal(body)
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, b.config.InternalAPIURL+endpoint, reader)
	if err != nil {
		return provisionResponse{}, err
	}
	request.Header.Set("Authorization", "Bearer "+b.config.InternalAPIToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := b.client.Do(request)
	if err != nil {
		return provisionResponse{}, safeNetworkError("internal platform API", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return provisionResponse{}, fmt.Errorf("internal API returned status %d", response.StatusCode)
	}
	var result provisionResponse
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result); err != nil {
		return provisionResponse{}, err
	}
	return result, nil
}

func (b *Bot) confirmLink(ctx context.Context, code string, telegramUserID int64, username string) error {
	body, _ := json.Marshal(map[string]any{"code": code, "telegram_user_id": telegramUserID, "username": username})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config.InternalAPIURL+"/api/v3/internal/telegram/link-requests/confirm", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+b.config.InternalAPIToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return safeNetworkError("identity binding", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("internal API returned status %d", response.StatusCode)
	}
	return nil
}

func (b *Bot) fetchCredential(ctx context.Context, name string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.config.InternalAPIURL+"/api/v3/internal/credentials/"+url.PathEscape(name)+"/reveal", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+b.config.InternalAPIToken)
	response, err := b.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("credential endpoint returned status %d", response.StatusCode)
	}
	var body struct {
		Secret string `json:"secret"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body); err != nil {
		return "", err
	}
	if body.Secret == "" {
		return "", errors.New("credential is empty")
	}
	return body.Secret, nil
}

func (b *Bot) sendMessage(ctx context.Context, token string, chatID int64, text string) error {
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config.APIBase+"/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram sendMessage returned status %d", response.StatusCode)
	}
	return nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func safeNetworkError(operation string, err error) error {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return fmt.Errorf("%s network request failed: %w", operation, urlError.Err)
	}
	return fmt.Errorf("%s network request failed", operation)
}
