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
	if err := b.setCommands(ctx, token); err != nil {
		b.logger.Warn("Telegram command menu setup failed", "error", err)
	}
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
		for delivered := 0; delivered < 5; delivered++ {
			worked, deliveryErr := b.deliverNextNotification(ctx, token)
			if deliveryErr != nil {
				b.logger.Warn("Telegram notification delivery failed", "error", deliveryErr)
				break
			}
			if !worked {
				break
			}
		}
	}
	return nil
}

func (b *Bot) deliverNextNotification(ctx context.Context, token string) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, b.config.InternalAPIURL+"/api/v3/internal/notifications/telegram/next", nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+b.config.InternalAPIToken)
	response, err := b.client.Do(request)
	if err != nil {
		return false, safeNetworkError("notification claim", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNoContent {
		return false, nil
	}
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return false, fmt.Errorf("notification claim returned status %d", response.StatusCode)
	}
	var item struct {
		NotificationID string `json:"notification_id"`
		TelegramUserID int64  `json:"telegram_user_id"`
		Title          string `json:"title"`
		Body           string `json:"body"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&item); err != nil {
		return false, err
	}
	sendErr := b.sendMessage(ctx, token, item.TelegramUserID, item.Title+"\n\n"+item.Body)
	completeBody := map[string]string{}
	if sendErr != nil {
		completeBody["error"] = sendErr.Error()
	}
	encoded, _ := json.Marshal(completeBody)
	completeRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config.InternalAPIURL+"/api/v3/internal/notifications/telegram/"+url.PathEscape(item.NotificationID)+"/complete", bytes.NewReader(encoded))
	if err != nil {
		return true, err
	}
	completeRequest.Header.Set("Authorization", "Bearer "+b.config.InternalAPIToken)
	completeRequest.Header.Set("Content-Type", "application/json")
	completeResponse, err := b.client.Do(completeRequest)
	if err != nil {
		return true, err
	}
	defer completeResponse.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(completeResponse.Body, 4096))
	if completeResponse.StatusCode != http.StatusNoContent {
		return true, fmt.Errorf("notification completion returned status %d", completeResponse.StatusCode)
	}
	return true, sendErr
}

type update struct {
	ID            int64          `json:"update_id"`
	Message       *message       `json:"message"`
	CallbackQuery *callbackQuery `json:"callback_query"`
}
type message struct {
	ID   int64 `json:"message_id"`
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

type callbackQuery struct {
	ID   string `json:"id"`
	From struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"from"`
	Message *message `json:"message"`
	Data    string   `json:"data"`
}

type inlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type inlineKeyboard struct {
	InlineKeyboard [][]inlineButton `json:"inline_keyboard"`
}

func (b *Bot) getUpdates(ctx context.Context, token string) ([]update, error) {
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?timeout=25&offset=%d&allowed_updates=%%5B%%22message%%22%%2C%%22callback_query%%22%%5D", b.config.APIBase, token, b.offset)
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
	if item.CallbackQuery != nil {
		b.handleCallback(ctx, token, item)
		return
	}
	if item.Message == nil || item.Message.Chat.Type != "private" {
		return
	}
	fields := strings.Fields(item.Message.Text)
	if len(fields) == 0 {
		return
	}
	command := strings.Split(fields[0], "@")[0]
	var reply string
	var keyboard *inlineKeyboard
	switch command {
	case "/start", "/menu":
		reply, keyboard = b.dashboardReply(ctx, item.Message.From.ID)
	case "/help":
		reply = botHelpText()
		keyboard = mainKeyboard(false)
	case "/me", "/wallet", "/membership", "/emby":
		reply, keyboard = b.dashboardReply(ctx, item.Message.From.ID)
	case "/media":
		if len(fields) < 2 {
			reply = "用法：/media <影片名称>"
			break
		}
		var results []map[string]any
		err := b.botAction(ctx, item.Message.From.ID, "media_search", map[string]string{"query": strings.Join(fields[1:], " ")}, fmt.Sprintf("tg-media-%d", item.ID), &results)
		if err != nil {
			reply = "影片搜索失败，请确认 TMDB 已配置后重试。"
		} else {
			reply, keyboard = formatMediaResults(results)
		}
	case "/request":
		if len(fields) != 2 {
			reply = "用法：/request <搜索结果中的影片ID>"
			break
		}
		reply = b.createMediaRequestReply(ctx, item.Message.From.ID, fields[1], fmt.Sprintf("tg-request-%d", item.ID))
		keyboard = mainKeyboard(false)
	case "/requests":
		reply = b.collectionReply(ctx, item.Message.From.ID, "requests", "我的求片", formatRequestLine)
		keyboard = mainKeyboard(false)
	case "/tickets":
		reply = b.collectionReply(ctx, item.Message.From.ID, "tickets", "我的工单", formatTicketLine)
		keyboard = mainKeyboard(false)
	case "/ticket":
		payload := strings.TrimSpace(strings.TrimPrefix(item.Message.Text, fields[0]))
		parts := strings.SplitN(payload, "|", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			reply = "用法：/ticket 主题 | 问题描述"
			break
		}
		var ticket map[string]any
		err := b.botAction(ctx, item.Message.From.ID, "ticket_create", map[string]string{"subject": parts[0], "body": parts[1], "category": "general", "priority": "normal"}, fmt.Sprintf("tg-ticket-%d", item.ID), &ticket)
		if err != nil {
			reply = "工单创建失败，请稍后重试。"
		} else {
			reply = "工单已创建：" + stringField(ticket, "ticket_no") + "\n客服回复后会通过通知中心同步。"
		}
		keyboard = mainKeyboard(false)
	case "/notifications":
		reply = b.collectionReply(ctx, item.Message.From.ID, "notifications", "未读通知", formatNotificationLine)
		keyboard = mainKeyboard(false)
	case "/entitlements":
		reply = b.collectionReply(ctx, item.Message.From.ID, "entitlements", "我的权益", formatEntitlementLine)
		keyboard = mainKeyboard(false)
	case "/lines":
		reply = b.collectionReply(ctx, item.Message.From.ID, "lines", "可用线路", formatLineEntry)
		keyboard = mainKeyboard(false)
	case "/redeem":
		if len(fields) != 2 {
			reply = "用法：/redeem <权益码>"
			break
		}
		var result map[string]any
		if err := b.botAction(ctx, item.Message.From.ID, "entitlement_redeem", map[string]string{"code": fields[1]}, fmt.Sprintf("tg-entitlement-%d", item.ID), &result); err != nil {
			reply = "权益码兑换失败：请确认权益码有效且未被使用。"
		} else {
			reply = "权益兑换成功，Emby 媒体库权限正在同步。"
		}
		keyboard = mainKeyboard(false)
	case "/favorites":
		reply = b.collectionReply(ctx, item.Message.From.ID, "favorites", "Emby 收藏", formatFavoriteLine)
		keyboard = mainKeyboard(false)
	case "/favorite-sync":
		if len(fields) != 2 {
			reply = "用法：/favorite-sync <Emby绑定ID>"
			break
		}
		var result map[string]any
		if err := b.botAction(ctx, item.Message.From.ID, "favorite_sync", map[string]string{"binding_id": fields[1]}, fmt.Sprintf("tg-favorite-sync-%d", item.ID), &result); err != nil {
			reply = "收藏同步任务创建失败。"
		} else {
			reply = "收藏同步任务已创建。"
		}
		keyboard = mainKeyboard(false)
	case "/like", "/unlike":
		if len(fields) != 2 {
			reply = "用法：/like <影评ID> 或 /unlike <影评ID>"
			break
		}
		liked := command == "/like"
		var result map[string]any
		if err := b.botAction(ctx, item.Message.From.ID, "review_like", map[string]string{"review_id": fields[1], "liked": fmt.Sprint(liked)}, fmt.Sprintf("tg-review-like-%d", item.ID), &result); err != nil {
			reply = "影评点赞操作失败。"
		} else if liked {
			reply = "已点赞这条影评。"
		} else {
			reply = "已取消点赞。"
		}
		keyboard = mainKeyboard(false)
	case "/report-review":
		if len(fields) < 3 {
			reply = "用法：/report-review <影评ID> <spam|abuse|spoiler|copyright|other> [说明]"
			break
		}
		var result map[string]any
		if err := b.botAction(ctx, item.Message.From.ID, "review_report", map[string]string{"review_id": fields[1], "reason": fields[2], "detail": strings.Join(fields[3:], " ")}, fmt.Sprintf("tg-review-report-%d", item.ID), &result); err != nil {
			reply = "影评举报提交失败。"
		} else {
			reply = "举报已提交，管理员处理结果会保留审计记录。"
		}
		keyboard = mainKeyboard(false)
	case "/admin":
		reply = b.adminDashboardReply(ctx, item.Message.From.ID)
		keyboard = adminKeyboard()
	case "/users":
		reply = b.collectionReply(ctx, item.Message.From.ID, "admin_accounts", "最近账号", formatAccountLine)
		keyboard = adminKeyboard()
	case "/tasks":
		reply = b.collectionReply(ctx, item.Message.From.ID, "admin_tasks", "后台任务", formatTaskLine)
		keyboard = adminKeyboard()
	case "/risks":
		reply = b.collectionReply(ctx, item.Message.From.ID, "admin_risks", "风险事件", formatRiskLine)
		keyboard = adminKeyboard()
	case "/broadcast":
		payload := strings.TrimSpace(strings.TrimPrefix(item.Message.Text, fields[0]))
		parts := strings.SplitN(payload, "|", 2)
		if len(parts) != 2 {
			reply = "用法：/broadcast 标题 | 内容"
			break
		}
		var result map[string]any
		err := b.botAction(ctx, item.Message.From.ID, "admin_broadcast", map[string]string{"title": parts[0], "body": parts[1], "channel": "telegram"}, fmt.Sprintf("tg-broadcast-%d", item.ID), &result)
		if err != nil {
			reply = "广播创建失败：请确认管理员权限和目标账号。"
		} else {
			reply = "广播任务已创建，将由 Worker 可靠发送。"
		}
		keyboard = adminKeyboard()
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
		reply = "未识别的命令。发送 /help 查看可用功能。"
		keyboard = mainKeyboard(false)
	}
	_ = b.sendMessageWithKeyboard(ctx, token, item.Message.Chat.ID, reply, keyboard)
}

func (b *Bot) handleCallback(ctx context.Context, token string, item update) {
	query := item.CallbackQuery
	if query == nil || query.Message == nil || query.Message.Chat.Type != "private" {
		return
	}
	_ = b.answerCallback(ctx, token, query.ID)
	var reply string
	var keyboard *inlineKeyboard
	switch {
	case query.Data == "nav:home":
		reply, keyboard = b.dashboardReply(ctx, query.From.ID)
	case query.Data == "nav:requests":
		reply = b.collectionReply(ctx, query.From.ID, "requests", "我的求片", formatRequestLine)
		keyboard = mainKeyboard(false)
	case query.Data == "nav:tickets":
		reply = b.collectionReply(ctx, query.From.ID, "tickets", "我的工单", formatTicketLine)
		keyboard = mainKeyboard(false)
	case query.Data == "nav:notifications":
		reply = b.collectionReply(ctx, query.From.ID, "notifications", "未读通知", formatNotificationLine)
		keyboard = mainKeyboard(false)
	case query.Data == "nav:access":
		reply = b.collectionReply(ctx, query.From.ID, "entitlements", "我的权益", formatEntitlementLine)
		keyboard = mainKeyboard(false)
	case query.Data == "nav:favorites":
		reply = b.collectionReply(ctx, query.From.ID, "favorites", "Emby 收藏", formatFavoriteLine)
		keyboard = mainKeyboard(false)
	case query.Data == "nav:admin":
		reply = b.adminDashboardReply(ctx, query.From.ID)
		keyboard = adminKeyboard()
	case query.Data == "admin:users":
		reply = b.collectionReply(ctx, query.From.ID, "admin_accounts", "最近账号", formatAccountLine)
		keyboard = adminKeyboard()
	case query.Data == "admin:tasks":
		reply = b.collectionReply(ctx, query.From.ID, "admin_tasks", "后台任务", formatTaskLine)
		keyboard = adminKeyboard()
	case query.Data == "admin:risks":
		reply = b.collectionReply(ctx, query.From.ID, "admin_risks", "风险事件", formatRiskLine)
		keyboard = adminKeyboard()
	case strings.HasPrefix(query.Data, "request:"):
		reply = b.createMediaRequestReply(ctx, query.From.ID, strings.TrimPrefix(query.Data, "request:"), fmt.Sprintf("tg-callback-%d", item.ID))
		keyboard = mainKeyboard(false)
	default:
		reply = "按钮已失效，请发送 /menu 重新打开。"
		keyboard = mainKeyboard(false)
	}
	_ = b.sendMessageWithKeyboard(ctx, token, query.Message.Chat.ID, reply, keyboard)
}

func (b *Bot) dashboardReply(ctx context.Context, telegramID int64) (string, *inlineKeyboard) {
	var dashboard map[string]any
	if err := b.botAction(ctx, telegramID, "dashboard", nil, "", &dashboard); err != nil {
		return "尚未绑定统一账号。请先在 Web 用户中心生成绑定码，再发送：\n/bind 绑定码", mainKeyboard(false)
	}
	var access map[string]any
	_ = b.botAction(ctx, telegramID, "context", nil, "", &access)
	account, _ := access["account"].(map[string]any)
	membership, _ := dashboard["membership"].(map[string]any)
	wallet, _ := dashboard["wallet"].(map[string]any)
	bindings, _ := dashboard["emby_bindings"].([]any)
	administrator := false
	if permissions, ok := access["permissions"].([]any); ok {
		for _, permission := range permissions {
			if fmt.Sprint(permission) == "dashboard.read" {
				administrator = true
			}
		}
	}
	name := stringField(account, "display_name")
	if name == "" {
		name = "Sakura 用户"
	}
	plan := stringField(membership, "plan_name")
	if plan == "" {
		plan = "暂无会员"
	}
	reply := fmt.Sprintf("🌸 %s，欢迎回来\n\n会员：%s\n积分：%s\nEmby 账号：%d\n\nWeb 与 Bot 使用同一业务 API，操作结果会实时同步。", name, plan, numberText(wallet["balance"]), len(bindings))
	return reply, mainKeyboard(administrator)
}

func (b *Bot) adminDashboardReply(ctx context.Context, telegramID int64) string {
	var dashboard map[string]any
	if err := b.botAction(ctx, telegramID, "admin_dashboard", nil, "", &dashboard); err != nil {
		return "没有后台管理权限，或管理 API 暂时不可用。"
	}
	return fmt.Sprintf("🛡 Sakura 管理中心\n\n后台任务：%d\n批量任务：%d\n自动化执行：%d\n风险事件：%d\n\n所有操作都会经过 RBAC 和审计记录。", sliceLength(dashboard["tasks"]), sliceLength(dashboard["batch_operations"]), sliceLength(dashboard["automation_executions"]), sliceLength(dashboard["risk_events"]))
}

func (b *Bot) createMediaRequestReply(ctx context.Context, telegramID int64, mediaID, key string) string {
	var result map[string]any
	err := b.botAction(ctx, telegramID, "media_request", map[string]string{"media_id": mediaID}, key, &result)
	if err != nil {
		return "求片提交失败：影片 ID 无效、影片已入库，或账号状态不允许操作。"
	}
	duplicate := boolField(result, "duplicate")
	message := "求片已提交"
	if duplicate {
		message = "已订阅现有求片，系统不会重复下载"
	}
	return fmt.Sprintf("%s\n编号：%s\n状态：%s", message, stringField(result, "request_no"), stringField(result, "status"))
}

func (b *Bot) collectionReply(ctx context.Context, telegramID int64, action, title string, format func(map[string]any) string) string {
	var items []map[string]any
	if err := b.botAction(ctx, telegramID, action, nil, "", &items); err != nil {
		return title + "查询失败：账号未绑定、权限不足或服务暂时不可用。"
	}
	if len(items) == 0 {
		return title + "\n\n暂无记录。"
	}
	if len(items) > 10 {
		items = items[:10]
	}
	lines := []string{title, ""}
	for _, item := range items {
		lines = append(lines, format(item))
	}
	return strings.Join(lines, "\n")
}

func formatMediaResults(items []map[string]any) (string, *inlineKeyboard) {
	if len(items) == 0 {
		return "没有找到匹配影片，请换一个关键词。", mainKeyboard(false)
	}
	if len(items) > 6 {
		items = items[:6]
	}
	lines := []string{"TMDB 影片搜索", ""}
	keyboard := &inlineKeyboard{}
	for index, item := range items {
		available := "可求片"
		if boolField(item, "available") {
			available = "已入库"
		}
		title := stringField(item, "title")
		lines = append(lines, fmt.Sprintf("%d. %s · %s · %s", index+1, title, stringField(item, "media_type"), available))
		if !boolField(item, "available") {
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []inlineButton{{Text: "求片 · " + truncateRunes(title, 22), CallbackData: "request:" + stringField(item, "id")}})
		}
	}
	keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []inlineButton{{Text: "返回首页", CallbackData: "nav:home"}})
	return strings.Join(lines, "\n"), keyboard
}

