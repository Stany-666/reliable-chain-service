package asset

import (
	"encoding/json"
	"errors"
	"net/http"
	"reliable-chain-service/internal/auth"
	"strconv"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateWithdrawal(w http.ResponseWriter, r *http.Request) {
	// 1. 解析请求 JSON
	var req CreateWithdrawalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	// 2. 调用 service.CreateWithdrawal
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	withdrawalID, err := h.service.CreateWithdrawal(r.Context(), userID, req.AssetID, req.Amount, req.ToAddress)

	// 3. 把业务错误转换成 HTTP 状态码
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidAmount):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrInvalidAddress):
			http.Error(w, err.Error(), http.StatusBadRequest)
		case errors.Is(err, ErrInsufficientBalance):
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	// 4. 成功返回 201 和提现单 ID
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int64{
		"withdrawal_id": withdrawalID,
	})
}

func (h *Handler) CancelWithdrawal(w http.ResponseWriter, r *http.Request) {
	// 1. 读取路径参数，并转换为正整数
	idText := r.PathValue("id")
	withdrawalID, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || withdrawalID <= 0 {
		http.Error(w, "invalid withdrawal id", http.StatusBadRequest)
		return
	}

	// 2. 获取用户 ID，调用 service.CancelWithdrawal
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	err = h.service.CancelWithdrawal(r.Context(), userID, withdrawalID)
	// 3. 业务错误转成 HTTP 状态码
	if err != nil {
		switch {
		case errors.Is(err, ErrWithdrawalNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, ErrInvalidStatus):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}
	// 4. 成功返回 204
	w.WriteHeader(http.StatusNoContent)

}
