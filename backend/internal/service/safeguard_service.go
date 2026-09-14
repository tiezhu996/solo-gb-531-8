package service

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"hazop-safeguard-coverage/backend/internal/repository"
	"hazop-safeguard-coverage/backend/internal/util"
	"net/http"
	"time"
)

type SafeguardService interface {
	Create(context.Context, dto.CreateSafeguardRequest, util.Actor) (dto.SafeguardResponse, error)
	Get(context.Context, uint) (dto.SafeguardResponse, error)
	List(context.Context, dto.SafeguardQuery) (dto.SafeguardListResponse, error)
	Update(context.Context, uint, dto.UpdateSafeguardRequest, util.Actor) (dto.SafeguardResponse, error)
	Verify(context.Context, uint, dto.VerifySafeguardRequest, util.Actor) (dto.SafeguardResponse, error)
	Invalidate(context.Context, uint, dto.SafeguardActionRequest, util.Actor) (dto.SafeguardResponse, error)
	Restore(context.Context, uint, dto.SafeguardActionRequest, util.Actor) (dto.SafeguardResponse, error)
}
type safeguardService struct {
	safeguards repository.SafeguardRepository
	reviews    repository.SafeguardReviewRepository
	scenarios  repository.DeviationScenarioRepository
	audits     repository.AuditRepository
	now        func() time.Time
}

func NewSafeguardService(
	safeguards repository.SafeguardRepository,
	reviews repository.SafeguardReviewRepository,
	scenarios repository.DeviationScenarioRepository,
	audits repository.AuditRepository,
) SafeguardService {
	return &safeguardService{
		safeguards: safeguards, reviews: reviews, scenarios: scenarios, audits: audits,
		now: func() time.Time { return time.Now().UTC() },
	}
}
func (s *safeguardService) Create(
	ctx context.Context,
	request dto.CreateSafeguardRequest,
	actor util.Actor,
) (dto.SafeguardResponse, error) {
	request.Normalize()
	if _, err := s.scenarios.GetByID(ctx, request.TargetScenarioID, false); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardResponse{}, util.NotFound("deviation scenario")
		}
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load target scenario", err)
	}
	now := s.now()
	if request.LastVerifiedAt != nil && request.LastVerifiedAt.After(now.Add(5*time.Minute)) {
		return dto.SafeguardResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "last_verified_at cannot be in the future")
	}
	state := "pending"
	if request.LastVerifiedAt != nil {
		expires := request.LastVerifiedAt.AddDate(0, 0, request.TestIntervalDays)
		if now.After(expires) {
			state = "expired"
		} else {
			state = "active"
		}
	}
	safeguard := model.Safeguard{
		Name: request.Name, SafeguardType: request.SafeguardType,
		TargetScenarioID: request.TargetScenarioID, IndependenceKey: request.IndependenceKey,
		Effectiveness: request.Effectiveness, TestIntervalDays: request.TestIntervalDays,
		LastVerifiedAt: request.LastVerifiedAt, LifecycleState: state,
		EvidenceNote: request.EvidenceNote, CreatedAt: now, UpdatedAt: now,
	}
	if request.LastVerifiedAt != nil {
		verifier := actor.UserID
		safeguard.LastVerificationBy = &verifier
	}
	if err := s.safeguards.Create(ctx, &safeguard); err != nil {
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to create safeguard", err)
	}
	if err := s.recordAudit(ctx, actor, safeguard.ID, "create", nil, safeguard, ""); err != nil {
		return dto.SafeguardResponse{}, err
	}
	return dto.NewSafeguardResponse(safeguard, now), nil
}
func (s *safeguardService) Get(ctx context.Context, id uint) (dto.SafeguardResponse, error) {
	safeguard, err := s.safeguards.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardResponse{}, util.NotFound("safeguard")
		}
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	response := dto.NewSafeguardResponse(safeguard, s.now())
	bundle, err := s.reviewBundle(ctx, []uint{id})
	if err != nil {
		return dto.SafeguardResponse{}, err
	}
	response.AttachReviewProjection(bundle[id], s.now())
	return response, nil
}
func (s *safeguardService) List(ctx context.Context, query dto.SafeguardQuery) (dto.SafeguardListResponse, error) {
	now := s.now()
	safeguards, total, err := s.safeguards.List(ctx, query, now)
	if err != nil {
		return dto.SafeguardListResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to list safeguards", err)
	}
	ids := make([]uint, 0, len(safeguards))
	for _, safeguard := range safeguards {
		ids = append(ids, safeguard.ID)
	}
	bundles, err := s.reviewBundle(ctx, ids)
	if err != nil {
		return dto.SafeguardListResponse{}, err
	}
	response := dto.SafeguardListResponse{
		Items: make([]dto.SafeguardResponse, 0, len(safeguards)),
		Total: total, Page: query.Page, Size: query.PageSize,
	}
	for _, safeguard := range safeguards {
		item := dto.NewSafeguardResponse(safeguard, now)
		item.AttachReviewProjection(bundles[safeguard.ID], now)
		response.Items = append(response.Items, item)
	}
	return response, nil
}

