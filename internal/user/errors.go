package user

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid input")                    // 请求参数不合法
	ErrUserExists         = errors.New("username or email already exists") // 注册时用户已存在
	ErrUserNotFound       = errors.New("user not found")                   // 查不到这个用户
	ErrInvalidCredentials = errors.New("invalid username or password")     // 登录时用户名或密码错
)
