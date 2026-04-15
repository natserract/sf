package api

import (
	"encoding/json"
	"testing"
)

func TestCountResponse_Unmarshal(t *testing.T) {
	raw := `{"totalCount":19602287,"serviceMessageID":"abc"}`
	var out CountResponse
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("unmarshal count response: %v", err)
	}
	if out.TotalCount != 19602287 {
		t.Fatalf("expected totalCount 19602287, got %d", out.TotalCount)
	}
}
