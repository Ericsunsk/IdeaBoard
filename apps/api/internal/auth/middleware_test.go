package auth

import "testing"

func TestShouldAutoBindLegacyUsername_DefaultDisabled(t *testing.T) {
	t.Setenv(legacyUsernameBindingEnvKey, "")
	if shouldAutoBindLegacyUsername("") {
		t.Fatal("expected legacy username auto binding to be disabled by default")
	}
}

func TestShouldAutoBindLegacyUsername_ExplicitEnabled(t *testing.T) {
	t.Setenv(legacyUsernameBindingEnvKey, "true")
	if !shouldAutoBindLegacyUsername("") {
		t.Fatal("expected legacy username auto binding to be enabled when flag=true")
	}

	t.Setenv(legacyUsernameBindingEnvKey, "1")
	if !shouldAutoBindLegacyUsername("") {
		t.Fatal("expected legacy username auto binding to be enabled when flag=1")
	}
}
