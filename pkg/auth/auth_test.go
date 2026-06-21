package auth

import (
	"testing"
	"time"
)

func TestPasswordHashing(t *testing.T) {
	password := "test-password-123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}
	if hash == "" {
		t.Fatal("hash should not be empty")
	}
	if !CheckPassword(password, hash) {
		t.Error("CheckPassword should return true for correct password")
	}
	if CheckPassword("wrong-password", hash) {
		t.Error("CheckPassword should return false for wrong password")
	}
}

func TestJWTTokenGeneration(t *testing.T) {
	manager := NewJWTManager("test-secret-key-1234567890123456", 3600*time.Second, 86400*time.Second)

	tokens, err := manager.GenerateTokens(1, "testuser", false)
	if err != nil {
		t.Fatalf("GenerateTokens failed: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Fatal("access token should not be empty")
	}
	if tokens.RefreshToken == "" {
		t.Fatal("refresh token should not be empty")
	}

	claims, err := manager.ValidateToken(tokens.AccessToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.UserID != 1 {
		t.Errorf("expected user_id 1, got %d", claims.UserID)
	}
	if claims.Username != "testuser" {
		t.Errorf("expected username testuser, got %s", claims.Username)
	}

	// Test refresh
	newTokens, err := manager.RefreshTokens(tokens.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshTokens failed: %v", err)
	}
	if newTokens.AccessToken == "" {
		t.Fatal("new access token should not be empty")
	}
}

func TestJWTInvalidToken(t *testing.T) {
	manager := NewJWTManager("test-secret-key-1234567890123456", 3600, 86400)
	_, err := manager.ValidateToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}
