package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"hazop-safeguard-coverage/backend/internal/dto"
	"hazop-safeguard-coverage/backend/internal/service"
	"hazop-safeguard-coverage/backend/internal/util"
)

type SafeguardReviewHandler struct {
	service service.SafeguardReviewService
}

func NewSafeguardReviewHandler(value service.SafeguardReviewService) *SafeguardReviewHandler {
	return &SafeguardReviewHandler{service: value}
}

// Open 处理 POST /safeguards/:id/reviews
func (h *SafeguardReviewHandler) Open(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	var request dto.OpenSafeguardReviewRequest
	if !bindJSON(c, &request) {
		return
	}
	result, err := h.service.Open(c.Request.Context(), id, request, mustActor(c))
	respond(c, http.StatusCreated, result, err)
}

// ListBySafeguard 处理 GET /safeguards/:id/reviews（历次校验 + 未办结任务，时间倒序）
func (h *SafeguardReviewHandler) ListBySafeguard(c *gin.Context) {
	id, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	items, err := h.service.ListBySafeguard(c.Request.Context(), id)
	if err != nil {
		respond(c, http.StatusOK, nil, err)
		return
	}
	util.Success(c, http.StatusOK, gin.H{"items": items, "total": len(items)})
}

// ListOpen 处理 GET /safeguard-reviews（待复评/逾期任务总览）
func (h *SafeguardReviewHandler) ListOpen(c *gin.Context) {
	safeguardID, ok := optionalUint(c, "safeguard_id")
	if !ok {
		return
	}
	overdue, err := strconv.ParseBool(defaultString(c.Query("overdue_only"), "false"))
	if err != nil {
		util.Fail(c, util.NewError(http.StatusBadRequest, util.CodeBadRequest, "overdue_only must be true or false"))
		return
	}
	status := c.Query("status")
	if status != "" && status != "open" {
		util.Fail(c, util.NewError(http.StatusBadRequest, util.CodeBadRequest, "status only supports open"))
		return
	}
	page, size := util.Pagination(c)
	result, serviceErr := h.service.ListOpen(c.Request.Context(), dto.SafeguardReviewQuery{
		SafeguardID: safeguardID, OverdueOnly: overdue, Page: page, PageSize: size,
	})
	respond(c, http.StatusOK, result, serviceErr)
}

func (h *SafeguardReviewHandler) Get(c *gin.Context) {
	safeguardID, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	id, err := util.ParseUintParam(c, "review_id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	result, err := h.service.Get(c.Request.Context(), safeguardID, id)
	respond(c, http.StatusOK, result, err)
}

func (h *SafeguardReviewHandler) Complete(c *gin.Context) {
	safeguardID, err := util.ParseUintParam(c, "id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	id, err := util.ParseUintParam(c, "review_id")
	if err != nil {
		util.Fail(c, err)
		return
	}
	var request dto.CompleteSafeguardReviewRequest
	if !bindJSON(c, &request) {
		return
	}
	result, err := h.service.Complete(c.Request.Context(), safeguardID, id, request, mustActor(c))
	respond(c, http.StatusOK, result, err)
}
