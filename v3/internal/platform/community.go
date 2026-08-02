package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

var notificationEventCatalog = []string{"media_request.status", "ticket.reply", "review.moderated", "broadcast.general", "automation.notification"}

func (s *Service) CreateTicket(ctx context.Context, accountID uuid.UUID, subject, category, priority, body string, actor identity.Actor) (Ticket, error) {
	subject, category, priority, body = strings.TrimSpace(subject), normalize(category), normalize(priority), strings.TrimSpace(body)
	if priority == "" {
		priority = "normal"
	}
	if subject == "" || len(subject) > 200 || category == "" || len(category) > 40 || !contains([]string{"low", "normal", "high", "urgent"}, priority) || body == "" || len(body) > 8000 {
		return Ticket{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Ticket{}, err
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	number := "TKT-" + time.Now().UTC().Format("20060102") + "-" + strings.ToUpper(id.String()[:8])
	_, err = tx.Exec(ctx, `INSERT INTO support_tickets(id,ticket_no,account_id,subject,category,priority,status,last_public_reply_at) VALUES($1,$2,$3,$4,$5,$6,'open',NOW())`, id, number, accountID, subject, category, priority)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO ticket_messages(id,ticket_id,author_account_id,author_label,body,is_internal) VALUES($1,$2,$3,$4,$5,FALSE)`, uuid.New(), id, accountID, actor.Label(), body)
	}
	if err != nil {
		return Ticket{}, err
	}
	if err = emitAutomationEventTx(ctx, tx, "ticket.created:"+id.String(), "ticket.created", "ticket", id.String(), map[string]any{"ticket_id": id.String(), "account_id": accountID.String(), "category": category, "priority": priority}); err != nil {
		return Ticket{}, err
	}
	if err = audit(ctx, tx, actor, "ticket.create", "ticket", id.String(), map[string]any{"category": category, "priority": priority}); err != nil {
		return Ticket{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}
	return s.GetTicket(ctx, id, &accountID)
}

func scanTicket(row rowScanner) (Ticket, error) {
	var item Ticket
	err := row.Scan(&item.ID, &item.TicketNo, &item.AccountID, &item.Subject, &item.Category, &item.Priority, &item.Status, &item.AssignedTo, &item.LastPublicReplyAt, &item.LastInternalNoteAt, &item.Revision, &item.CreatedAt, &item.UpdatedAt)
	return item, notFound(err)
}

const ticketSelect = `SELECT id,ticket_no,account_id,subject,category,priority,status,assigned_to,last_public_reply_at,last_internal_note_at,revision,created_at,updated_at FROM support_tickets`

func (s *Service) GetTicket(ctx context.Context, id uuid.UUID, owner *uuid.UUID) (Ticket, error) {
	return scanTicket(s.db.QueryRow(ctx, ticketSelect+` WHERE id=$1 AND ($2::uuid IS NULL OR account_id=$2)`, id, uuidQueryValue(owner)))
}

func (s *Service) ListTickets(ctx context.Context, owner *uuid.UUID, status string, limit int) ([]Ticket, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, ticketSelect+` WHERE ($1::uuid IS NULL OR account_id=$1) AND ($2='' OR status=$2) ORDER BY updated_at DESC LIMIT $3`, uuidQueryValue(owner), status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ticket
	for rows.Next() {
		item, scanErr := scanTicket(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListTicketMessages(ctx context.Context, ticketID uuid.UUID, owner *uuid.UUID, includeInternal bool) ([]TicketMessage, error) {
	if owner != nil {
		includeInternal = false
	}
	rows, err := s.db.Query(ctx, `SELECT m.id,m.ticket_id,m.author_account_id,m.author_label,m.body,m.is_internal,m.attachments,m.created_at FROM ticket_messages m JOIN support_tickets t ON t.id=m.ticket_id WHERE m.ticket_id=$1 AND ($2::uuid IS NULL OR t.account_id=$2) AND ($3 OR NOT m.is_internal) ORDER BY m.created_at,m.id`, ticketID, uuidQueryValue(owner), includeInternal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TicketMessage
	for rows.Next() {
		var item TicketMessage
		var raw []byte
		if err = rows.Scan(&item.ID, &item.TicketID, &item.AuthorAccountID, &item.AuthorLabel, &item.Body, &item.Internal, &raw, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = jsonUnmarshalArray(raw, &item.Attachments)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) AddTicketMessage(ctx context.Context, ticketID uuid.UUID, accountID *uuid.UUID, body string, internal, staff bool, actor identity.Actor) (TicketMessage, error) {
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 8000 || internal && !staff {
		return TicketMessage{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return TicketMessage{}, err
	}
	defer tx.Rollback(ctx)
	var ownerID uuid.UUID
	var status string
	if err = tx.QueryRow(ctx, `SELECT account_id,status FROM support_tickets WHERE id=$1 FOR UPDATE`, ticketID).Scan(&ownerID, &status); err != nil {
		return TicketMessage{}, notFound(err)
	}
	if !staff && (accountID == nil || *accountID != ownerID) {
		return TicketMessage{}, identity.ErrForbidden
	}
	if status == "closed" {
		return TicketMessage{}, identity.ErrConflict
	}
	id := uuid.New()
	_, err = tx.Exec(ctx, `INSERT INTO ticket_messages(id,ticket_id,author_account_id,author_label,body,is_internal) VALUES($1,$2,$3,$4,$5,$6)`, id, ticketID, accountID, actor.Label(), body, internal)
	if err != nil {
		return TicketMessage{}, err
	}
	if internal {
		_, err = tx.Exec(ctx, `UPDATE support_tickets SET last_internal_note_at=NOW(),revision=revision+1,updated_at=NOW() WHERE id=$1`, ticketID)
	} else {
		next := "waiting_staff"
		if staff {
			next = "waiting_user"
		}
		_, err = tx.Exec(ctx, `UPDATE support_tickets SET status=$2,last_public_reply_at=NOW(),revision=revision+1,updated_at=NOW() WHERE id=$1`, ticketID, next)
		if err == nil && staff {
			err = queuePreferredNotificationTx(ctx, tx, ownerID, "ticket.reply", "工单有新回复", body, map[string]any{"ticket_id": ticketID})
		}
	}
	if err != nil {
		return TicketMessage{}, err
	}
	if err = emitAutomationEventTx(ctx, tx, "ticket.message:"+id.String(), "ticket.replied", "ticket", ticketID.String(), map[string]any{"ticket_id": ticketID.String(), "account_id": ownerID.String(), "internal": internal, "staff": staff}); err != nil {
		return TicketMessage{}, err
	}
	if err = audit(ctx, tx, actor, map[bool]string{true: "ticket.internal_note", false: "ticket.reply"}[internal], "ticket", ticketID.String(), map[string]any{"message_id": id}); err != nil {
		return TicketMessage{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return TicketMessage{}, err
	}
	messages, err := s.ListTicketMessages(ctx, ticketID, nil, true)
	if err != nil {
		return TicketMessage{}, err
	}
	for _, message := range messages {
		if message.ID == id {
			return message, nil
		}
	}
	return TicketMessage{}, identity.ErrNotFound
}

func (s *Service) UpdateTicket(ctx context.Context, id uuid.UUID, status, priority string, assignedTo *uuid.UUID, expected int64, actor identity.Actor) (Ticket, error) {
	status, priority = normalize(status), normalize(priority)
	if !contains([]string{"open", "waiting_user", "waiting_staff", "resolved", "closed"}, status) || !contains([]string{"low", "normal", "high", "urgent"}, priority) {
		return Ticket{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Ticket{}, err
	}
	defer tx.Rollback(ctx)
	var revision int64
	if err = tx.QueryRow(ctx, `SELECT revision FROM support_tickets WHERE id=$1 FOR UPDATE`, id).Scan(&revision); err != nil {
		return Ticket{}, notFound(err)
	}
	if revision != expected {
		return Ticket{}, identity.ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE support_tickets SET status=$2::varchar,priority=$3,assigned_to=$4,resolved_at=CASE WHEN $2::varchar='resolved' THEN NOW() ELSE resolved_at END,closed_at=CASE WHEN $2::varchar='closed' THEN NOW() ELSE closed_at END,revision=revision+1,updated_at=NOW() WHERE id=$1`, id, status, priority, assignedTo)
	if err != nil {
		return Ticket{}, err
	}
	if err = audit(ctx, tx, actor, "ticket.update", "ticket", id.String(), map[string]any{"status": status, "priority": priority, "assigned_to": assignedTo}); err != nil {
		return Ticket{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}
	return s.GetTicket(ctx, id, nil)
}

func (s *Service) SubmitReview(ctx context.Context, accountID, mediaID uuid.UUID, rating int, title, body string, spoilers bool, actor identity.Actor) (Review, error) {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if rating < 1 || rating > 10 || len(title) > 200 || body == "" || len(body) > 6000 {
		return Review{}, identity.ErrInvalid
	}
	requireModeration := true
	_ = s.db.QueryRow(ctx, `SELECT (value #>> '{}')::boolean FROM dynamic_settings WHERE key='reviews.require_moderation'`).Scan(&requireModeration)
	status := "approved"
	if requireModeration {
		status = "pending"
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Review{}, err
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	err = tx.QueryRow(ctx, `INSERT INTO media_reviews(id,media_id,account_id,rating,title,body,contains_spoilers,status) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8) ON CONFLICT(media_id,account_id) DO UPDATE SET rating=EXCLUDED.rating,title=EXCLUDED.title,body=EXCLUDED.body,contains_spoilers=EXCLUDED.contains_spoilers,status=$8,moderation_reason=NULL,moderated_by=NULL,moderated_at=NULL,revision=media_reviews.revision+1,updated_at=NOW() RETURNING id`, id, mediaID, accountID, rating, title, body, spoilers, status).Scan(&id)
	if err != nil {
		return Review{}, notFound(err)
	}
	if err = emitAutomationEventTx(ctx, tx, "review.submitted:"+id.String()+":"+fmt.Sprint(time.Now().UnixNano()), "review.submitted", "review", id.String(), map[string]any{"review_id": id.String(), "media_id": mediaID.String(), "account_id": accountID.String(), "rating": rating}); err != nil {
		return Review{}, err
	}
	if err = audit(ctx, tx, actor, "review.submit", "review", id.String(), map[string]any{"status": status, "rating": rating}); err != nil {
		return Review{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Review{}, err
	}
	return s.GetReview(ctx, id, true)
}

func (s *Service) GetReview(ctx context.Context, id uuid.UUID, includeUnapproved bool) (Review, error) {
	var item Review
	var mediaID uuid.UUID
	err := s.db.QueryRow(ctx, `SELECT r.id,r.media_id,r.account_id,r.rating,COALESCE(r.title,''),r.body,r.contains_spoilers,r.status,COALESCE(r.moderation_reason,''),COALESCE(r.moderated_by,''),r.moderated_at,r.revision,r.created_at,r.updated_at,(SELECT COUNT(*) FROM review_reactions x WHERE x.review_id=r.id),(SELECT COUNT(*) FROM review_reports p WHERE p.review_id=r.id) FROM media_reviews r WHERE r.id=$1 AND ($2 OR r.status='approved')`, id, includeUnapproved).Scan(&item.ID, &mediaID, &item.AccountID, &item.Rating, &item.Title, &item.Body, &item.ContainsSpoilers, &item.Status, &item.ModerationReason, &item.ModeratedBy, &item.ModeratedAt, &item.Revision, &item.CreatedAt, &item.UpdatedAt, &item.LikeCount, &item.ReportCount)
	if err != nil {
		return Review{}, notFound(err)
	}
	item.Media, err = s.GetMedia(ctx, mediaID)
	return item, err
}

func (s *Service) ListReviews(ctx context.Context, mediaID *uuid.UUID, status string, includeUnapproved bool, limit int) ([]Review, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	if !includeUnapproved {
		status = "approved"
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM media_reviews WHERE ($1::uuid IS NULL OR media_id=$1) AND ($2='' OR status=$2) ORDER BY created_at DESC LIMIT $3`, uuidQueryValue(mediaID), status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Review
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		item, getErr := s.GetReview(ctx, id, includeUnapproved)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ListAccountReviews(ctx context.Context, accountID uuid.UUID, limit int) ([]Review, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM media_reviews WHERE account_id=$1 ORDER BY updated_at DESC LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Review, 0, len(ids))
	for _, id := range ids {
		item, getErr := s.GetReview(ctx, id, true)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Service) ModerateReview(ctx context.Context, id uuid.UUID, status, reason string, expected int64, actor identity.Actor) (Review, error) {
	status, reason = normalize(status), strings.TrimSpace(reason)
	if !contains([]string{"approved", "rejected", "hidden"}, status) || reason == "" || len(reason) > 1000 {
		return Review{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Review{}, err
	}
	defer tx.Rollback(ctx)
	var revision int64
	var accountID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT revision,account_id FROM media_reviews WHERE id=$1 FOR UPDATE`, id).Scan(&revision, &accountID); err != nil {
		return Review{}, notFound(err)
	}
	if revision != expected {
		return Review{}, identity.ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE media_reviews SET status=$2,moderation_reason=$3,moderated_by=$4,moderated_at=NOW(),revision=revision+1,updated_at=NOW() WHERE id=$1`, id, status, reason, actor.Label())
	if err == nil {
		err = queuePreferredNotificationTx(ctx, tx, accountID, "review.moderated", "影评审核结果", reason, map[string]any{"review_id": id, "status": status})
	}
	if err != nil {
		return Review{}, err
	}
	if err = audit(ctx, tx, actor, "review.moderate", "review", id.String(), map[string]any{"status": status, "reason": reason}); err != nil {
		return Review{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Review{}, err
	}
	return s.GetReview(ctx, id, true)
}

func (s *Service) NotificationPreferences(ctx context.Context, accountID uuid.UUID) ([]NotificationPreference, error) {
	rows, err := s.db.Query(ctx, `SELECT event_key,channel,enabled FROM notification_preferences WHERE account_id=$1`, accountID)
	if err != nil {
		return nil, err
	}
	stored := map[string]bool{}
	for rows.Next() {
		var event, channel string
		var enabled bool
		if err = rows.Scan(&event, &channel, &enabled); err != nil {
			rows.Close()
			return nil, err
		}
		stored[event+":"+channel] = enabled
	}
	rows.Close()
	var out []NotificationPreference
	for _, event := range notificationEventCatalog {
		for _, channel := range []string{"in_app", "telegram"} {
			enabled, ok := stored[event+":"+channel]
			if !ok {
				enabled = true
			}
			out = append(out, NotificationPreference{EventKey: event, Channel: channel, Enabled: enabled})
		}
	}
	return out, nil
}

func (s *Service) SetNotificationPreference(ctx context.Context, accountID uuid.UUID, eventKey, channel string, enabled bool, actor identity.Actor) (NotificationPreference, error) {
	eventKey, channel = normalize(eventKey), normalize(channel)
	if eventKey == "" || len(eventKey) > 80 || !contains([]string{"in_app", "telegram"}, channel) {
		return NotificationPreference{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return NotificationPreference{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO notification_preferences(account_id,event_key,channel,enabled) VALUES($1,$2,$3,$4) ON CONFLICT(account_id,event_key,channel) DO UPDATE SET enabled=EXCLUDED.enabled,updated_at=NOW()`, accountID, eventKey, channel, enabled)
	if err != nil {
		return NotificationPreference{}, err
	}
	if err = audit(ctx, tx, actor, "notification.preference", "account", accountID.String(), map[string]any{"event_key": eventKey, "channel": channel, "enabled": enabled}); err != nil {
		return NotificationPreference{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return NotificationPreference{}, err
	}
	return NotificationPreference{EventKey: eventKey, Channel: channel, Enabled: enabled}, nil
}

func notificationAllowedTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, eventKey, channel string) (bool, error) {
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT enabled FROM notification_preferences WHERE account_id=$1 AND event_key=$2 AND channel=$3`, accountID, eventKey, channel).Scan(&enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	return enabled, err
}

func queuePreferredNotificationTx(ctx context.Context, tx pgx.Tx, accountID uuid.UUID, eventKey, title, body string, metadata map[string]any) error {
	notificationMetadata := make(map[string]any, len(metadata)+1)
	for key, value := range metadata {
		notificationMetadata[key] = value
	}
	notificationMetadata["event_key"] = eventKey
	for _, channel := range []string{"in_app", "telegram"} {
		enabled, err := notificationAllowedTx(ctx, tx, accountID, eventKey, channel)
		if err != nil || !enabled {
			if err != nil {
				return err
			}
			continue
		}
		if channel == "telegram" {
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM account_identities WHERE account_id=$1 AND kind='telegram' AND NOT disabled)`, accountID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				continue
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO account_notifications(id,account_id,title,body,channel,delivery_status,metadata) VALUES($1,$2,$3,$4,$5,CASE WHEN $5='in_app' THEN 'sent' ELSE 'pending' END,$6)`, uuid.New(), accountID, title, body, channel, jsonBytes(notificationMetadata))
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) CreateBroadcast(ctx context.Context, title, body, eventKey, channel string, target BatchTarget, idempotencyKey string, actor identity.Actor) (Broadcast, error) {
	title, body, eventKey, channel = strings.TrimSpace(title), strings.TrimSpace(body), normalize(eventKey), normalize(channel)
	if title == "" || len(title) > 160 || body == "" || len(body) > 4000 || eventKey == "" || len(eventKey) > 80 || !contains([]string{"in_app", "telegram"}, channel) {
		return Broadcast{}, identity.ErrInvalid
	}
	batch, err := s.CreateBatch(ctx, "notification", target, map[string]any{"title": title, "body": body, "channel": channel, "event_key": eventKey, "broadcast": true}, "broadcast:"+idempotencyKey, 3, actor)
	if err != nil {
		return Broadcast{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Broadcast{}, err
	}
	defer tx.Rollback(ctx)
	id := uuid.New()
	if err = tx.QueryRow(ctx, `INSERT INTO broadcasts(id,batch_operation_id,title,body,event_key,channel,target_spec,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(batch_operation_id) DO UPDATE SET batch_operation_id=EXCLUDED.batch_operation_id RETURNING id`, id, batch.ID, title, body, eventKey, channel, jsonBytes(map[string]any{"account_ids": target.AccountIDs, "status": target.Status, "tag_ids": target.TagIDs}), actor.Label()).Scan(&id); err != nil {
		return Broadcast{}, err
	}
	if err = audit(ctx, tx, actor, "broadcast.create", "broadcast", id.String(), map[string]any{"batch_operation_id": batch.ID, "channel": channel}); err != nil {
		return Broadcast{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Broadcast{}, err
	}
	return s.GetBroadcast(ctx, id)
}

func (s *Service) GetBroadcast(ctx context.Context, id uuid.UUID) (Broadcast, error) {
	var item Broadcast
	var batchID uuid.UUID
	var raw []byte
	err := s.db.QueryRow(ctx, `SELECT id,batch_operation_id,title,body,event_key,channel,target_spec,created_by,created_at FROM broadcasts WHERE id=$1`, id).Scan(&item.ID, &batchID, &item.Title, &item.Body, &item.EventKey, &item.Channel, &raw, &item.CreatedBy, &item.CreatedAt)
	if err != nil {
		return Broadcast{}, notFound(err)
	}
	item.TargetSpec = decodeJSON(raw)
	item.BatchOperation, err = s.GetBatch(ctx, batchID)
	return item, err
}

func (s *Service) ListBroadcasts(ctx context.Context, limit int) ([]Broadcast, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM broadcasts ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Broadcast
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		item, getErr := s.GetBroadcast(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SaveAutomationRule(ctx context.Context, id *uuid.UUID, item AutomationRule, expected int64, actor identity.Actor) (AutomationRule, error) {
	item.Code, item.Name, item.TriggerEvent = normalize(item.Code), strings.TrimSpace(item.Name), normalize(item.TriggerEvent)
	if item.Code == "" || len(item.Code) > 80 || item.Name == "" || len(item.Name) > 160 || item.TriggerEvent == "" || len(item.TriggerEvent) > 80 || len(item.Actions) == 0 || item.Priority < 0 || item.Priority > 100000 || !validAutomationActions(item.Actions) {
		return AutomationRule{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return AutomationRule{}, err
	}
	defer tx.Rollback(ctx)
	ruleID := uuid.New()
	action := "automation.create"
	if id == nil {
		_, err = tx.Exec(ctx, `INSERT INTO automation_rules(id,code,name,description,trigger_event,conditions,actions,enabled,priority,created_by) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10)`, ruleID, item.Code, item.Name, item.Description, item.TriggerEvent, jsonBytes(item.Conditions), jsonBytes(item.Actions), item.Enabled, item.Priority, actor.Label())
	} else {
		ruleID = *id
		var revision int64
		if err = tx.QueryRow(ctx, `SELECT revision FROM automation_rules WHERE id=$1 FOR UPDATE`, ruleID).Scan(&revision); err != nil {
			return AutomationRule{}, notFound(err)
		}
		if revision != expected {
			return AutomationRule{}, identity.ErrConflict
		}
		_, err = tx.Exec(ctx, `UPDATE automation_rules SET code=$2,name=$3,description=NULLIF($4,''),trigger_event=$5,conditions=$6,actions=$7,enabled=$8,priority=$9,revision=revision+1,updated_at=NOW() WHERE id=$1`, ruleID, item.Code, item.Name, item.Description, item.TriggerEvent, jsonBytes(item.Conditions), jsonBytes(item.Actions), item.Enabled, item.Priority)
		action = "automation.update"
	}
	if err != nil {
		return AutomationRule{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, action, "automation_rule", ruleID.String(), map[string]any{"trigger_event": item.TriggerEvent}); err != nil {
		return AutomationRule{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AutomationRule{}, err
	}
	return s.GetAutomationRule(ctx, ruleID)
}

func validAutomationActions(actions []map[string]any) bool {
	for _, action := range actions {
		if !contains([]string{"notify_account", "submit_moviepilot", "set_request_status"}, normalize(fmt.Sprint(action["type"]))) {
			return false
		}
	}
	return true
}

func scanAutomationRule(row rowScanner) (AutomationRule, error) {
	var item AutomationRule
	var conditions, actions []byte
	err := row.Scan(&item.ID, &item.Code, &item.Name, &item.Description, &item.TriggerEvent, &conditions, &actions, &item.Enabled, &item.Priority, &item.Revision, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	item.Conditions = decodeJSON(conditions)
	_ = jsonUnmarshalActions(actions, &item.Actions)
	return item, notFound(err)
}

const automationRuleSelect = `SELECT id,code,name,COALESCE(description,''),trigger_event,conditions,actions,enabled,priority,revision,created_by,created_at,updated_at FROM automation_rules`

func (s *Service) GetAutomationRule(ctx context.Context, id uuid.UUID) (AutomationRule, error) {
	return scanAutomationRule(s.db.QueryRow(ctx, automationRuleSelect+` WHERE id=$1`, id))
}

func (s *Service) ListAutomationRules(ctx context.Context) ([]AutomationRule, error) {
	rows, err := s.db.Query(ctx, automationRuleSelect+` ORDER BY priority,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutomationRule
	for rows.Next() {
		item, scanErr := scanAutomationRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ProcessNextAutomation(ctx context.Context, workerID string, lease time.Duration) (bool, error) {
	if lease < 30*time.Second {
		lease = 90 * time.Second
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var eventID uuid.UUID
	var eventType string
	var payloadRaw []byte
	var attempts, maximum int
	err = tx.QueryRow(ctx, `SELECT id,event_type,payload,attempts+1,max_attempts FROM automation_events WHERE ((status IN ('pending','failed') AND available_at<=NOW()) OR (status='running' AND lease_expires_at<NOW())) AND attempts<max_attempts ORDER BY available_at,created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&eventID, &eventType, &payloadRaw, &attempts, &maximum)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE automation_events SET status='running',attempts=$2,lease_owner=$3,lease_expires_at=NOW()+($4::double precision*INTERVAL '1 second'),updated_at=NOW() WHERE id=$1`, eventID, attempts, workerID, lease.Seconds())
	if err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	payload := decodeJSON(payloadRaw)
	processErr := s.executeAutomationEvent(ctx, eventID, eventType, payload, workerID)
	if processErr == nil {
		_, err = s.db.Exec(ctx, `UPDATE automation_events SET status='succeeded',lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,finished_at=NOW(),updated_at=NOW() WHERE id=$1 AND lease_owner=$2`, eventID, workerID)
		return true, err
	}
	var permanentErr PermanentError
	if errors.As(processErr, &permanentErr) {
		attempts = maximum
	}
	delay := math.Min(math.Pow(2, float64(attempts)), 300)
	_, err = s.db.Exec(ctx, `UPDATE automation_events SET status='failed',attempts=$4,lease_owner=NULL,lease_expires_at=NULL,last_error=$2,available_at=NOW()+($3::double precision*INTERVAL '1 second'),finished_at=CASE WHEN $4>=$5 THEN NOW() ELSE NULL END,updated_at=NOW() WHERE id=$1`, eventID, truncateError(processErr), delay, attempts, maximum)
	if err != nil {
		return true, err
	}
	return true, nil
}

func (s *Service) executeAutomationEvent(ctx context.Context, eventID uuid.UUID, eventType string, payload map[string]any, workerID string) error {
	rules, err := s.ListAutomationRules(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if !rule.Enabled || rule.TriggerEvent != eventType {
			continue
		}
		tx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return beginErr
		}
		var previousStatus string
		beginErr = tx.QueryRow(ctx, `SELECT status FROM automation_executions WHERE event_id=$1 AND rule_id=$2`, eventID, rule.ID).Scan(&previousStatus)
		if beginErr != nil && !errors.Is(beginErr, pgx.ErrNoRows) {
			tx.Rollback(ctx)
			return beginErr
		}
		if previousStatus == "succeeded" || previousStatus == "skipped" {
			tx.Rollback(ctx)
			continue
		}
		if previousStatus == "failed" {
			if _, beginErr = tx.Exec(ctx, `DELETE FROM automation_executions WHERE event_id=$1 AND rule_id=$2 AND status='failed'`, eventID, rule.ID); beginErr != nil {
				tx.Rollback(ctx)
				return beginErr
			}
		}
		if !automationConditionsMatch(rule.Conditions, payload) {
			_, beginErr = tx.Exec(ctx, `INSERT INTO automation_executions(event_id,rule_id,status,result) VALUES($1,$2,'skipped',$3)`, eventID, rule.ID, jsonBytes(map[string]any{"reason": "conditions_not_met"}))
			if beginErr == nil {
				beginErr = tx.Commit(ctx)
			} else {
				tx.Rollback(ctx)
			}
			if beginErr != nil {
				return beginErr
			}
			continue
		}
		result := map[string]any{"actions": len(rule.Actions)}
		for _, action := range rule.Actions {
			if beginErr = executeAutomationActionTx(ctx, tx, eventID, rule, action, payload, workerID); beginErr != nil {
				tx.Rollback(ctx)
				_, _ = s.db.Exec(ctx, `INSERT INTO automation_executions(event_id,rule_id,status,result,error_message) VALUES($1,$2,'failed','{}'::jsonb,$3) ON CONFLICT(event_id,rule_id) DO UPDATE SET status='failed',result='{}'::jsonb,error_message=EXCLUDED.error_message,created_at=NOW()`, eventID, rule.ID, truncateError(beginErr))
				return beginErr
			}
		}
		if _, beginErr = tx.Exec(ctx, `INSERT INTO automation_executions(event_id,rule_id,status,result) VALUES($1,$2,'succeeded',$3)`, eventID, rule.ID, jsonBytes(result)); beginErr != nil {
			tx.Rollback(ctx)
			return beginErr
		}
		if beginErr = tx.Commit(ctx); beginErr != nil {
			return beginErr
		}
	}
	return nil
}

func automationConditionsMatch(conditions, payload map[string]any) bool {
	for key, expected := range conditions {
		if fmt.Sprint(payload[key]) != fmt.Sprint(expected) {
			return false
		}
	}
	return true
}

func executeAutomationActionTx(ctx context.Context, tx pgx.Tx, eventID uuid.UUID, rule AutomationRule, action, payload map[string]any, workerID string) error {
	typeName := normalize(fmt.Sprint(action["type"]))
	switch typeName {
	case "notify_account":
		accountID, err := automationUUID(action["account_id"], payload, "account_id")
		if err != nil {
			return PermanentError{Err: err}
		}
		title, body := strings.TrimSpace(fmt.Sprint(action["title"])), strings.TrimSpace(fmt.Sprint(action["body"]))
		if title == "" || title == "<nil>" || body == "" || body == "<nil>" || len(title) > 160 || len(body) > 4000 {
			return PermanentError{Err: identity.ErrInvalid}
		}
		return queuePreferredNotificationTx(ctx, tx, accountID, stringOr(action["event_key"], "automation.notification"), title, body, map[string]any{"automation_rule_id": rule.ID, "automation_event_id": eventID})
	case "submit_moviepilot":
		requestID, err := automationUUID(action["request_id"], payload, "media_request_id")
		if err != nil {
			return PermanentError{Err: err}
		}
		var mediaID uuid.UUID
		if err = tx.QueryRow(ctx, `SELECT media_id FROM media_requests WHERE id=$1`, requestID).Scan(&mediaID); err != nil {
			return notFound(err)
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM moviepilot_jobs WHERE media_id=$1 AND status IN ('pending','submitting','submitted','downloading','completed'))`, mediaID).Scan(&exists); err != nil || exists {
			return err
		}
		jobID, taskID := uuid.New(), uuid.New()
		key := "automation:" + rule.ID.String() + ":" + eventID.String()
		resource, _ := action["resource"].(map[string]any)
		if _, err = tx.Exec(ctx, `INSERT INTO moviepilot_jobs(id,media_id,request_id,task_id,idempotency_key,payload,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`, jobID, mediaID, requestID, taskID, key, jsonBytes(map[string]any{"request_id": requestID, "resource": resource}), "system:"+workerID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,max_attempts,created_by) VALUES($1,'moviepilot.submit',$2,$3,8,$4)`, taskID, key, jsonBytes(map[string]any{"moviepilot_job_id": jobID.String()}), "system:"+workerID)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE media_requests SET status=CASE WHEN status IN ('requested','approved') THEN 'queued' ELSE status END,revision=revision+1,updated_at=NOW() WHERE id=$1`, requestID)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,to_status,actor,reason,details) VALUES($1,'automation_moviepilot_queued','queued',$2,'Automation queued MoviePilot',$3)`, requestID, "system:"+workerID, jsonBytes(map[string]any{"job_id": jobID, "rule_id": rule.ID}))
		}
		return err
	case "set_request_status":
		requestID, err := automationUUID(action["request_id"], payload, "media_request_id")
		if err != nil {
			return PermanentError{Err: err}
		}
		status := normalize(fmt.Sprint(action["status"]))
		if !contains([]string{"approved", "queued", "downloading", "completed", "rejected", "canceled"}, status) {
			return PermanentError{Err: identity.ErrInvalid}
		}
		var current string
		if err = tx.QueryRow(ctx, `SELECT status FROM media_requests WHERE id=$1 FOR UPDATE`, requestID).Scan(&current); err != nil {
			return notFound(err)
		}
		if current == status {
			return nil
		}
		if !validRequestTransition(current, status) {
			return PermanentError{Err: identity.ErrConflict}
		}
		reason := stringOr(action["reason"], "Automation rule: "+rule.Name)
		_, err = tx.Exec(ctx, `UPDATE media_requests SET status=$2::varchar,resolution_reason=$3,resolved_by=$4,resolved_at=CASE WHEN $2::varchar IN ('completed','rejected','canceled') THEN NOW() ELSE NULL END,revision=revision+1,updated_at=NOW() WHERE id=$1`, requestID, status, reason, "system:"+workerID)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO media_request_events(request_id,event_type,from_status,to_status,actor,reason,details) VALUES($1,'automation_status_changed',$2,$3,$4,$5,$6)`, requestID, current, status, "system:"+workerID, reason, jsonBytes(map[string]any{"rule_id": rule.ID, "event_id": eventID}))
		}
		if err == nil {
			err = notifyRequestSubscribersTx(ctx, tx, requestID, "media_request.status", "求片状态已更新", reason)
		}
		return err
	default:
		return PermanentError{Err: identity.ErrInvalid}
	}
}

func automationUUID(raw any, payload map[string]any, fallback string) (uuid.UUID, error) {
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "" || strings.HasPrefix(value, "$event.") {
		key := fallback
		if strings.HasPrefix(value, "$event.") {
			key = strings.TrimPrefix(value, "$event.")
		}
		value = fmt.Sprint(payload[key])
	}
	return uuid.Parse(value)
}

func stringOr(value any, fallback string) string {
	result := strings.TrimSpace(fmt.Sprint(value))
	if result == "" || result == "<nil>" {
		return fallback
	}
	return result
}

func (s *Service) ListAutomationExecutions(ctx context.Context, limit int) ([]AutomationExecution, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT x.id,x.event_id,x.rule_id,r.name,x.status,x.result,COALESCE(x.error_message,''),x.created_at FROM automation_executions x JOIN automation_rules r ON r.id=x.rule_id ORDER BY x.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutomationExecution
	for rows.Next() {
		var item AutomationExecution
		var raw []byte
		if err = rows.Scan(&item.ID, &item.EventID, &item.RuleID, &item.RuleName, &item.Status, &raw, &item.ErrorMessage, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Result = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func jsonUnmarshalArray(raw []byte, target *[]any) error {
	return json.Unmarshal(raw, target)
}

func jsonUnmarshalActions(raw []byte, target *[]map[string]any) error {
	return json.Unmarshal(raw, target)
}
