package repository

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
)

type SafeguardReviewRepository interface {
	Create(context.Context, *model.SafeguardReview) error
	GetByID(context.Context, uint) (model.SafeguardReview, error)
	ListBySafeguard(context.Context, uint) ([]model.SafeguardReview, error)
	ListOpen(context.Context, dto.SafeguardReviewQuery, time.Time) ([]model.SafeguardReview, int64, error)
	InTx(context.Context, func(*gorm.DB) error) error
	HasOpenInTx(context.Context, *gorm.DB, uint) (bool, error)
	NextSequenceInTx(context.Context, *gorm.DB, uint) (int, error)
	CreateInTx(context.Context, *gorm.DB, *model.SafeguardReview) error
	FindOpenBySafeguards(context.Context, []uint) (map[uint]model.SafeguardReview, error)
	LatestCompletedBySafeguards(context.Context, []uint) (map[uint]model.SafeguardReview, error)
	CompletedCountBySafeguards(context.Context, []uint) (map[uint]int, error)
	VerifyWithSafeguard(context.Context, VerifyClosure) (model.Safeguard, model.SafeguardReview, bool, error)
	CompleteInTx(context.Context, *gorm.DB, ReviewClosure) (model.Safeguard, model.SafeguardReview, *model.SafeguardReview, bool, error)
}

// VerifyClosure 是“旧验证接口 + 同步写入一条合格校验记录”的事务参数。
type VerifyClosure struct {
	SafeguardID uint
	FromStates  []string
	VerifiedAt  time.Time
	NextDueAt   time.Time
	Evidence    string
	ActorID     uint
	ActorName   string
	Now         time.Time
}

// ReviewClosure 是办结复评的事务参数。
type ReviewClosure struct {
	ReviewID    uint
	CheckedAt   time.Time
	NextDueAt   time.Time
	Conclusion  string
	Evidence    string
	Responsible string
	ActorID     uint
	ActorName   string
	// Pass 时台账转为该生命周期（active）；Fail 时固定 invalid。
	PassLifecycle string
	// Fail 时生成的跟进复评任务。
	FollowupReason     string
	FollowupOpenedBy   uint
	FollowupOpenedName string
	Now                time.Time
}

type safeguardReviewRepository struct{ db *gorm.DB }

func NewSafeguardReviewRepository(db *gorm.DB) SafeguardReviewRepository {
	return &safeguardReviewRepository{db: db}
}

func (r *safeguardReviewRepository) Create(ctx context.Context, review *model.SafeguardReview) error {
	if err := r.db.WithContext(ctx).Create(review).Error; err != nil {
		return fmt.Errorf("create safeguard review: %w", err)
	}
	return nil
}

func (r *safeguardReviewRepository) GetByID(ctx context.Context, id uint) (model.SafeguardReview, error) {
	var review model.SafeguardReview
	if err := r.db.WithContext(ctx).Preload("Safeguard").First(&review, id).Error; err != nil {
		return model.SafeguardReview{}, fmt.Errorf("find safeguard review %d: %w", id, err)
	}
	return review, nil
}

func (r *safeguardReviewRepository) ListBySafeguard(ctx context.Context, safeguardID uint) ([]model.SafeguardReview, error) {
	var reviews []model.SafeguardReview
	if err := r.db.WithContext(ctx).Where("safeguard_id = ?", safeguardID).
		Order("sequence DESC, id DESC").Find(&reviews).Error; err != nil {
		return nil, fmt.Errorf("list reviews for safeguard %d: %w", safeguardID, err)
	}
	return reviews, nil
}

func (r *safeguardReviewRepository) ListOpen(
	ctx context.Context,
	query dto.SafeguardReviewQuery,
	now time.Time,
) ([]model.SafeguardReview, int64, error) {
	base := r.db.WithContext(ctx).Model(&model.SafeguardReview{}).Where("status = ?", "open")
	if query.SafeguardID != 0 {
		base = base.Where("safeguard_id = ?", query.SafeguardID)
	}
	if query.OverdueOnly {
		base = base.Where("due_at IS NOT NULL AND due_at < ?", now)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count open safeguard reviews: %w", err)
	}
	var reviews []model.SafeguardReview
	offset := (query.Page - 1) * query.PageSize
	err := base.Preload("Safeguard").
		Order("CASE WHEN due_at IS NULL THEN 1 ELSE 0 END ASC, due_at ASC, id ASC").
		Limit(query.PageSize).Offset(offset).Find(&reviews).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list open safeguard reviews: %w", err)
	}
	return reviews, total, nil
}

