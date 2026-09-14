package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/model"
	"hazop-safeguard-coverage/backend/internal/repository"
	"hazop-safeguard-coverage/backend/internal/util"
)

func reviewTestFixture(t *testing.T) (SafeguardReviewService, SafeguardService, *gormFixture) {
	t.Helper()
	db := testDB(t)
	nodeRepo := repository.NewProcessNodeRepository(db)
	scenarioRepo := repository.NewDeviationScenarioRepository(db)
	safeguardRepo := repository.NewSafeguardRepository(db)
	reviewRepo := repository.NewSafeguardReviewRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	now := time.Now().UTC().Truncate(time.Second).Add(-30 * time.Second)
	node := model.ProcessNode{
		NodeCode: "R-900", Name: "Review Test Node", UnitName: "u", Medium: "gas",
		DesignPressure: 1, DesignTemperature: 20, OwnerTeam: "team", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := nodeRepo.Create(context.Background(), &node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	scenario := model.DeviationScenario{
		ProcessNodeID: node.ID, Guideword: "more", Parameter: "pressure",
		Cause: "blocked outlet", Consequence: "rupture", Likelihood: 3, Severity: 5,
		ScenarioState: "analyzed", Version: 1,
		CreatedBy: 7, CreatedByName: "engineer", CreatedAt: now, UpdatedAt: now,
	}
	if err := scenarioRepo.Create(context.Background(), &scenario); err != nil {
		t.Fatalf("create scenario: %v", err)
	}
	verifiedAt := now.AddDate(0, 0, -400)
	safeguard := model.Safeguard{
		Name: "Review test valve", SafeguardType: "relief",
		TargetScenarioID: scenario.ID, IndependenceKey: "PSV-TEST-1",
		Effectiveness: 0.9, TestIntervalDays: 365, LastVerifiedAt: &verifiedAt,
		LifecycleState: "expired", EvidenceNote: "old certificate", CreatedAt: now, UpdatedAt: now,
	}
	if err := safeguardRepo.Create(context.Background(), &safeguard); err != nil {
		t.Fatalf("create safeguard: %v", err)
	}
	reviewSvc := NewSafeguardReviewService(reviewRepo, safeguardRepo, auditRepo)
	safeguardSvc := NewSafeguardService(safeguardRepo, reviewRepo, scenarioRepo, auditRepo)
	return reviewSvc, safeguardSvc, &gormFixture{safeguardID: safeguard.ID, now: now}
}

type gormFixture struct {
	safeguardID uint
	now         time.Time
}

func TestReviewOpenCompletePassUpdatesRegister(t *testing.T) {
	reviewSvc, safeguardSvc, fixture := reviewTestFixture(t)
	actor := util.Actor{UserID: 20, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-open"}
	dueAt := fixture.now.AddDate(0, 0, 7)
	opened, err := reviewSvc.Open(context.Background(), fixture.safeguardID, dto.OpenSafeguardReviewRequest{
		Reason: "certificate expired; schedule proof test", DueAt: &dueAt,
	}, actor)
	if err != nil {
		t.Fatalf("open review: %v", err)
	}
	if opened.Status != "open" || opened.Origin != "expiry" || opened.Sequence != 1 {
		t.Fatalf("unexpected opened review: %#v", opened)
	}

	// 同一保护层同时只能有一项未完成复评。
	_, err = reviewSvc.Open(context.Background(), fixture.safeguardID, dto.OpenSafeguardReviewRequest{
		Reason: "second open must be rejected", DueAt: &dueAt,
	}, actor)
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Status != 409 {
		t.Fatalf("duplicate open should be 409, got %v", err)
	}

	checkedAt := fixture.now
	nextDueAt := fixture.now.AddDate(0, 0, 365)
	completed, err := reviewSvc.Complete(context.Background(), fixture.safeguardID, opened.ID, dto.CompleteSafeguardReviewRequest{
		CheckedAt: checkedAt, Conclusion: "pass", Evidence: "new proof-test certificate PT-001",
		Responsible: "reviewer", NextDueAt: nextDueAt,
	}, actor)
	if err != nil {
		t.Fatalf("complete review pass: %v", err)
	}
	if completed.Status != "completed" || completed.Conclusion != "pass" || !completed.NextDueAt.Equal(nextDueAt) {
		t.Fatalf("unexpected completed review: %#v", completed)
	}

	// 历史记录不可覆盖：重复办结应 409。
	_, err = reviewSvc.Complete(context.Background(), fixture.safeguardID, opened.ID, dto.CompleteSafeguardReviewRequest{
		CheckedAt: checkedAt, Conclusion: "fail", Evidence: "attempted overwrite",
		Responsible: "reviewer", NextDueAt: nextDueAt,
	}, actor)
	if !errors.As(err, &appErr) || appErr.Status != 409 {
		t.Fatalf("re-complete should be 409, got %v", err)
	}

	// 合格后台账有效期更新、保护层恢复可用，且投影到详情。
	detail, err := safeguardSvc.Get(context.Background(), fixture.safeguardID)
	if err != nil {
		t.Fatalf("get safeguard: %v", err)
	}
	if detail.LifecycleState != "active" {
		t.Fatalf("expected active after pass, got %s", detail.LifecycleState)
	}
	if detail.VerificationExpires == nil || !detail.VerificationExpires.Equal(nextDueAt) {
		t.Fatalf("register expiry should be next_due_at, got %#v", detail.VerificationExpires)
	}
	if detail.ReviewPending {
		t.Fatalf("no open review should remain after pass")
	}
	if detail.LatestConclusion != "pass" || detail.CompletedReviewCnt != 1 {
		t.Fatalf("unexpected latest projection: %#v", detail)
	}
}

func TestReviewCompleteFailKeepsUnavailableAndOpensFollowup(t *testing.T) {
	reviewSvc, safeguardSvc, fixture := reviewTestFixture(t)
	actor := util.Actor{UserID: 21, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-fail"}
	dueAt := fixture.now.AddDate(0, 0, 7)
	opened, err := reviewSvc.Open(context.Background(), fixture.safeguardID, dto.OpenSafeguardReviewRequest{
		Reason: "expired; proof test", DueAt: &dueAt,
	}, actor)
	if err != nil {
		t.Fatalf("open review: %v", err)
	}
	nextDueAt := fixture.now.AddDate(0, 0, 30)
	completed, err := reviewSvc.Complete(context.Background(), fixture.safeguardID, opened.ID, dto.CompleteSafeguardReviewRequest{
		CheckedAt: fixture.now, Conclusion: "fail", Evidence: "stroke time out of spec",
		Responsible: "reviewer", NextDueAt: nextDueAt,
	}, actor)
	if err != nil {
		t.Fatalf("complete review fail: %v", err)
	}
	if completed.Conclusion != "fail" {
		t.Fatalf("expected fail conclusion, got %s", completed.Conclusion)
	}

	// 不合格：台账保持不可用。
	detail, err := safeguardSvc.Get(context.Background(), fixture.safeguardID)
	if err != nil {
		t.Fatalf("get safeguard: %v", err)
	}
	if detail.LifecycleState != "invalid" {
		t.Fatalf("expected invalid after fail, got %s", detail.LifecycleState)
	}
	if !detail.ReviewPending || detail.OpenReviewID == 0 {
		t.Fatalf("follow-up review should be pending: %#v", detail)
	}
	if detail.LatestConclusion != "fail" {
		t.Fatalf("latest conclusion should be fail, got %q", detail.LatestConclusion)
	}

	// 不合格自动生成了跟进复评任务，且历史记录保留。
	history, err := reviewSvc.ListBySafeguard(context.Background(), fixture.safeguardID)
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 records (1 completed + 1 open follow-up), got %d", len(history))
	}
	if history[0].Status != "open" || history[0].Origin != "failure" || history[0].ParentReviewID == nil || *history[0].ParentReviewID != opened.ID {
		t.Fatalf("unexpected follow-up review: %#v", history[0])
	}
	if history[1].Conclusion != "fail" || history[1].ID != completed.ID {
		t.Fatalf("fail history must be preserved: %#v", history[1])
	}

	// 跟进任务逾期后应在总览中可查且标记 overdue。
	list, err := reviewSvc.ListOpen(context.Background(), dto.SafeguardReviewQuery{OverdueOnly: false, Page: 1, PageSize: 20})
	if err != nil || list.Total != 1 {
		t.Fatalf("open review list: total=%d err=%v", list.Total, err)
	}
}

func TestReviewOverdueFlag(t *testing.T) {
	reviewSvc, _, fixture := reviewTestFixture(t)
	actor := util.Actor{UserID: 22, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-overdue"}
	pastDue := fixture.now.AddDate(0, 0, -1)
	opened, err := reviewSvc.Open(context.Background(), fixture.safeguardID, dto.OpenSafeguardReviewRequest{
		Reason: "overdue proof test", DueAt: &pastDue,
	}, actor)
	if err != nil {
		t.Fatalf("open review: %v", err)
	}
	if !opened.Overdue {
		t.Fatalf("review due in the past must be flagged overdue")
	}
	overdue, err := reviewSvc.ListOpen(context.Background(), dto.SafeguardReviewQuery{OverdueOnly: true, Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list overdue: %v", err)
	}
	if overdue.Total != 1 || overdue.Items[0].ID != opened.ID {
		t.Fatalf("overdue overview should contain review %d, got %#v", opened.ID, overdue)
	}
}

func TestReviewRejectsNextDueBeyondInterval(t *testing.T) {
	reviewSvc, _, fixture := reviewTestFixture(t)
	actor := util.Actor{UserID: 23, Username: "reviewer", Role: "safety_reviewer", RequestID: "req-interval"}
	dueAt := fixture.now.AddDate(0, 0, 7)
	opened, err := reviewSvc.Open(context.Background(), fixture.safeguardID, dto.OpenSafeguardReviewRequest{
		Reason: "expired proof test", DueAt: &dueAt,
	}, actor)
	if err != nil {
		t.Fatalf("open review: %v", err)
	}
	_, err = reviewSvc.Complete(context.Background(), fixture.safeguardID, opened.ID, dto.CompleteSafeguardReviewRequest{
		CheckedAt: fixture.now, Conclusion: "pass", Evidence: "cert",
		Responsible: "reviewer", NextDueAt: fixture.now.AddDate(0, 0, 366),
	}, actor)
	var appErr *util.AppError
	if !errors.As(err, &appErr) || appErr.Status != 422 {
		t.Fatalf("next_due beyond interval should be 422, got %v", err)
	}
}
