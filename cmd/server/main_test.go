package main

import (
	"testing"
)

func TestGenerateToken(t *testing.T) {
	tok1 := generateToken()
	tok2 := generateToken()
	if len(tok1) != 32 {
		t.Fatalf("expected 32 hex chars, got %d", len(tok1))
	}
	if tok1 == tok2 {
		t.Fatal("expected random tokens to differ")
	}
}

func TestEnvOr(t *testing.T) {
	t.Setenv("TEST_KEY_123", "custom")
	if val := envOr("TEST_KEY_123", "fallback"); val != "custom" {
		t.Fatalf("want custom, got %s", val)
	}
	if val := envOr("TEST_NON_EXISTENT", "fallback"); val != "fallback" {
		t.Fatalf("want fallback, got %s", val)
	}
}
