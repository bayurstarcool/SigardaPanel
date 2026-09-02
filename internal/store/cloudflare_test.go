package store

import "testing"

func TestCloudflareStore_SaveConfigPerUser(t *testing.T) {
	database := testDB(t)
	cloudflare := NewCloudflareStore(database)

	if err := cloudflare.SaveConfig(1, CloudflareConfig{
		UserID:   1,
		APIToken: "token-user-1",
		Email:    "one@example.com",
	}); err != nil {
		t.Fatalf("save user 1 config: %v", err)
	}
	if err := cloudflare.SaveConfig(3, CloudflareConfig{
		UserID:   3,
		APIToken: "token-user-3",
		Email:    "three@example.com",
	}); err != nil {
		t.Fatalf("save user 3 config: %v", err)
	}

	got, err := cloudflare.GetConfig(3)
	if err != nil {
		t.Fatalf("get user 3 config: %v", err)
	}
	if got.APIToken != "token-user-3" || got.Email != "three@example.com" {
		t.Fatalf("unexpected user 3 config: %#v", got)
	}

	if err := cloudflare.SaveConfig(3, CloudflareConfig{
		UserID:   3,
		APIToken: "token-user-3-updated",
		Email:    "updated@example.com",
	}); err != nil {
		t.Fatalf("update user 3 config: %v", err)
	}
	got, err = cloudflare.GetConfig(1)
	if err != nil {
		t.Fatalf("get user 1 config: %v", err)
	}
	if got.APIToken != "token-user-1" {
		t.Fatalf("user 1 config changed while updating user 3: %#v", got)
	}
}
