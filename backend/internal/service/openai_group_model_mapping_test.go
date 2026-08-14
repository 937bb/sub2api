package service

import "testing"

func TestGroupResolveOpenAIModelMapping(t *testing.T) {
	group := &Group{
		Platform: PlatformOpenAI,
		OpenAIModelMapping: map[string]string{
			"gpt-5.6-sol": "gpt-5.6-sol-wm",
			"gpt-5.*":     "gpt-5-default",
			"gpt-5.6-*":   "gpt-5.6-family",
		},
	}
	if got, matched := group.ResolveOpenAIModelMapping("gpt-5.6-sol"); !matched || got != "gpt-5.6-sol-wm" {
		t.Fatalf("exact mapping = (%q, %v)", got, matched)
	}
	if got, matched := group.ResolveOpenAIModelMapping("gpt-5.6-terra"); !matched || got != "gpt-5.6-family" {
		t.Fatalf("longest wildcard mapping = (%q, %v)", got, matched)
	}
	if got, matched := group.ResolveOpenAIModelMapping("gpt-4o"); matched || got != "gpt-4o" {
		t.Fatalf("unmapped model = (%q, %v)", got, matched)
	}
}

func TestNormalizeOpenAIGroupModelMapping(t *testing.T) {
	if _, err := NormalizeOpenAIGroupModelMapping(PlatformOpenAI, map[string]string{"gpt*5": "x"}); err == nil {
		t.Fatal("expected invalid wildcard error")
	}
	if got, err := NormalizeOpenAIGroupModelMapping(PlatformAnthropic, map[string]string{"x": "y"}); err != nil || len(got) != 0 {
		t.Fatalf("non-openai mapping = %#v, %v", got, err)
	}
}
