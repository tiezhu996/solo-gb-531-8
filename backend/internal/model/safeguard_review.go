package model

import "time"

// SafeguardReview 同时承载“历次校验记录”和“复评任务”两种角色：
// status=open 时是一项待办复评；status=completed 后冻结为不可覆盖的历史证据。
type SafeguardReview struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	SafeguardID    uint       `gorm:"not null;index:idx_review_safeguard_seq,unique,priority:1;index:idx_safeguard_reviews_safeguard;uniqueIndex:ux_review_open_per_safeguard,priority:1,where:status = 'open'" json:"safeguard_id"`
	Safeguard      Safeguard  `gorm:"foreignKey:SafeguardID" json:"safeguard,omitempty"`
	Sequence       int        `gorm:"not null;index:idx_review_safeguard_seq,unique,priority:2" json:"sequence"`
	Status         string     `gorm:"size:16;not null;index;uniqueIndex:ux_review_open_per_safeguard,priority:2,where:status = 'open'" json:"status"`
	Origin         string     `gorm:"size:16;not null" json:"origin"`
	Reason         string     `gorm:"type:text;not null" json:"reason"`
	DueAt          *time.Time `gorm:"type:datetime;index" json:"due_at,omitempty"`
	ParentReviewID *uint      `json:"parent_review_id,omitempty"`
	OpenedAt       time.Time  `gorm:"type:datetime;not null" json:"opened_at"`
	OpenedBy       uint       `gorm:"not null" json:"opened_by"`
	OpenedByName   string     `gorm:"size:80;not null" json:"opened_by_name"`
	CompletedAt    *time.Time `gorm:"type:datetime" json:"completed_at,omitempty"`
	CheckedAt      *time.Time `gorm:"type:datetime" json:"checked_at,omitempty"`
	Conclusion     string     `gorm:"size:16" json:"conclusion,omitempty"`
	Evidence       string     `gorm:"type:text" json:"evidence,omitempty"`
	Responsible    string     `gorm:"size:120" json:"responsible,omitempty"`
	ResponsibleID  *uint      `json:"responsible_id,omitempty"`
	NextDueAt      *time.Time `gorm:"type:datetime" json:"next_due_at,omitempty"`
	ClosedBy       *uint      `json:"closed_by,omitempty"`
	ClosedByName   string     `gorm:"size:80" json:"closed_by_name,omitempty"`
	CreatedAt      time.Time  `gorm:"type:datetime;not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"type:datetime;not null" json:"updated_at"`
}

func (SafeguardReview) TableName() string { return "safeguard_reviews" }

// IsOverdue 未办结且已超过下次到期时间。
func (r SafeguardReview) IsOverdue(at time.Time) bool {
	return r.Status == "open" && r.DueAt != nil && at.After(*r.DueAt)
}
