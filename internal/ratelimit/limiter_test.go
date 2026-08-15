package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterBlocksAfterLimit(t *testing.T) {
	limiter := New()
	now := time.Unix(1000, 0)
	limiter.now = func() time.Time { return now }
	policy := Policy{MaxAttempts: 2, Window: time.Minute}

	if ok, _ := limiter.Allow("login:ip", policy); !ok {
		t.Fatal("primer intento bloqueado")
	}
	if ok, _ := limiter.Allow("login:ip", policy); !ok {
		t.Fatal("segundo intento bloqueado")
	}
	if ok, retry := limiter.Allow("login:ip", policy); ok || retry <= 0 {
		t.Fatal("tercer intento debió bloquearse")
	}

	now = now.Add(time.Minute + time.Second)
	if ok, _ := limiter.Allow("login:ip", policy); !ok {
		t.Fatal("el límite no se reinició")
	}
}