func formatRequestLine(item map[string]any) string {
	media, _ := item["media"].(map[string]any)
	return fmt.Sprintf("• %s｜%s｜%s", stringField(media, "title"), stringField(item, "status"), stringField(item, "request_no"))
}

func formatTicketLine(item map[string]any) string {
	return fmt.Sprintf("• %s｜%s｜%s", stringField(item, "subject"), stringField(item, "status"), stringField(item, "ticket_no"))
}

func formatNotificationLine(item map[string]any) string {
	return fmt.Sprintf("• %s：%s", stringField(item, "title"), truncateRunes(stringField(item, "body"), 45))
}

func formatAccountLine(item map[string]any) string {
	return fmt.Sprintf("• %s｜%s｜%s", stringField(item, "display_name"), stringField(item, "status"), stringField(item, "id"))
}

func formatTaskLine(item map[string]any) string {
	return fmt.Sprintf("• %s｜%s｜尝试 %s/%s", stringField(item, "task_type"), stringField(item, "status"), numberText(item["attempts"]), numberText(item["max_attempts"]))
}

func formatRiskLine(item map[string]any) string {
	return fmt.Sprintf("• %s｜%s｜%s", stringField(item, "severity"), stringField(item, "status"), truncateRunes(stringField(item, "title"), 35))
}

