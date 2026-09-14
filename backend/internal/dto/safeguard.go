package dto

import (
	"hazop-safeguard-coverage/backend/internal/model"
	"strings"
	"time"
)

type CreateSafeguardRequest struct {
	Name             string     `json:"name" binding:"required,min=2,max=180"`
	SafeguardType    string     `json:"safeguard_type" binding:"required,oneof=alarm interlock relief procedural containment detection"`
	TargetScenarioID uint       `json:"target_scenario_id" binding:"required"`
	IndependenceKey  string     `json:"independence_key" binding:"required,min=2,max=100"`
	Effectiveness    float64    `json:"effectiveness" binding:"required,gt=0,lte=1"`
	TestIntervalDays int        `json:"test_interval_days" binding:"required,min=1,max=3650"`
	LastVerifiedAt   *time.Time `json:"last_verified_at"`
	EvidenceNote     string     `json:"evidence_note" binding:"required,min=3,max=4000"`
}

func (r *CreateSafeguardRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.SafeguardType = strings.ToLower(strings.TrimSpace(r.SafeguardType))
	r.IndependenceKey = strings.ToUpper(strings.TrimSpace(r.IndependenceKey))
	r.EvidenceNote = strings.TrimSpace(r.EvidenceNote)
}

type UpdateSafeguardRequest struct {
	Name             *string  `json:"name" binding:"omitempty,min=2,max=180"`
	SafeguardType    *string  `json:"safeguard_type" binding:"omitempty,oneof=alarm interlock relief procedural containment detection"`
	IndependenceKey  *string  `json:"independence_key" binding:"omitempty,min=2,max=100"`
	Effectiveness    *float64 `json:"effectiveness" binding:"omitempty,gt=0,lte=1"`
	TestIntervalDays *int     `json:"test_interval_days" binding:"omitempty,min=1,max=3650"`
	EvidenceNote     *string  `json:"evidence_note" binding:"omitempty,min=3,max=4000"`
}

func (r *UpdateSafeguardRequest) Normalize() {
	r.Name = trimPointer(r.Name)
	if r.SafeguardType != nil {
		value := strings.ToLower(strings.TrimSpace(*r.SafeguardType))
		r.SafeguardType = &value
	}
	if r.IndependenceKey != nil {
		value := strings.ToUpper(strings.TrimSpace(*r.IndependenceKey))
		r.IndependenceKey = &value
	}
	r.EvidenceNote = trimPointer(r.EvidenceNote)
}

type VerifySafeguardRequest struct {
	VerifiedAt   time.Time `json:"verified_at" binding:"required"`
	EvidenceNote string    `json:"evidence_note" binding:"required,min=3,max=4000"`
}
type SafeguardActionRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=1000"`
}
type SafeguardQuery struct {
	ScenarioID     uint
	Type           string
	LifecycleState string
	ExpiredOnly    bool
	// ReviewStatus: open=有未完成复评; overdue=有逾期未完成复评; none=无未完成复评。
	ReviewStatus string
	// LatestConclusion: pass/fail，仅返回最近一次已办结校验为该结论的保护层。
	LatestConclusion string
	Search           string
	Page             int
	PageSize         int
}
type SafeguardResponse struct {
	ID                  uint       `json:"id"`
	Name                string     `json:"name"`
	SafeguardType       string     `json:"safeguard_type"`
	TargetScenarioID    uint       `json:"target_scenario_id"`
	IndependenceKey     string     `json:"independence_key"`
	Effectiveness       float64    `json:"effectiveness"`
	TestIntervalDays    int        `json:"test_interval_days"`
	LastVerifiedAt      *time.Time `json:"last_verified_at,omitempty"`
	VerificationExpires *time.Time `json:"verification_expires_at,omitempty"`
	VerificationExpired bool       `json:"verification_expired"`
	LifecycleState      string     `json:"lifecycle_state"`
	EvidenceNote        string     `json:"evidence_note"`
	LastVerificationBy  *uint      `json:"last_verification_by,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	// 复评闭环投影：台账与详情共用同一组字段。
	OpenReviewID       uint       `json:"open_review_id,omitempty"`
	OpenReviewDueAt    *time.Time `json:"open_review_due_at,omitempty"`
	OpenReviewOverdue  bool       `json:"open_review_overdue"`
	ReviewPending      bool       `json:"review_pending"`
	LatestReviewID     uint       `json:"latest_review_id,omitempty"`
	LatestConclusion   string     `json:"latest_conclusion,omitempty"`
	LatestCheckedAt    *time.Time `json:"latest_checked_at,omitempty"`
	LatestNextDueAt    *time.Time `json:"latest_next_due_at,omitempty"`
	LatestResponsible  string     `json:"latest_responsible,omitempty"`
	LatestEvidence     string     `json:"latest_evidence,omitempty"`
	CompletedReviewCnt int        `json:"completed_review_count"`
}
type SafeguardListResponse struct {
	Items []SafeguardResponse `json:"items"`
	Total int64               `json:"total"`
	Page  int                 `json:"page"`
	Size  int                 `json:"page_size"`
}

func NewSafeguardResponse(s model.Safeguard, now time.Time) SafeguardResponse {
	expires := s.VerificationExpiresAt()
	expired := expires == nil || now.After(*expires)
	return SafeguardResponse{
		ID: s.ID, Name: s.Name, SafeguardType: s.SafeguardType, TargetScenarioID: s.TargetScenarioID,
		IndependenceKey: s.IndependenceKey, Effectiveness: s.Effectiveness,
		TestIntervalDays: s.TestIntervalDays, LastVerifiedAt: s.LastVerifiedAt,
		VerificationExpires: expires, VerificationExpired: expired,
		LifecycleState: s.LifecycleState, EvidenceNote: s.EvidenceNote,
		LastVerificationBy: s.LastVerificationBy, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

// SafeguardReviewBundle 按保护层聚合复评投影，供台账与详情复用。
type SafeguardReviewBundle struct {
	Open      *model.SafeguardReview
	Latest    *model.SafeguardReview
	Completed int
}

// AttachReviewProjection 将待复评/逾期状态和最近一次结论附加到台账响应。
func (r *SafeguardResponse) AttachReviewProjection(bundle SafeguardReviewBundle, now time.Time) {
	if bundle.Open != nil {
		r.OpenReviewID = bundle.Open.ID
		r.OpenReviewDueAt = bundle.Open.DueAt
		r.OpenReviewOverdue = bundle.Open.IsOverdue(now)
		r.ReviewPending = true
	}
	if bundle.Latest != nil {
		r.LatestReviewID = bundle.Latest.ID
		r.LatestConclusion = bundle.Latest.Conclusion
		r.LatestCheckedAt = bundle.Latest.CheckedAt
		r.LatestNextDueAt = bundle.Latest.NextDueAt
		r.LatestResponsible = bundle.Latest.Responsible
		r.LatestEvidence = bundle.Latest.Evidence
	}
	r.CompletedReviewCnt = bundle.Completed
}