// reviewBundle 一次性聚合每个保护层的未完成复评、最近结论与历史次数，避免台账 N+1 查询。
func (s *safeguardService) reviewBundle(ctx context.Context, ids []uint) (map[uint]dto.SafeguardReviewBundle, error) {
	bundles := make(map[uint]dto.SafeguardReviewBundle, len(ids))
	for _, id := range ids {
		bundles[id] = dto.SafeguardReviewBundle{}
	}
	open, err := s.reviews.FindOpenBySafeguards(ctx, ids)
	if err != nil {
		return nil, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load open reviews", err)
	}
	latest, err := s.reviews.LatestCompletedBySafeguards(ctx, ids)
	if err != nil {
		return nil, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load latest reviews", err)
	}
	counts, err := s.reviews.CompletedCountBySafeguards(ctx, ids)
	if err != nil {
		return nil, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to count reviews", err)
	}
	for id := range bundles {
		bundle := dto.SafeguardReviewBundle{Completed: counts[id]}
		if review, ok := open[id]; ok {
			reviewCopy := review
			bundle.Open = &reviewCopy
		}
		if review, ok := latest[id]; ok {
			reviewCopy := review
			bundle.Latest = &reviewCopy
		}
		bundles[id] = bundle
	}
	return bundles, nil
}
func (s *safeguardService) Update(
	ctx context.Context,
	id uint,
	request dto.UpdateSafeguardRequest,
	actor util.Actor,
) (dto.SafeguardResponse, error) {
	request.Normalize()
	safeguard, err := s.safeguards.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardResponse{}, util.NotFound("safeguard")
		}
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	before := safeguard
	if request.Name != nil {
		safeguard.Name = *request.Name
	}
	if request.SafeguardType != nil {
		safeguard.SafeguardType = *request.SafeguardType
	}
	if request.IndependenceKey != nil {
		safeguard.IndependenceKey = *request.IndependenceKey
	}
	if request.Effectiveness != nil {
		safeguard.Effectiveness = *request.Effectiveness
	}
	if request.TestIntervalDays != nil {
		safeguard.TestIntervalDays = *request.TestIntervalDays
	}
	if request.EvidenceNote != nil {
		safeguard.EvidenceNote = *request.EvidenceNote
	}
	if expires := safeguard.VerificationExpiresAt(); expires != nil {
		if s.now().After(*expires) && safeguard.LifecycleState == "active" {
			safeguard.LifecycleState = "expired"
		}
	}
	safeguard.UpdatedAt = s.now()
	if err := s.safeguards.Update(ctx, &safeguard); err != nil {
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to update safeguard", err)
	}
	if safeguard.LifecycleState != before.LifecycleState {
		_, err = s.safeguards.SetLifecycle(ctx, id, []string{before.LifecycleState}, safeguard.LifecycleState, map[string]any{})
		if err != nil {
			return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to synchronize safeguard lifecycle", err)
		}
	}
	if err := s.recordAudit(ctx, actor, id, "update", before, safeguard, ""); err != nil {
		return dto.SafeguardResponse{}, err
	}
	return s.Get(ctx, id)
}
func (s *safeguardService) Verify(
	ctx context.Context,
	id uint,
	request dto.VerifySafeguardRequest,
	actor util.Actor,
) (dto.SafeguardResponse, error) {
	safeguard, err := s.safeguards.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardResponse{}, util.NotFound("safeguard")
		}
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	now := s.now()
	if request.VerifiedAt.After(now.Add(5 * time.Minute)) {
		return dto.SafeguardResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "verified_at cannot be in the future")
	}
	nextDueAt := request.VerifiedAt.AddDate(0, 0, safeguard.TestIntervalDays)
	if now.After(nextDueAt) {
		return dto.SafeguardResponse{}, util.NewError(http.StatusUnprocessableEntity, util.CodeValidation, "verification is already expired")
	}
	before := safeguard
	after, _, changed, err := s.reviews.VerifyWithSafeguard(ctx, repository.VerifyClosure{
		SafeguardID: id, FromStates: []string{"pending", "active", "expired", "invalid"},
		VerifiedAt: request.VerifiedAt, NextDueAt: nextDueAt,
		Evidence: request.EvidenceNote,
		ActorID:  actor.UserID, ActorName: actor.Username, Now: now,
	})
	if err != nil {
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to verify safeguard", err)
	}
	if !changed {
		return dto.SafeguardResponse{}, util.NewError(http.StatusConflict, util.CodeConflict, "safeguard state changed concurrently")
	}
	if err := s.recordAudit(ctx, actor, id, "verify", before, after, request.EvidenceNote); err != nil {
		return dto.SafeguardResponse{}, err
	}
	response := dto.NewSafeguardResponse(after, now)
	bundle, err := s.reviewBundle(ctx, []uint{id})
	if err != nil {
		return dto.SafeguardResponse{}, err
	}
	response.AttachReviewProjection(bundle[id], now)
	return response, nil
}
func (s *safeguardService) Invalidate(
	ctx context.Context,
	id uint,
	request dto.SafeguardActionRequest,
	actor util.Actor,
) (dto.SafeguardResponse, error) {
	return s.changeLifecycle(ctx, id, []string{"pending", "active", "expired"}, "invalid", "invalidate", request.Reason, actor)
}
func (s *safeguardService) Restore(
	ctx context.Context,
	id uint,
	request dto.SafeguardActionRequest,
	actor util.Actor,
) (dto.SafeguardResponse, error) {
	safeguard, err := s.safeguards.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardResponse{}, util.NotFound("safeguard")
		}
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	target := "pending"
	if safeguard.IsEffectiveAt(s.now()) {
		target = "active"
	}
	return s.changeLifecycle(ctx, id, []string{"invalid"}, target, "restore", request.Reason, actor)
}
func (s *safeguardService) changeLifecycle(
	ctx context.Context,
	id uint,
	from []string,
	to string,
	action string,
	reason string,
	actor util.Actor,
) (dto.SafeguardResponse, error) {
	before, err := s.safeguards.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return dto.SafeguardResponse{}, util.NotFound("safeguard")
		}
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to load safeguard", err)
	}
	evidence := fmt.Sprintf("%s\n[%s] %s", before.EvidenceNote, action, reason)
	changed, err := s.safeguards.SetLifecycle(ctx, id, from, to, map[string]any{"evidence_note": evidence})
	if err != nil {
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to change safeguard lifecycle", err)
	}
	if !changed {
		return dto.SafeguardResponse{}, util.NewError(http.StatusConflict, util.CodeStateTransition, "safeguard cannot transition from its current lifecycle state")
	}
	after, err := s.safeguards.GetByID(ctx, id)
	if err != nil {
		return dto.SafeguardResponse{}, util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to reload safeguard", err)
	}
	if err := s.recordAudit(ctx, actor, id, action, before, after, reason); err != nil {
		return dto.SafeguardResponse{}, err
	}
	return dto.NewSafeguardResponse(after, s.now()), nil
}
func (s *safeguardService) recordAudit(
	ctx context.Context,
	actor util.Actor,
	entityID uint,
	action string,
	before any,
	after any,
	summary string,
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
		ActorRole: actor.Role, EntityType: "safeguard", EntityID: entityID, Action: action,
		BeforeSnapshot: beforeJSON, AfterSnapshot: afterJSON,
		ResultSummary: util.CompactText(summary, 1000), CreatedAt: s.now(),
	}
	if err := s.audits.Record(ctx, log); err != nil {
		return util.WrapError(http.StatusInternalServerError, util.CodeInternal, "unable to record write audit", err)
	}
	return nil
}