func formatEntitlementLine(item map[string]any) string {
	return fmt.Sprintf("• %s｜%s｜到期 %s", stringField(item, "resource_key"), stringField(item, "status"), stringField(item, "expires_at"))
}
func formatLineEntry(item map[string]any) string {
	return fmt.Sprintf("• %s｜%s｜%s", stringField(item, "name"), stringField(item, "last_status"), stringField(item, "base_url"))
}
func formatFavoriteLine(item map[string]any) string {
	return fmt.Sprintf("• %s｜%s｜%s", stringField(item, "title"), stringField(item, "instance_name"), stringField(item, "sync_status"))
}

func mainKeyboard(admin bool) *inlineKeyboard {
	rows := [][]inlineButton{
		{{Text: "我的首页", CallbackData: "nav:home"}, {Text: "我的求片", CallbackData: "nav:requests"}},
		{{Text: "我的工单", CallbackData: "nav:tickets"}, {Text: "通知", CallbackData: "nav:notifications"}},
		{{Text: "权益", CallbackData: "nav:access"}, {Text: "收藏", CallbackData: "nav:favorites"}},
	}
	if admin {
		rows = append(rows, []inlineButton{{Text: "管理中心", CallbackData: "nav:admin"}})
	}
	return &inlineKeyboard{InlineKeyboard: rows}
}

