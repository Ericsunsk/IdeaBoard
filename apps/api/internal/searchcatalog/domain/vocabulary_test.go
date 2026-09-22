package domain

import "testing"

func TestSearchProviderVocabularyIsClosed(t *testing.T) {
	if len(Providers) != 2 || Providers[0] != ProviderUnsplash || Providers[1] != ProviderPexels {
		t.Fatalf("Providers = %#v, want unsplash and pexels in display order", Providers)
	}
	for _, provider := range []Provider{ProviderUnsplash, ProviderPexels} {
		if !IsProvider(provider) {
			t.Fatalf("IsProvider(%q) = false", provider)
		}
	}
	for _, provider := range []Provider{"vision", "", "evil"} {
		if IsProvider(provider) {
			t.Fatalf("IsProvider(%q) = true, want closed vocabulary rejection", provider)
		}
	}
}
