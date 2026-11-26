package auth_test

import (
	"testing"

	"personal-server/internal/auth"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := auth.HashPassword("s3cr3t-pass!")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	ok, err := auth.VerifyPassword("s3cr3t-pass!", hash)
	if err != nil || !ok {
		t.Fatalf("expected valid password verification, ok=%v err=%v", ok, err)
	}
	wrong, err := auth.VerifyPassword("wrong-pass", hash)
	if err != nil || wrong {
		t.Fatalf("expected invalid password rejection, ok=%v err=%v", wrong, err)
	}
}
