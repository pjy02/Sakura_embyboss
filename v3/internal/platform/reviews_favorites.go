package platform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pjy02/Sakura_embyboss/v3/internal/identity"
)

func (s *Service) SetReviewLike(ctx context.Context, reviewID, accountID uuid.UUID, liked bool, actor identity.Actor) (Review, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Review{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM media_reviews WHERE id=$1 FOR SHARE`, reviewID).Scan(&status); err != nil {
		return Review{}, notFound(err)
	}
	if status != "approved" {
		return Review{}, identity.ErrForbidden
	}
	if liked {
		_, err = tx.Exec(ctx, `INSERT INTO review_reactions(review_id,account_id,reaction) VALUES($1,$2,'like') ON CONFLICT DO NOTHING`, reviewID, accountID)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM review_reactions WHERE review_id=$1 AND account_id=$2 AND reaction='like'`, reviewID, accountID)
	}
	if err != nil {
		return Review{}, err
	}
	if err = audit(ctx, tx, actor, "review.like", "review", reviewID.String(), map[string]any{"liked": liked}); err != nil {
		return Review{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Review{}, err
	}
	return s.GetReview(ctx, reviewID, false)
}

func (s *Service) ReportReview(ctx context.Context, reviewID, accountID uuid.UUID, reason, detail string, actor identity.Actor) (ReviewReport, error) {
	reason, detail = normalize(reason), strings.TrimSpace(detail)
	if !contains([]string{"spam", "abuse", "spoiler", "copyright", "other"}, reason) || len(detail) > 1000 {
		return ReviewReport{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReviewReport{}, err
	}
	defer tx.Rollback(ctx)
	var owner uuid.UUID
	var status string
	if err = tx.QueryRow(ctx, `SELECT account_id,status FROM media_reviews WHERE id=$1 FOR SHARE`, reviewID).Scan(&owner, &status); err != nil {
		return ReviewReport{}, notFound(err)
	}
	if owner == accountID || status != "approved" {
		return ReviewReport{}, identity.ErrForbidden
	}
	id := uuid.New()
	err = tx.QueryRow(ctx, `INSERT INTO review_reports(id,review_id,reporter_account_id,reason,detail) VALUES($1,$2,$3,$4,NULLIF($5,'')) ON CONFLICT(review_id,reporter_account_id) DO UPDATE SET reason=EXCLUDED.reason,detail=EXCLUDED.detail,status='open',resolution=NULL,resolved_by=NULL,resolved_at=NULL RETURNING id`, id, reviewID, accountID, reason, detail).Scan(&id)
	if err != nil {
		return ReviewReport{}, err
	}
	if err = audit(ctx, tx, actor, "review.report", "review", reviewID.String(), map[string]any{"report_id": id, "reason": reason}); err != nil {
		return ReviewReport{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ReviewReport{}, err
	}
	return s.GetReviewReport(ctx, id)
}

func (s *Service) GetReviewReport(ctx context.Context, id uuid.UUID) (ReviewReport, error) {
	var item ReviewReport
	err := s.db.QueryRow(ctx, `SELECT id,review_id,reporter_account_id,reason,COALESCE(detail,''),status,COALESCE(resolution,''),COALESCE(resolved_by,''),resolved_at,created_at FROM review_reports WHERE id=$1`, id).Scan(&item.ID, &item.ReviewID, &item.ReporterAccountID, &item.Reason, &item.Detail, &item.Status, &item.Resolution, &item.ResolvedBy, &item.ResolvedAt, &item.CreatedAt)
	return item, notFound(err)
}

func (s *Service) ListReviewReports(ctx context.Context, status string, limit int) ([]ReviewReport, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT id FROM review_reports WHERE ($1='' OR status=$1) ORDER BY created_at DESC LIMIT $2`, normalize(status), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewReport
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		item, getErr := s.GetReviewReport(ctx, id)
		if getErr != nil {
			return nil, getErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) ResolveReviewReport(ctx context.Context, id uuid.UUID, status, resolution string, actor identity.Actor) (ReviewReport, error) {
	status, resolution = normalize(status), strings.TrimSpace(resolution)
	if !contains([]string{"resolved", "dismissed"}, status) || resolution == "" || len(resolution) > 1000 {
		return ReviewReport{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ReviewReport{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE review_reports SET status=$2,resolution=$3,resolved_by=$4,resolved_at=NOW() WHERE id=$1 AND status='open'`, id, status, resolution, actor.Label())
	if err != nil {
		return ReviewReport{}, err
	}
	if tag.RowsAffected() != 1 {
		return ReviewReport{}, identity.ErrConflict
	}
	if err = audit(ctx, tx, actor, "review_report.resolve", "review_report", id.String(), map[string]any{"status": status, "resolution": resolution}); err != nil {
		return ReviewReport{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ReviewReport{}, err
	}
	return s.GetReviewReport(ctx, id)
}

func (s *Service) ListFavorites(ctx context.Context, accountID *uuid.UUID, limit int) ([]EmbyFavorite, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT f.id,f.account_id,f.instance_id,i.name,f.binding_id,f.media_id,f.remote_item_id,f.title,COALESCE(f.media_type,''),COALESCE(f.image_tag,''),f.remote_snapshot,f.desired_favorite,f.remote_favorite,f.sync_status,COALESCE(f.last_error,''),f.last_synced_at,f.created_at,f.updated_at FROM emby_favorites f JOIN emby_instances i ON i.id=f.instance_id WHERE ($1::uuid IS NULL OR f.account_id=$1) ORDER BY f.desired_favorite DESC,f.updated_at DESC LIMIT $2`, uuidQueryValue(accountID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EmbyFavorite
	for rows.Next() {
		var item EmbyFavorite
		var raw []byte
		if err = rows.Scan(&item.ID, &item.AccountID, &item.InstanceID, &item.InstanceName, &item.BindingID, &item.MediaID, &item.RemoteItemID, &item.Title, &item.MediaType, &item.ImageTag, &raw, &item.DesiredFavorite, &item.RemoteFavorite, &item.SyncStatus, &item.LastError, &item.LastSyncedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.RemoteSnapshot = decodeJSON(raw)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Service) SetFavorite(ctx context.Context, accountID, bindingID uuid.UUID, remoteItemID, title, mediaType string, mediaID *uuid.UUID, favorite bool, actor identity.Actor) (EmbyFavorite, error) {
	remoteItemID, title, mediaType = strings.TrimSpace(remoteItemID), strings.TrimSpace(title), strings.TrimSpace(mediaType)
	if remoteItemID == "" || len(remoteItemID) > 128 || title == "" || len(title) > 500 {
		return EmbyFavorite{}, identity.ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EmbyFavorite{}, err
	}
	defer tx.Rollback(ctx)
	var instanceID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT instance_id FROM emby_account_bindings WHERE id=$1 AND account_id=$2 AND status<>'deleted'`, bindingID, accountID).Scan(&instanceID); err != nil {
		return EmbyFavorite{}, notFound(err)
	}
	id := uuid.New()
	err = tx.QueryRow(ctx, `INSERT INTO emby_favorites(id,account_id,instance_id,binding_id,media_id,remote_item_id,title,media_type,desired_favorite,remote_favorite,sync_status) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,NOT $9,'pending') ON CONFLICT(binding_id,remote_item_id) DO UPDATE SET media_id=COALESCE(EXCLUDED.media_id,emby_favorites.media_id),title=EXCLUDED.title,media_type=EXCLUDED.media_type,desired_favorite=EXCLUDED.desired_favorite,sync_status='pending',last_error=NULL,updated_at=NOW() RETURNING id`, id, accountID, instanceID, bindingID, uuidQueryValue(mediaID), remoteItemID, title, mediaType, favorite).Scan(&id)
	if err != nil {
		return EmbyFavorite{}, err
	}
	key := "favorite:" + bindingID.String() + ":" + remoteItemID + ":" + fmt.Sprint(favorite) + ":" + fmt.Sprint(time.Now().UnixNano())
	_, err = tx.Exec(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,created_by) VALUES($1,'emby.favorite',$2,$3,$4)`, uuid.New(), key, jsonBytes(map[string]any{"instance_id": instanceID.String(), "favorite_id": id.String()}), actor.Label())
	if err != nil {
		return EmbyFavorite{}, err
	}
	if err = audit(ctx, tx, actor, "emby.favorite.request", "emby_favorite", id.String(), map[string]any{"favorite": favorite, "remote_item_id": remoteItemID, "binding_id": bindingID}); err != nil {
		return EmbyFavorite{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return EmbyFavorite{}, err
	}
	return s.GetFavorite(ctx, id, accountID)
}

func (s *Service) GetFavorite(ctx context.Context, id, accountID uuid.UUID) (EmbyFavorite, error) {
	items, err := s.ListFavorites(ctx, &accountID, 500)
	if err != nil {
		return EmbyFavorite{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return EmbyFavorite{}, identity.ErrNotFound
}

func (s *Service) EnqueueFavoriteSync(ctx context.Context, accountID, bindingID uuid.UUID, actor identity.Actor) (Task, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	var instanceID uuid.UUID
	if err = tx.QueryRow(ctx, `SELECT instance_id FROM emby_account_bindings WHERE id=$1 AND account_id=$2 AND status<>'deleted'`, bindingID, accountID).Scan(&instanceID); err != nil {
		return Task{}, notFound(err)
	}
	taskID := uuid.New()
	key := "favorite-sync:" + bindingID.String() + ":" + fmt.Sprint(time.Now().Unix()/30)
	err = tx.QueryRow(ctx, `INSERT INTO platform_tasks(id,task_type,idempotency_key,payload,created_by) VALUES($1,'emby.favorite_sync',$2,$3,$4) ON CONFLICT(idempotency_key) DO UPDATE SET id=platform_tasks.id RETURNING id`, taskID, key, jsonBytes(map[string]any{"instance_id": instanceID.String(), "binding_id": bindingID.String()}), actor.Label()).Scan(&taskID)
	if err != nil {
		return Task{}, err
	}
	if err = audit(ctx, tx, actor, "emby.favorite.sync", "emby_binding", bindingID.String(), map[string]any{"task_id": taskID}); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return s.GetTask(ctx, taskID)
}

func favoriteImageTag(item map[string]any) string {
	tags, ok := item["ImageTags"].(map[string]any)
	if !ok {
		return ""
	}
	return fmt.Sprint(tags["Primary"])
}
