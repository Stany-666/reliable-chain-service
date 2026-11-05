package asset

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateWithdrawalAmount_Valid(t *testing.T) {
	err := ValidateWithdrawalAmount("100")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
func TestValidateWithdrawalAmount_Negative(t *testing.T) {
	err := ValidateWithdrawalAmount("-5")
	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}
func TestValidateWithdrawalAmount_Zero(t *testing.T) {
	err := ValidateWithdrawalAmount("0")
	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}
func TestValidateWithdrawalAmount_Invalid(t *testing.T) {
	err := ValidateWithdrawalAmount("abc")
	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}
func TestValidateWithdrawalAmount_Empty(t *testing.T) {
	err := ValidateWithdrawalAmount("")
	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestValidateWithdrawalAmount_Float(t *testing.T) {
	err := ValidateWithdrawalAmount("1.3")
	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("expected ErrInvalidAmount, got %v", err)
	}
}

func TestValidateWithdrawalAddress_Valid(t *testing.T) {
	err := ValidateWithdrawalAddress("0x" + strings.Repeat("1", 40))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
func TestValidateWithdrawalAddress_Invalid(t *testing.T) {
	err := ValidateWithdrawalAddress("0x" + strings.Repeat("1", 39))
	if !errors.Is(err, ErrInvalidAddress) {
		t.Errorf("expected ErrInvalidAddress, got %v", err)
	}
}

func TestValidateWithdrawalAddress_InvalidString(t *testing.T) {
	err := ValidateWithdrawalAddress("abc")
	if !errors.Is(err, ErrInvalidAddress) {
		t.Errorf("expected ErrInvalidAddress, got %v", err)
	}
}
