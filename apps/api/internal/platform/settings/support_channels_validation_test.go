package settings

import (
	"strings"
	"testing"
)

func TestNormalizeSupportChannelsTrimsAndSorts(t *testing.T) {
	setting, err := NormalizeSupportChannelsSetting(SupportChannelsSetting{Channels: []SupportChannel{
		{ID: " b ", Name: " Telegram ", Kind: "telegram", URL: " https://t.me/example ", Enabled: true, SortOrder: 2},
		{ID: "a", Name: "Discord", Kind: "discord", URL: "https://discord.gg/example", Enabled: true, SortOrder: 1},
	}})
	if err != nil || len(setting.Channels) != 2 || setting.Channels[0].ID != "a" || setting.Channels[1].Name != "Telegram" {
		t.Fatalf("normalized setting = %+v, err = %v", setting, err)
	}
}

func TestNormalizeSupportChannelsBounds(t *testing.T) {
	valid := SupportChannel{ID: "a", Name: "Support", Kind: "custom", URL: "https://example.com"}
	for name, mutate := range map[string]func(*SupportChannel){
		"missing id":     func(c *SupportChannel) { c.ID = "" },
		"invalid id":     func(c *SupportChannel) { c.ID = "a/b" },
		"missing name":   func(c *SupportChannel) { c.Name = " " },
		"long name":      func(c *SupportChannel) { c.Name = strings.Repeat("x", 81) },
		"long URL":       func(c *SupportChannel) { c.URL += "/" + strings.Repeat("x", 2048) },
		"URL whitespace": func(c *SupportChannel) { c.URL += "/a b" },
		"URL backslash":  func(c *SupportChannel) { c.URL = `https://example.com\@other.example` },
		"relative URL":   func(c *SupportChannel) { c.URL = "/support" },
		"negative order": func(c *SupportChannel) { c.SortOrder = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			channel := valid
			mutate(&channel)
			if _, err := NormalizeSupportChannelsSetting(SupportChannelsSetting{Channels: []SupportChannel{channel}}); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
	if _, err := NormalizeSupportChannelsSetting(SupportChannelsSetting{Channels: make([]SupportChannel, 21)}); err == nil {
		t.Fatal("expected channel limit validation failure")
	}
}

func TestSupportChannelsEmptyCollectionIsNotNull(t *testing.T) {
	setting, err := NormalizeSupportChannelsSetting(SupportChannelsSetting{})
	if err != nil || setting.Channels == nil || EnabledSupportChannels(setting).Channels == nil {
		t.Fatalf("empty setting must be non-nil: %+v, err=%v", setting, err)
	}
}