func adminKeyboard() *inlineKeyboard {
	return &inlineKeyboard{InlineKeyboard: [][]inlineButton{
		{{Text: "账号", CallbackData: "admin:users"}, {Text: "任务", CallbackData: "admin:tasks"}},
		{{Text: "风险", CallbackData: "admin:risks"}, {Text: "用户首页", CallbackData: "nav:home"}},
	}}
}

func botHelpText() string {
	return "Sakura Bot 命令\n\n/start - 打开功能菜单\n/bind <绑定码> - 绑定 Web 账号\n/media <片名> - 搜索影片\n/request <影片ID> - 提交求片\n/requests - 我的求片\n/tickets - 我的工单\n/ticket 主题 | 描述 - 创建工单\n/notifications - 未读通知\n/entitlements - 我的权益\n/lines - 可用线路\n/redeem <权益码> - 兑换权益\n/favorites - Emby 收藏\n/favorite-sync <绑定ID> - 同步收藏\n/like /unlike <影评ID> - 点赞操作\n/report-review - 举报影评\n/register - 创建 Emby 账号\n/register-status - 查询建号任务\n\n管理员：/admin /users /tasks /risks /broadcast"
}

func stringField(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	value := item[key]
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func boolField(item map[string]any, key string) bool {
	value, _ := item[key].(bool)
	return value
}

func numberText(value any) string {
	if value == nil {
		return "0"
	}
	return strings.TrimSuffix(fmt.Sprint(value), ".0")
}

func sliceLength(value any) int {
	items, _ := value.([]any)
	return len(items)
}

func truncateRunes(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum]) + "…"
}

