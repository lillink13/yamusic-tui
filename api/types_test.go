package api

import (
	"encoding/json"
	"testing"
)

// Yandex returns albums.labels.id as a number; make sure the struct decodes it
// instead of failing the whole response (regression for the My Wave / likes /
// playlists "no items" bug).
func TestAlbumDecodesNumericLabelId(t *testing.T) {
	data := []byte(`{"id":123,"title":"Album","labels":[{"id":456,"name":"Some Label"}]}`)

	var album Album
	if err := json.Unmarshal(data, &album); err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if len(album.Labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(album.Labels))
	}
	if album.Labels[0].Id != 456 || album.Labels[0].Name != "Some Label" {
		t.Fatalf("label decoded wrong: %+v", album.Labels[0])
	}
}
