package user

import (
	"context"
	"errors"
	"fmt"
	"reliable-chain-service/internal/auth"

	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo      *Repository
	jwtSecret []byte
}

func NewService(repo *Repository, jwtSecret []byte) *Service {
	return &Service{
		repo:      repo,
		jwtSecret: jwtSecret,
	}
}

func ValidateCreateUser(req CreateUserRequest) error {
	if req.Username == "" {
		return fmt.Errorf("%w: username is required", ErrInvalidInput)
	}
	if req.Email == "" {
		return fmt.Errorf("%w: email is required", ErrInvalidInput)
	}
	if req.Password == "" {
		return fmt.Errorf("%w: password is required", ErrInvalidInput)
	}
	return nil
}

func ValidateLoginUser(req LoginUserRequest) error {
	if req.Username == "" {
		return fmt.Errorf("%w: username is required", ErrInvalidInput)
	}
	if req.Password == "" {
		return fmt.Errorf("%w: password is required", ErrInvalidInput)
	}
	return nil
}

func (s *Service) CreateUser(ctx context.Context, req CreateUserRequest) error {

	if err := ValidateCreateUser(req); err != nil {
		return err
	}

	exists, err := s.repo.ExistsByUsernameEmail(ctx, req.Username, req.Email)
	if err != nil {
		return err
	}
	if exists {
		return ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
	}
	err = s.repo.CreateUser(ctx, user)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) LoginUser(ctx context.Context, req LoginUserRequest) (string, error) {
	//校验请求参数
	if err := ValidateLoginUser(req); err != nil {
		return "", err
	}
	//查询用户
	user, err := s.repo.GetUserByUsername(ctx, req.Username)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return "", ErrInvalidCredentials
		}
		return "", err
	}

	//比较密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return "", ErrInvalidCredentials //密码错误
	}

	return auth.GenerateToken(user.ID, s.jwtSecret)
}