func (r *safeguardReviewRepository) InTx(ctx context.Context, fn func(*gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}

func (r *safeguardReviewRepository) HasOpenInTx(ctx context.Context, tx *gorm.DB, safeguardID uint) (bool, error) {
	var count int64
	if err := tx.WithContext(ctx).Model(&model.SafeguardReview{}).
		Where("safeguard_id = ? AND status = ?", safeguardID, "open").
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("check open review for safeguard %d: %w", safeguardID, err)
	}
	return count > 0, nil
}

func (r *safeguardReviewRepository) NextSequenceInTx(ctx context.Context, tx *gorm.DB, safeguardID uint) (int, error) {
	var next int
	if err := tx.WithContext(ctx).Model(&model.SafeguardReview{}).
		Where("safeguard_id = ?", safeguardID).
		Select("COALESCE(MAX(sequence), 0) + 1").Scan(&next).Error; err != nil {
		return 0, fmt.Errorf("allocate review sequence for safeguard %d: %w", safeguardID, err)
	}
	return next, nil
}

func (r *safeguardReviewRepository) CreateInTx(ctx context.Context, tx *gorm.DB, review *model.SafeguardReview) error {
	if err := tx.WithContext(ctx).Create(review).Error; err != nil {
		return fmt.Errorf("create safeguard review in tx: %w", err)
	}
	return nil
}

func (r *safeguardReviewRepository) FindOpenBySafeguards(ctx context.Context, ids []uint) (map[uint]model.SafeguardReview, error) {
	result := make(map[uint]model.SafeguardReview)
	if len(ids) == 0 {
		return result, nil
	}
	var reviews []model.SafeguardReview
	if err := r.db.WithContext(ctx).Where("status = ? AND safeguard_id IN ?", "open", ids).
		Find(&reviews).Error; err != nil {
		return nil, fmt.Errorf("find open reviews: %w", err)
	}
	for _, review := range reviews {
		result[review.SafeguardID] = review
	}
	return result, nil
}

func (r *safeguardReviewRepository) LatestCompletedBySafeguards(ctx context.Context, ids []uint) (map[uint]model.SafeguardReview, error) {
	result := make(map[uint]model.SafeguardReview)
	if len(ids) == 0 {
		return result, nil
	}
	var reviews []model.SafeguardReview
	subQuery := r.db.Model(&model.SafeguardReview{}).
		Select("MAX(id)").Where("status = ?", "completed").Group("safeguard_id")
	if err := r.db.WithContext(ctx).Where("id IN (?)", subQuery).
		Where("safeguard_id IN ?", ids).Find(&reviews).Error; err != nil {
		return nil, fmt.Errorf("find latest completed reviews: %w", err)
	}
	for _, review := range reviews {
		result[review.SafeguardID] = review
	}
	return result, nil
}

func (r *safeguardReviewRepository) CompletedCountBySafeguards(ctx context.Context, ids []uint) (map[uint]int, error) {
	result := make(map[uint]int)
	if len(ids) == 0 {
		return result, nil
	}
	type row struct {
		SafeguardID uint
		Count       int
	}
	var rows []row
	if err := r.db.WithContext(ctx).Model(&model.SafeguardReview{}).
		Where("status = ? AND safeguard_id IN ?", "completed", ids).
		Select("safeguard_id, COUNT(*) AS count").
		Group("safeguard_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count completed reviews: %w", err)
	}
	for _, item := range rows {
		result[item.SafeguardID] = item.Count
	}
	return result, nil
}

func (r *safeguardReviewRepository) VerifyWithSafeguard(
	ctx context.Context,
	input VerifyClosure,
) (model.Safeguard, model.SafeguardReview, bool, error) {
	var safeguard model.Safeguard
	var review model.SafeguardReview
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.Safeguard{}).
			Where("id = ? AND lifecycle_state IN ?", input.SafeguardID, input.FromStates).
			Updates(map[string]any{
				"lifecycle_state":      "active",
				"last_verified_at":     input.VerifiedAt.UTC(),
				"verified_until":       input.NextDueAt.UTC(),
				"last_verification_by": input.ActorID,
				"evidence_note":        input.Evidence,
				"updated_at":           input.Now,
			})
		if result.Error != nil {
			return fmt.Errorf("verify safeguard in tx: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return errLifecycleConflict
		}
		if err := tx.First(&safeguard, input.SafeguardID).Error; err != nil {
			return fmt.Errorf("reload safeguard %d: %w", input.SafeguardID, err)
		}
		sequence, err := r.NextSequenceInTx(ctx, tx, input.SafeguardID)
		if err != nil {
			return err
		}
		review = model.SafeguardReview{
			SafeguardID: input.SafeguardID, Sequence: sequence, Status: "completed",
			Origin: "manual", Reason: "即时验证登记",
			OpenedAt: input.Now, OpenedBy: input.ActorID, OpenedByName: input.ActorName,
			CompletedAt: &input.Now, CheckedAt: &input.VerifiedAt, Conclusion: "pass",
			Evidence: input.Evidence, Responsible: input.ActorName, ResponsibleID: &input.ActorID,
			NextDueAt: &input.NextDueAt, ClosedBy: &input.ActorID, ClosedByName: input.ActorName,
			CreatedAt: input.Now, UpdatedAt: input.Now,
		}
		if err := r.CreateInTx(ctx, tx, &review); err != nil {
			return err
		}
		return nil
	})
	if err == errLifecycleConflict {
		return model.Safeguard{}, model.SafeguardReview{}, false, nil
	}
	if err != nil {
		return model.Safeguard{}, model.SafeguardReview{}, false, err
	}
	return safeguard, review, true, nil
}

