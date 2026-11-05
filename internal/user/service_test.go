package user

import (
	"errors"
	"testing"
)

func TestValidateLoginUser_EmptyUsername(t *testing.T) {
	req := LoginUserRequest{Username: "", Password: "123456"}

	err := ValidateLoginUser(req)

	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestValidateLoginUser_EmptyPassword(t *testing.T) {
	req := LoginUserRequest{Username: "babe", Password: ""}

	err := ValidateLoginUser(req)

	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestValidateLoginUser_Valid(t *testing.T) {
	req := LoginUserRequest{Username: "babe", Password: "123456"}

	err := ValidateLoginUser(req)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
