package model

import "testing"

func TestStatusBackwardCompatEmptyIsActive(t *testing.T) {
	u := User{ID: "legacy", PK: "PK"} // no status set
	if !u.IsActive() {
		t.Fatal("empty status should be treated as active")
	}
	if u.IsPending() {
		t.Fatal("empty status should not be pending")
	}
}

func TestStatusExplicit(t *testing.T) {
	active := User{Status: StatusActive}
	pending := User{Status: StatusPending}
	if !active.IsActive() || active.IsPending() {
		t.Fatal("active user misclassified")
	}
	if !pending.IsPending() || pending.IsActive() {
		t.Fatal("pending user misclassified")
	}
}

func TestActivePendingFilters(t *testing.T) {
	e := &Envis{Users: []User{
		{ID: "a", Status: StatusActive, PK: "A"},
		{ID: "b", Status: StatusPending, PK: "B"},
		{ID: "legacy", PK: "L"}, // empty => active
	}}
	if got := len(e.ActiveUsers()); got != 2 {
		t.Fatalf("ActiveUsers = %d, want 2", got)
	}
	if got := len(e.PendingUsers()); got != 1 {
		t.Fatalf("PendingUsers = %d, want 1", got)
	}
}

func TestFindActiveUserByPKIgnoresPending(t *testing.T) {
	e := &Envis{Users: []User{
		{ID: "b", Status: StatusPending, PK: "B"},
	}}
	if e.FindActiveUserByPK("B") != nil {
		t.Fatal("pending user should not be found as active")
	}
	// Flip to active and it should be found.
	e.Users[0].Status = StatusActive
	if e.FindActiveUserByPK("B") == nil {
		t.Fatal("active user should be found")
	}
}
