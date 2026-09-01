package deploy

import (
	"testing"
	"time"
)

func TestCooldownReservationsSerializeConcurrentDeploys(t *testing.T) {
	c := NewCooldown(time.Minute)
	first, ok, retry := c.Reserve("token", "127.0.0.1")
	if !ok || first == nil || retry != 0 {
		t.Fatalf("first reservation = (%v, %v, %v); want success", first, ok, retry)
	}
	if available, wait := c.Check("other-token", "other-ip"); available || wait <= 0 {
		t.Fatalf("check during in-flight reservation = (%v, %v); want rate limited", available, wait)
	}
	second, ok, retry := c.Reserve("other-token", "other-ip")
	if ok || second != nil || retry <= 0 {
		t.Fatalf("second reservation = (%v, %v, %v); want rejection", second, ok, retry)
	}

	first.Cancel()
	if available, wait := c.Check("other-token", "other-ip"); !available || wait != 0 {
		t.Fatalf("check after cancellation = (%v, %v); want available", available, wait)
	}
	second, ok, retry = c.Reserve("other-token", "other-ip")
	if !ok || second == nil || retry != 0 {
		t.Fatalf("reservation after cancellation = (%v, %v, %v); want success", second, ok, retry)
	}
	if got := second.Commit(); got != time.Minute {
		t.Fatalf("commit wait = %s, want 1m", got)
	}
	if available, wait := c.Check("new-token", "new-ip"); available || wait <= 0 {
		t.Fatalf("check after commit = (%v, %v); want rate limited", available, wait)
	}

	// Commit/Cancel are deliberately idempotent; a deferred cleanup must not
	// clear the cooldown that was already committed.
	second.Cancel()
	if available, _ := c.Check("new-token", "new-ip"); available {
		t.Fatal("cancellation after commit cleared the cooldown")
	}
}

func TestCooldownReservationCancelDoesNotConsumeWindow(t *testing.T) {
	c := NewCooldown(time.Minute)
	r, ok, _ := c.Reserve("token", "ip")
	if !ok {
		t.Fatal("reserve failed")
	}
	r.Cancel()
	r.Cancel()
	if available, wait := c.Check("token", "ip"); !available || wait != 0 {
		t.Fatalf("check after cancellation = (%v, %v); want available", available, wait)
	}
}

func TestCooldownSetWindowZeroDisablesExistingCooldown(t *testing.T) {
	c := NewCooldown(time.Minute)
	c.Consume("token", "ip")
	if available, _ := c.Check("token", "ip"); available {
		t.Fatal("initial Consume did not start cooldown")
	}
	c.SetWindow(0)
	if available, wait := c.Check("token", "ip"); !available || wait != 0 {
		t.Fatalf("check after disabling cooldown = (%v, %v); want available", available, wait)
	}
	reservation, ok, wait := c.Reserve("token", "ip")
	if !ok || wait != 0 || reservation == nil {
		t.Fatalf("reserve with disabled cooldown = (%v, %v, %v); want success", reservation, ok, wait)
	}
	reservation.Commit()
	if available, wait := c.Check("token", "ip"); !available || wait != 0 {
		t.Fatalf("check after disabled commit = (%v, %v); want available", available, wait)
	}
}
