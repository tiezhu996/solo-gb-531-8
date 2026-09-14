package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"

	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"hazop-safeguard-coverage/backend/internal/repository"
	"hazop-safeguard-coverage/backend/internal/util"
)

type SafeguardReviewService interface {
	Open(context.Context, uint, dto.OpenSafeguardReviewRequest, util.Actor) (dto.SafeguardReviewResponse, error)
	Get(context.Context, uint, uint) (dto.SafeguardReviewResponse, error)
	ListBySafeguard(context.Context, uint) ([]dto.SafeguardReviewResponse, error)
	ListOpen(context.Context, dto.SafeguardReviewQuery) (dto.SafeguardReviewListResponse, error)
	Complete(context.Context, uint, uint, dto.CompleteSafeguardReviewRequest, util.Actor) (dto.SafeguardReviewResponse, error)
}

type safeguardReviewService struct {
	reviews    repository.SafeguardReviewRepository
	safeguards repository.SafeguardRepository
	audits     repository.AuditRepository
	now        func() time.Time
}

func NewSafeguardReviewService(
	reviews repository.SafeguardReviewRepository,
	safeguards repository.SafeguardRepository,
	audits repository.AuditRepository,
) SafeguardReviewService {
	return &safeguardReviewService{
		reviews: reviews, safeguards: safeguards, audits: audits,
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *safeguardReviewService) Open(
	ctx context.Context,
	safeguardID uint,
	request dto.OpenSafeguardReviewRequest,
	actor util.Actor,
) (dto.SafeguardReviewResponse, error) {
	request.Normalize()
	safeguard, err := s.safeguards.GetByID(ctx, safeguardID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardReviewResponse{}, util.NotFound("safeguard")
		}
		return dto.SafeguardReviewResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	now := s.now()
	dueAt := request.DueAt.UTC()
	var opened model.SafeguardReview
	err = s.inTransaction(ctx, func(tx *gorm.DB) error {
		hasOpen, checkErr := s.reviews.HasOpenInTx(ctx, tx, safeguardID)
		if checkErr != nil {
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to check existing review", checkErr)
		}
		if hasOpen {
			return util.NewError(http.StatusConflict, util.CodeConflict, "safeguard already has an unfinished review")
		}
		sequence, seqErr := s.reviews.NextSequenceInTx(ctx, tx, safeguardID)
		if seqErr != nil {
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to allocate review sequence", seqErr)
		}
		origin := "manual"
		expires := safeguard.VerificationExpiresAt()
		if safeguard.LifecycleState == "invalid" {
			origin = "failure"
		} else if expires == nil || now.After(*expires) {
			origin = "expiry"
		}
		opened = model.SafeguardReview{
			SafeguardID: safeguardID, Sequence: sequence, Status: "open", Origin: origin,
			Reason: request.Reason, DueAt: &dueAt, OpenedAt: now,
			OpenedBy: actor.UserID, OpenedByName: actor.Username,
			CreatedAt: now, UpdatedAt: now,
		}
		if createErr := s.reviews.CreateInTx(ctx, tx, &opened); createErr != nil {
			if uniqueViolation(createErr) {
				return util.NewError(http.StatusConflict, util.CodeConflict, "safeguard already has an unfinished review")
			}
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to open review", createErr)
		}
		return s.recordAuditInTx(ctx, tx, actor, opened.ID, "open", nil, opened, request.Reason, now)
	})
	if err != nil {
		return dto.SafeguardReviewResponse{}, err
	}
	return dto.NewSafeguardReviewResponse(opened, now), nil
}

func (s *safeguardReviewService) Get(ctx context.Context, safeguardID, id uint) (dto.SafeguardReviewResponse, error) {
	review, err := s.reviews.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardReviewResponse{}, util.NotFound("safeguard review")
		}
		return dto.SafeguardReviewResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load review", err)
	}
	if review.SafeguardID != safeguardID {
		return dto.SafeguardReviewResponse{}, util.NotFound("safeguard review")
	}
	return dto.NewSafeguardReviewResponse(review, s.now()), nil
}

func (s *safeguardReviewService) ListBySafeguard(ctx context.Context, safeguardID uint) ([]dto.SafeguardReviewResponse, error) {
	if _, err := s.safeguards.GetByID(ctx, safeguardID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, util.NotFound("safeguard")
		}
		return nil, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	reviews, err := s.reviews.ListBySafeguard(ctx, safeguardID)
	if err != nil {
		return nil, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to list reviews", err)
	}
	now := s.now()
	items := make([]dto.SafeguardReviewResponse, 0, len(reviews))
	for _, review := range reviews {
		items = append(items, dto.NewSafeguardReviewResponse(review, now))
	}
	return items, nil
}

func (s *safeguardReviewService) ListOpen(ctx context.Context, query dto.SafeguardReviewQuery) (dto.SafeguardReviewListResponse, error) {
	reviews, total, err := s.reviews.ListOpen(ctx, query, s.now())
	if err != nil {
		return dto.SafeguardReviewListResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to list open reviews", err)
	}
	return dto.NewSafeguardReviewListResponse(reviews, total, query.Page, query.PageSize, s.now()), nil
}

