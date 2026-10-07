package settings

import (
	"context"
	"testing"

	"ideaboard/api/internal/db/dbutil"
	"ideaboard/api/internal/testdb"
)

func TestRuntimeSettingsSupportChannelsDefaultsAndPersists(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	service := NewRuntimeSettingsService(NewSystemConfigRepository(db))

	if got, err := service.GetSupportChannels(ctx); err != nil || len(got.Channels) != 0 {
		t.Fatalf("default support channels = %+v, err=%v", got, err)
	}

	updated, err := service.UpdateSupportChannels(ctx, SupportChannelsSetting{Channels: []SupportChannel{
		{ID: " telegram ", Name: " Telegram ", Kind: "telegram", URL: " https://t.me/example ", Enabled: true, SortOrder: 20},
		{ID: "discord", Name: "Discord", Kind: "discord", URL: "https://discord.gg/example", Enabled: false, SortOrder: 10},
	}})
	if err != nil {
		t.Fatalf("update support channels: %v", err)
	}
	if len(updated.Channels) != 2 || updated.Channels[0].ID != "discord" || updated.Channels[1].ID != "telegram" {
		t.Fatalf("normalized support channels = %+v", updated)
	}

	got, err := service.GetSupportChannels(ctx)
	if err != nil || len(got.Channels) != 2 || got.Channels[0].URL != "https://discord.gg/example" {
		t.Fatalf("persisted support channels = %+v, err=%v", got, err)
	}
	enabled := EnabledSupportChannels(got)
	if len(enabled.Channels) != 1 || enabled.Channels[0].ID != "telegram" {
		t.Fatalf("enabled support channels = %+v", enabled)
	}

}

func TestNormalizeSupportChannelsRejectsInvalidEntries(t *testing.T) {
	tests := []SupportChannelsSetting{
		{Channels: []SupportChannel{{ID: "same", Name: "A", Kind: "custom", URL: "https://a.example"}, {ID: "same", Name: "B", Kind: "custom", URL: "https://b.example"}}},
		{Channels: []SupportChannel{{ID: "x", Name: "A", Kind: "unknown", URL: "https://a.example"}}},
		{Channels: []SupportChannel{{ID: "x", Name: "A", Kind: "custom", URL: "javascript:alert(1)"}}},
		{Channels: []SupportChannel{{ID: "x", Name: "A", Kind: "custom", URL: "https://user:pass@example.com"}}},
	}
	for _, setting := range tests {
		if _, err := NormalizeSupportChannelsSetting(setting); err == nil {
			t.Fatalf("NormalizeSupportChannelsSetting(%+v) succeeded, want validation error", setting)
		}
	}
}

func TestRuntimeSettingsSupportChannelsUpdateJoinsAmbientTransaction(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	repo := NewSystemConfigRepository(db)
	service := NewRuntimeSettingsService(repo)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin carrier tx: %v", err)
	}
	carrier := dbutil.ContextWithAmbientTx(ctx, tx)
	_, err = service.UpdateSupportChannels(carrier, SupportChannelsSetting{Channels: []SupportChannel{{ID: "discord", Name: "Discord", Kind: "discord", URL: "https://discord.gg/example", Enabled: true}}})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("update on carrier: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback carrier tx: %v", err)
	}
	if _, err := repo.Get(ctx, supportChannelsKey); err == nil {
		t.Fatal("support channels persisted after carrier rollback")
	}
}
