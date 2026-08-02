package platform

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type UserRealtimeSnapshot struct {
	GeneratedAt   time.Time      `json:"generated_at"`
	Provisioning  []Task         `json:"provisioning"`
	Requests      []MediaRequest `json:"media_requests"`
	Tickets       []Ticket       `json:"tickets"`
	Notifications []Notification `json:"notifications"`
}

type AdminRealtimeSnapshot struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Tasks       []Task                `json:"tasks"`
	Batches     []BatchOperation      `json:"batch_operations"`
	Automations []AutomationExecution `json:"automation_executions"`
	Risks       []RiskEvent           `json:"risk_events"`
}

func (s *Service) UserRealtime(ctx context.Context, accountID uuid.UUID) (UserRealtimeSnapshot, error) {
	result := UserRealtimeSnapshot{GeneratedAt: time.Now().UTC()}
	rows, err := s.db.Query(ctx, `SELECT task_id FROM emby_provision_requests WHERE account_id=$1 ORDER BY created_at DESC LIMIT 25`, accountID)
	if err != nil {
		return result, err
	}
	var taskIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		taskIDs = append(taskIDs, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	for _, id := range taskIDs {
		item, itemErr := s.GetTask(ctx, id)
		if itemErr != nil {
			return result, itemErr
		}
		result.Provisioning = append(result.Provisioning, item)
	}
	if result.Requests, err = s.ListMediaRequests(ctx, &accountID, "", 25); err != nil {
		return result, err
	}
	if result.Tickets, err = s.ListTickets(ctx, &accountID, "", 25); err != nil {
		return result, err
	}
	if result.Notifications, err = s.ListNotifications(ctx, accountID, "unread", 25); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) AdminRealtime(ctx context.Context) (AdminRealtimeSnapshot, error) {
	result := AdminRealtimeSnapshot{GeneratedAt: time.Now().UTC()}
	var err error
	if result.Tasks, err = s.ListTasks(ctx, nil, "", 40); err != nil {
		return result, err
	}
	if result.Batches, err = s.ListBatches(ctx, "", 25); err != nil {
		return result, err
	}
	if result.Automations, err = s.ListAutomationExecutions(ctx, 25); err != nil {
		return result, err
	}
	if result.Risks, err = s.ListRiskEvents(ctx, nil, nil, "", "", 25); err != nil {
		return result, err
	}
	return result, nil
}
