package ratio_setting

import "testing"

func TestBillingLookupPrefersOriginalThenBaseModel(t *testing.T) {
	modelPriceMap.Set("unit-test-price-model", 1.25)
	modelPriceMap.Set("unit-test-price-model-xhigh", 2.5)

	price, ok := GetModelPrice("unit-test-price-model-xhigh", false)
	if !ok {
		t.Fatal("expected original model price to be found")
	}
	if price != 2.5 {
		t.Fatalf("GetModelPrice original priority = %v, want 2.5", price)
	}

	modelPriceMap.Set("unit-test-price-fallback", 3.75)
	price, ok = GetModelPrice("unit-test-price-fallback-xhigh", false)
	if !ok {
		t.Fatal("expected base model price fallback to be found")
	}
	if price != 3.75 {
		t.Fatalf("GetModelPrice base fallback = %v, want 3.75", price)
	}

	modelRatioMap.Set("unit-test-ratio-fallback", 4.5)
	ratio, ok, matchName := GetModelRatio("unit-test-ratio-fallback-xhigh")
	if !ok {
		t.Fatal("expected base model ratio fallback to be found")
	}
	if ratio != 4.5 {
		t.Fatalf("GetModelRatio base fallback = %v, want 4.5", ratio)
	}
	if matchName != "unit-test-ratio-fallback" {
		t.Fatalf("GetModelRatio matchName = %q, want %q", matchName, "unit-test-ratio-fallback")
	}
}