func (b *Bot) botAction(ctx context.Context, telegramID int64, action string, arguments map[string]string, key string, target any) error {
	body, _ := json.Marshal(map[string]any{"telegram_user_id": telegramID, "action": action, "arguments": arguments, "idempotency_key": key})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config.InternalAPIURL+"/api/v3/internal/bot/actions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+b.config.InternalAPIToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := b.client.Do(request)
	if err != nil {
		return safeNetworkError("Bot business API", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Bot business API returned status %d", response.StatusCode)
	}
	if target == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(target)
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
	return b.sendMessageWithKeyboard(ctx, token, chatID, text, nil)
}

func (b *Bot) sendMessageWithKeyboard(ctx context.Context, token string, chatID int64, text string, keyboard *inlineKeyboard) error {
	payload := map[string]any{"chat_id": chatID, "text": text}
	if keyboard != nil && len(keyboard.InlineKeyboard) > 0 {
		payload["reply_markup"] = keyboard
	}
	body, _ := json.Marshal(payload)
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

func (b *Bot) answerCallback(ctx context.Context, token, callbackID string) error {
	body, _ := json.Marshal(map[string]any{"callback_query_id": callbackID})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config.APIBase+"/bot"+token+"/answerCallbackQuery", bytes.NewReader(body))
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
		return fmt.Errorf("Telegram answerCallbackQuery returned status %d", response.StatusCode)
	}
	return nil
}

func (b *Bot) setCommands(ctx context.Context, token string) error {
	commands := []map[string]string{
		{"command": "start", "description": "打开 Sakura 功能菜单"},
		{"command": "media", "description": "搜索影片"},
		{"command": "requests", "description": "查看我的求片"},
		{"command": "tickets", "description": "查看我的工单"},
		{"command": "notifications", "description": "查看未读通知"},
		{"command": "entitlements", "description": "查看媒体库权益"},
		{"command": "lines", "description": "查看可用线路"},
		{"command": "favorites", "description": "查看 Emby 收藏"},
		{"command": "register", "description": "创建 Emby 账号"},
		{"command": "admin", "description": "打开管理中心"},
		{"command": "help", "description": "查看完整命令帮助"},
	}
	body, _ := json.Marshal(map[string]any{"commands": commands})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, b.config.APIBase+"/bot"+token+"/setMyCommands", bytes.NewReader(body))
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
		return fmt.Errorf("Telegram setMyCommands returned status %d", response.StatusCode)
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
