package taskbilling

import "testing"

func TestPatternGenerationSKUIsStableAndModelSpecific(t *testing.T) {
	first, err := PatternGenerationRequest(" GPT-Image-1 ")
	if err != nil {
		t.Fatal(err)
	}
	firstSKU, err := ResolveSKU(first)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := PatternGenerationRequest("gpt-image-1")
	secondSKU, _ := ResolveSKU(second)
	other, _ := PatternGenerationRequest("imagen-4")
	otherSKU, _ := ResolveSKU(other)
	if firstSKU != secondSKU {
		t.Fatalf("equivalent canonical models resolved to %q and %q", firstSKU, secondSKU)
	}
	if firstSKU == otherSKU {
		t.Fatalf("different models resolved to the same SKU %q", firstSKU)
	}
}
