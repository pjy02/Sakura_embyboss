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
	APIBase          string
	InternalAPIURL   string
	InternalAPIToken string
	BotToken         string
	RequestTimeout   time.Duration
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
	if len(fields) == 0 || strings.Split(fields[0], "@")[0] != "/bind" {
		return
	}
	if len(fields) != 2 {
		_ = b.sendMessage(ctx, token, item.Message.Chat.ID, "用法：/bind Web 页面显示的一次性绑定码")
		return
	}
	err := b.confirmLink(ctx, fields[1], item.Message.From.ID, item.Message.From.Username)
	if err != nil {
		b.logger.Warn("Telegram identity binding failed", "telegram_user_id", item.Message.From.ID, "error", err)
		_ = b.sendMessage(ctx, token, item.Message.Chat.ID, "绑定失败：绑定码无效、已过期，或该 Telegram 已绑定其他账号。")
		return
	}
	_ = b.sendMessage(ctx, token, item.Message.Chat.ID, "绑定成功，现在 Web 账号与 Telegram 身份已关联。")
}

func (b *Bot) confirmLink(ctx context.Context, code string, telegramUserID int64, username string) error {
	body, _ := json.Marshal(map[string]any{"code": code, "telegram_user_id": telegramUserID, "username": username})
	endpoint := b.config.InternalAPIURL + "/api/v3/internal/telegram/link-requests/confirm"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+b.config.InternalAPIToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return safeNetworkError("Telegram sendMessage", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("internal API returned status %d", response.StatusCode)
	}
	return nil
}

func (b *Bot) fetchCredential(ctx context.Context, name string) (string, error) {
	endpoint := b.config.InternalAPIURL + "/api/v3/internal/credentials/" + url.PathEscape(name) + "/reveal"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