func (r *safeguardReviewRepository) CompleteInTx(
	ctx context.Context,
	tx *gorm.DB,
	input ReviewClosure,
) (model.Safeguard, model.SafeguardReview, *model.SafeguardReview, bool, error) {
	var safeguard model.Safeguard
	var completed model.SafeguardReview
	var followup *model.SafeguardReview
	run := func() error {
		var review model.SafeguardReview
		if err := tx.First(&review, input.ReviewID).Error; err != nil {
			return fmt.Errorf("load review %d: %w", input.ReviewID, err)
		}
		closedBy := input.ActorID
		now := input.Now.UTC()
		checkedAt := input.CheckedAt.UTC()
		nextDueAt := input.NextDueAt.UTC()
		updates := map[string]any{
			"status":         "completed",
			"completed_at":   now,
			"checked_at":     checkedAt,
			"conclusion":     input.Conclusion,
			"evidence":       input.Evidence,
			"responsible":    input.Responsible,
			"responsible_id": input.ActorID,
			"next_due_at":    nextDueAt,
			"closed_by":      closedBy,
			"closed_by_name": input.ActorName,
			"updated_at":     now,
		}
		result := tx.Model(&model.SafeguardReview{}).
			Where("id = ? AND status = ?", input.ReviewID, "open").
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("complete review %d: %w", input.ReviewID, result.Error)
		}
		if result.RowsAffected == 0 {
			return errLifecycleConflict
		}
		safeguardUpdates := map[string]any{
			"lifecycle_state": "invalid",
			"updated_at":      now,
		}
		if input.Conclusion == "pass" {
			safeguardUpdates["lifecycle_state"] = input.PassLifecycle
			safeguardUpdates["last_verified_at"] = checkedAt
			safeguardUpdates["verified_until"] = nextDueAt
			safeguardUpdates["last_verification_by"] = input.ActorID
		}
		safeguardResult := tx.Model(&model.Safeguard{}).
			Where("id = ?", review.SafeguardID).Updates(safeguardUpdates)
		if safeguardResult.Error != nil {
			return fmt.Errorf("update safeguard %d after review: %w", review.SafeguardID, safeguardResult.Error)
		}
		if safeguardResult.RowsAffected == 0 {
			return fmt.Errorf("safeguard %d disappeared during review completion", review.SafeguardID)
		}
		if input.Conclusion == "fail" {
			sequence, err := r.NextSequenceInTx(ctx, tx, review.SafeguardID)
			if err != nil {
				return err
			}
			followup = &model.SafeguardReview{
				SafeguardID: review.SafeguardID, Sequence: sequence, Status: "open",
				Origin: "failure", Reason: input.FollowupReason, DueAt: &nextDueAt,
				ParentReviewID: &review.ID, OpenedAt: now,
				OpenedBy: input.FollowupOpenedBy, OpenedByName: input.FollowupOpenedName,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := r.CreateInTx(ctx, tx, followup); err != nil {
				return err
			}
		}
		if err := tx.First(&completed, input.ReviewID).Error; err != nil {
			return fmt.Errorf("reload completed review %d: %w", input.ReviewID, err)
		}
		if err := tx.First(&safeguard, review.SafeguardID).Error; err != nil {
			return fmt.Errorf("reload safeguard %d: %w", review.SafeguardID, err)
		}
		return nil
	}
	if err := run(); err == errLifecycleConflict {
		return model.Safeguard{}, model.SafeguardReview{}, nil, false, nil
	} else if err != nil {
		return model.Safeguard{}, model.SafeguardReview{}, nil, false, err
	}
	return safeguard, completed, followup, true, nil
}

var errLifecycleConflict = fmt.Errorf("lifecycle conflict")
