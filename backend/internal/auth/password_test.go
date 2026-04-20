package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_CorrectPasswordVerifies(t *testing.T) {
	hash, err := HashPassword("super-secret-2026")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := VerifyPassword("super-secret-2026", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Error("correct password did not verify")
	}
}

func TestHashPassword_WrongPasswordRejected(t *testing.T) {
	hash, _ := HashPassword("correct")
	ok, _ := VerifyPassword("wrong", hash)
	if ok {
		t.Error("wrong password accepted")
	}
}

func TestHashPassword_RehashesAreDifferent(t *testing.T) {
	h1, _ := HashPassword("same-password")
	h2, _ := HashPassword("same-password")
	if h1 == h2 {
		t.Error("two hashes of same password identical — salt not applied")
	}
}

func TestHashPassword_Format(t *testing.T) {
	h, _ := HashPassword("x")
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Errorf("hash does not start with argon2id marker: %s", h)
	}
}