func (s *safeguardReviewService) Complete(
	ctx context.Context,
	safeguardID, id uint,
	request dto.CompleteSafeguardReviewRequest,
	actor util.Actor,
) (dto.SafeguardReviewResponse, error) {
	request.Normalize()
	now := s.now()
	if request.CheckedAt.After(now.Add(5 * time.Minute)) {
		return dto.SafeguardReviewResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "checked_at cannot be in the future")
	}
	if !request.NextDueAt.After(request.CheckedAt) {
		return dto.SafeguardReviewResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "next_due_at must be after checked_at")
	}
	review, err := s.reviews.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardReviewResponse{}, util.NotFound("safeguard review")
		}
		return dto.SafeguardReviewResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load review", err)
	}
	if review.Status != "open" {
		return dto.SafeguardReviewResponse{}, util.NewError(http.StatusConflict, util.CodeStateTransition, "review is already completed; historical records cannot be overwritten")
	}
	if review.SafeguardID != safeguardID {
		return dto.SafeguardReviewResponse{}, util.NotFound("safeguard review")
	}
	// 下次到期时间必须落在台账允许的试验间隔内，防止用超长有效期让保护层“永久合格”。
	intervalDays := int(request.NextDueAt.Sub(request.CheckedAt).Hours() / 24)
	if intervalDays < 1 {
		intervalDays = 1
	}
	if intervalDays > review.Safeguard.TestIntervalDays && review.Safeguard.TestIntervalDays > 0 {
		return dto.SafeguardReviewResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation,
			fmt.Sprintf("next_due_at exceeds test interval of %d days", review.Safeguard.TestIntervalDays))
	}
	passLifecycle := "active"
	if request.NextDueAt.Before(now) {
		// 登记的是一次已经再次到期的历史校验，不能恢复为可用。
		passLifecycle = "expired"
	}
	summary := fmt.Sprintf("复评办结：%s；责任人 %s", map[bool]string{true: "合格", false: "不合格"}[request.Conclusion == "pass"], request.Responsible)
	before := review
	var completed model.SafeguardReview
	err = s.inTransaction(ctx, func(tx *gorm.DB) error {
		var followup *model.SafeguardReview
		var closureErr error
		_, completed, followup, _, closureErr = s.reviews.CompleteInTx(ctx, tx, repository.ReviewClosure{
			ReviewID: id, CheckedAt: request.CheckedAt, NextDueAt: request.NextDueAt,
			Conclusion: request.Conclusion, Evidence: request.Evidence,
			Responsible: request.Responsible, ActorID: actor.UserID, ActorName: actor.Username,
			PassLifecycle:    passLifecycle,
			FollowupReason:   fmt.Sprintf("复评 #%d 不合格：%s", review.Sequence, util.CompactText(request.Evidence, 400)),
			FollowupOpenedBy: actor.UserID, FollowupOpenedName: actor.Username,
			Now: now,
		})
		if closureErr != nil {
			if uniqueViolation(closureErr) {
				return util.NewError(http.StatusConflict, util.CodeConflict, "safeguard already has an unfinished review")
			}
			return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to complete review", closureErr)
		}
		if auditErr := s.recordAuditInTx(ctx, tx, actor, id, "complete", before, completed, summary, now); auditErr != nil {
			return auditErr
		}
		if followup != nil {
			if auditErr := s.recordAuditInTx(ctx, tx, actor, followup.ID, "auto_open", completed, followup,
				"不合格复评自动生成跟进复评任务", now); auditErr != nil {
				return auditErr
			}
		}
		return nil
	})
	if err != nil {
		return dto.SafeguardReviewResponse{}, err
	}
	return dto.NewSafeguardReviewResponse(completed, now), nil
}

// inTransaction 在同一事务中运行服务级校验与写入，确保“同一保护层仅一项未完成复评”不被并发穿透。
func (s *safeguardReviewService) inTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.reviews.InTx(ctx, fn)
}

func (s *safeguardReviewService) recordAuditInTx(
	ctx context.Context,
	tx *gorm.DB,
	actor util.Actor,
	entityID uint,
	action string,
	before any,
	after any,
	summary string,
	at time.Time,
) error {
	beforeJSON, err := snapshotJSON(before)
	if err != nil {
		return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to serialize audit snapshot", err)
	}
	afterJSON, err := snapshotJSON(after)
	if err != nil {
		return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to serialize audit snapshot", err)
	}
	log := model.AuditLog{
		RequestID: actor.RequestID, ActorID: actor.UserID, ActorName: actor.Username,
		ActorRole: actor.Role, EntityType: "safeguard_review", EntityID: entityID, Action: action,
		BeforeSnapshot: beforeJSON, AfterSnapshot: afterJSON,
		ResultSummary: util.CompactText(summary, 1000), CreatedAt: at,
	}
	if err := tx.WithContext(ctx).Create(&log).Error; err != nil {
		return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to record write audit", err)
	}
	return nil
}
