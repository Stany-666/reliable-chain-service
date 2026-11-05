package asset

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateWithdrawalHandlerInvalidJSON(t *testing.T) {
	// 准备：故意发送不完整的 JSON
	h := NewHandler(nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/withdrawals",
		strings.NewReader(`{"amount":`),
	)
	res := httptest.NewRecorder()

	// 执行
	h.CreateWithdrawal(res, req)
	t.Logf("状态码：%d，响应正文：%s", res.Code, res.Body.String())

	// 检查：由你补，状态码必须是 http.StatusBadRequest
	if res.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, res.Code)
	}
	if res.Body.String() != "invalid json\n" {
		t.Errorf("expected invalid json response, got %q", res.Body.String())
	}
}

func TestCreateWithdrawalHandlerInvalidAmount(t *testing.T) {
	// 准备：故意发送传入非法金额
	h := NewHandler(nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/withdrawals",
		strings.NewReader(`
		{"asset_id":1,
		"amount":"0",
		"to_address":"0x1234567890abcdef1234567890abcdef12345678"}
		`),
	)
	res := httptest.NewRecorder()

	// 执行
	h.CreateWithdrawal(res, req)
	t.Logf("状态码：%d，响应正文：%s", res.Code, res.Body.String())

	// 检查：由你补，状态码必须是 http.StatusBadRequest
	if res.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, res.Code)
	}
	if res.Body.String() != "invalid amount: amount must be greater than 0\n" {
		t.Errorf("expected invalid amount response, got %q", res.Body.String())
	}
}

func TestCreateWithdrawalHandlerInvalidAddress(t *testing.T) {
	// 准备：故意发送传入非法address
	h := NewHandler(nil)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/withdrawals",
		strings.NewReader(`
		{"asset_id":1,
		"amount":"1",
		"to_address":"not-an-address"}
		`),
	)
	res := httptest.NewRecorder()

	// 执行
	h.CreateWithdrawal(res, req)
	t.Logf("状态码：%d，响应正文：%s", res.Code, res.Body.String())

	// 检查：由你补，状态码必须是 http.StatusBadRequest
	if res.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, res.Code)
	}

}
