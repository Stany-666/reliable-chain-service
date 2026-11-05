package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const userIDKey contextKey = "user_id"

func RequireAuth(secret []byte, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 从请求头取出 JWT
		authorization := r.Header.Get("Authorization")
		parts := strings.Fields(authorization)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// 2. 调用 VerifyToken，取得用户 ID
		userID, err := VerifyToken(parts[1], secret)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// 3. 把用户 ID 放入请求上下文
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		// 4. 验证通过后，交给 next 处理
		next.ServeHTTP(w, r.WithContext(ctx))

	})
}

func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userIDKey).(int64)
	return userID, ok
}
