package dto

import (
	"strings"
	"time"

	"hazop-safeguard-coverage/backend/internal/model"
)

// OpenSafeguardReviewRequest 为某个保护层新增一项复评任务。
type OpenSafeguardReviewRequest struct {
	Reason string     `json:"reason" binding:"required,min=3,max=1000"`
	DueAt  *time.Time `json:"due_at" binding:"required"`
}

func (r *OpenSafeguardReviewRequest) Normalize() {
	r.Reason = strings.TrimSpace(r.Reason)
}

// CompleteSafeguardReviewRequest 办结复评并登记本次校验结论。
type CompleteSafeguardReviewRequest struct {
	CheckedAt   time.Time `json:"checked_at" binding:"required"`
	Conclusion  string    `json:"conclusion" binding:"required,oneof=pass fail"`
	Evidence    string    `json:"evidence" binding:"required,min=3,max=4000"`
	Responsible string    `json:"responsible" binding:"required,min=2,max=120"`
	// NextDueAt 为下次到期时间；合格时同时更新台账有效期，不合格时写入跟进复评任务。
	NextDueAt time.Time `json:"next_due_at" binding:"required"`
}

func (r *CompleteSafeguardReviewRequest) Normalize() {
	r.Evidence = strings.TrimSpace(r.Evidence)
	r.Responsible = strings.TrimSpace(r.Responsible)
	r.Conclusion = strings.ToLower(strings.TrimSpace(r.Conclusion))
}

type SafeguardReviewQuery struct {
	SafeguardID uint
	Status      string
	OverdueOnly bool
	Page        int
	PageSize    int
}

type SafeguardReviewResponse struct {
	ID             uint                    `json:"id"`
	SafeguardID    uint                    `json:"safeguard_id"`
	Sequence       int                     `json:"sequence"`
	Status         string                  `json:"status"`
	Origin         string                  `json:"origin"`
	Reason         string                  `json:"reason"`
	DueAt          *time.Time              `json:"due_at,omitempty"`
	ParentReviewID *uint                   `json:"parent_review_id,omitempty"`
	OpenedAt       time.Time               `json:"opened_at"`
	OpenedBy       uint                    `json:"opened_by"`
	OpenedByName   string                  `json:"opened_by_name"`
	CompletedAt    *time.Time              `json:"completed_at,omitempty"`
	CheckedAt      *time.Time              `json:"checked_at,omitempty"`
	Conclusion     string                  `json:"conclusion,omitempty"`
	Evidence       string                  `json:"evidence,omitempty"`
	Responsible    string                  `json:"responsible,omitempty"`
	ResponsibleID  *uint                   `json:"responsible_id,omitempty"`
	NextDueAt      *time.Time              `json:"next_due_at,omitempty"`
	ClosedBy       *uint                   `json:"closed_by,omitempty"`
	ClosedByName   string                  `json:"closed_by_name,omitempty"`
	Overdue        bool                    `json:"overdue"`
	CreatedAt      time.Time               `json:"created_at"`
	Safeguard      *SafeguardReviewSubject `json:"safeguard,omitempty"`
}

// SafeguardReviewSubject 是复评记录中内嵌的保护层摘要，供待复评总览直接展示。
type SafeguardReviewSubject struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	SafeguardType    string `json:"safeguard_type"`
	LifecycleState   string `json:"lifecycle_state"`
	TestIntervalDays int    `json:"test_interval_days"`
}

type SafeguardReviewListResponse struct {
	Items []SafeguardReviewResponse `json:"items"`
	Total int64                     `json:"total"`
	Page  int                       `json:"page"`
	Size  int                       `json:"page_size"`
}

func NewSafeguardReviewResponse(r model.SafeguardReview, now time.Time) SafeguardReviewResponse {
	response := SafeguardReviewResponse{
		ID: r.ID, SafeguardID: r.SafeguardID, Sequence: r.Sequence,
		Status: r.Status, Origin: r.Origin, Reason: r.Reason, DueAt: r.DueAt,
		ParentReviewID: r.ParentReviewID, OpenedAt: r.OpenedAt,
		OpenedBy: r.OpenedBy, OpenedByName: r.OpenedByName,
		CompletedAt: r.CompletedAt, CheckedAt: r.CheckedAt, Conclusion: r.Conclusion,
		Evidence: r.Evidence, Responsible: r.Responsible, ResponsibleID: r.ResponsibleID,
		NextDueAt: r.NextDueAt, ClosedBy: r.ClosedBy, ClosedByName: r.ClosedByName,
		Overdue: r.IsOverdue(now), CreatedAt: r.CreatedAt,
	}
	if r.Safeguard.ID != 0 {
		response.Safeguard = &SafeguardReviewSubject{
			ID: r.Safeguard.ID, Name: r.Safeguard.Name, SafeguardType: r.Safeguard.SafeguardType,
			LifecycleState: r.Safeguard.LifecycleState, TestIntervalDays: r.Safeguard.TestIntervalDays,
		}
	}
	return response
}

func NewSafeguardReviewListResponse(reviews []model.SafeguardReview, total int64, page, size int, now time.Time) SafeguardReviewListResponse {
	result := SafeguardReviewListResponse{
		Items: make([]SafeguardReviewResponse, 0, len(reviews)),
		Total: total, Page: page, Size: size,
	}
	for _, review := range reviews {
		result.Items = append(result.Items, NewSafeguardReviewResponse(review, now))
	}
	return result
}
