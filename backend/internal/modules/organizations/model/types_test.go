package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMemberJSONHidesInternalIDs(t *testing.T) {
	raw, err := json.Marshal(Member{ID: 41, User: User{ID: 42, Email: "a@b.test"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{`"ID"`, `"id"`, "41", "42"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("member JSON exposes %s: %s", leak, raw)
		}
	}
}
