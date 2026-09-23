package principal

import (
	"testing"

	"github.com/google/uuid"
)

func TestWithControlGrantOperatorRoleGrant(t *testing.T) {
	publicID := uuid.New()

	newAdmin := func() Principal {
		p, err := New(11, publicID, "auth-op-11", "Operator", "admin")
		if err != nil {
			t.Fatalf("New() = %v", err)
		}
		return p
	}

	t.Run("admin human session receives wildcard grant", func(t *testing.T) {
		granted, err := WithControlGrant(newAdmin())
		if err != nil || !granted.IsValid() {
			t.Fatalf("WithControlGrant() = %+v, %v; want valid", granted, err)
		}
		for _, capability := range ControlCapabilities() {
			if !HasControlCapability(granted, capability) {
				t.Fatalf("granted operator must hold %q via wildcard", capability)
			}
		}
		if granted.HasCapability(CapabilityWorkspaceRead) {
			t.Fatal("grant must not add public capabilities")
		}
	})

	t.Run("non-admin user denied", func(t *testing.T) {
		p, err := New(12, publicID, "auth-12", "Member", "user")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := WithControlGrant(p); err == nil {
			t.Fatal("non-admin must not receive the control grant")
		}
	})

	t.Run("service account and api key denied", func(t *testing.T) {
		for _, subjectType := range []SubjectType{SubjectServiceAccount, SubjectAPIKey, SubjectOAuthApp, SubjectInternalWorker} {
			p, err := NewExtended(13, publicID, "auth-13", "Svc", "admin",
				subjectType, "service", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := WithControlGrant(p); err == nil {
				t.Fatalf("subject %q must not receive the operator role grant", subjectType)
			}
		}
	})

	t.Run("public capability holder denied on control", func(t *testing.T) {
		p, err := NewExtended(14, publicID, "auth-14", "Pub", "user",
			SubjectHumanSession, "session", "", []string{CapabilityWorkspaceRead, CapabilityWorkspaceWrite})
		if err != nil {
			t.Fatal(err)
		}
		for _, capability := range ControlCapabilities() {
			if HasControlCapability(p, capability) {
				t.Fatalf("public-cap holder must not hold %q", capability)
			}
		}
	})

	t.Run("admin human session constructed with family grant", func(t *testing.T) {
		p, err := NewExtended(16, publicID, "auth-16", "Op", "admin",
			SubjectHumanSession, "session", "", []string{ControlCapabilityCredits})
		if err != nil || !p.IsValid() {
			t.Fatalf("NewExtended() = %+v, %v; want valid operator grant", p, err)
		}
		if !HasControlCapability(p, ControlCapabilityCredits) {
			t.Fatal("explicit family grant must hold")
		}
		if HasControlCapability(p, ControlCapabilityUsers) {
			t.Fatal("family grant must not imply sibling families")
		}
	})

	t.Run("non-admin human cannot be constructed with control grant", func(t *testing.T) {
		if _, err := NewExtended(17, publicID, "auth-17", "Member", "user",
			SubjectHumanSession, "session", "", []string{ControlCapabilityCredits}); err == nil {
			t.Fatal("non-admin human with control capability must be rejected")
		}
	})

	t.Run("least privilege family grant without wildcard", func(t *testing.T) {
		worker, err := NewExtended(15, publicID, "worker-15", "Worker", "admin",
			SubjectInternalWorker, "service", "", []string{ControlCapabilityCredits})
		if err != nil {
			t.Fatal(err)
		}
		if !HasControlCapability(worker, ControlCapabilityCredits) {
			t.Fatal("direct family grant must pass")
		}
		if HasControlCapability(worker, ControlCapabilityUsers) {
			t.Fatal("family grant must not imply sibling families")
		}
	})
}
