package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/richardwooding/forge/sdk/tool"
)

func asJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNormalizeRecipient(t *testing.T) {
	for in, want := range map[string]string{
		"+27 82 123 4567":   "27821234567",
		"27821234567":       "27821234567",
		"+1 (415) 555-0100": "14155550100",
		" +44.20.7946.0958": "442079460958",
	} {
		got, err := normalizeRecipient(in)
		if err != nil || got != want {
			t.Errorf("normalizeRecipient(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "12345", "+27 82 abc", "27+821234567", "1234567890123456", "whatsapp:+2782"} {
		if got, err := normalizeRecipient(in); err == nil {
			t.Errorf("normalizeRecipient(%q) = %q; want an error", in, got)
		}
	}
}

func TestTextMessage(t *testing.T) {
	m, err := textMessage(TextArgs{To: "+27821234567", Body: "hi", PreviewURL: true, ReplyTo: "wamid.1"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"context":{"message_id":"wamid.1"},"messaging_product":"whatsapp","recipient_type":"individual",` +
		`"text":{"body":"hi","preview_url":true},"to":"27821234567","type":"text"}`
	if got := asJSON(t, m); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	if _, err := textMessage(TextArgs{To: "+27821234567", Body: "  "}); err == nil {
		t.Error("an empty body was accepted")
	}
	if _, err := textMessage(TextArgs{To: "+27821234567", Body: strings.Repeat("é", 4097)}); err == nil {
		t.Error("a body over 4096 characters was accepted")
	}
}

func TestTemplateMessage(t *testing.T) {
	m, err := templateMessage(TemplateArgs{To: "27821234567", Name: "order_ready", BodyParams: []string{"Ann", "42"}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"name":"order_ready","language":{"code":"en_US"},"components":[{"parameters":` +
		`[{"text":"Ann","type":"text"},{"text":"42","type":"text"}],"type":"body"}]}`
	var got map[string]any
	if err := json.Unmarshal([]byte(asJSON(t, m["template"])), &got); err != nil {
		t.Fatal(err)
	}
	var wantMap map[string]any
	_ = json.Unmarshal([]byte(want), &wantMap)
	if asJSON(t, got) != asJSON(t, wantMap) {
		t.Errorf("got  %s\nwant %s", asJSON(t, got), asJSON(t, wantMap))
	}

	raw := []map[string]any{{"type": "header", "parameters": []any{}}}
	m, err = templateMessage(TemplateArgs{To: "27821234567", Name: "x", Language: "af", Components: raw})
	if err != nil {
		t.Fatal(err)
	}
	if s := asJSON(t, m["template"]); !strings.Contains(s, `"code":"af"`) || !strings.Contains(s, `"type":"header"`) {
		t.Errorf("raw components or language lost: %s", s)
	}

	if _, err := templateMessage(TemplateArgs{To: "27821234567", Name: "x", BodyParams: []string{"a"}, Components: raw}); err == nil {
		t.Error("body_params and components together were accepted")
	}
	if _, err := templateMessage(TemplateArgs{To: "27821234567"}); err == nil {
		t.Error("a template with no name was accepted")
	}
}

func TestMediaMessage(t *testing.T) {
	m, err := mediaMessage(MediaArgs{To: "27821234567", Kind: "document", Link: "https://x.test/a.pdf", Caption: "c", Filename: "a.pdf"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := asJSON(t, m["document"]), `{"caption":"c","filename":"a.pdf","link":"https://x.test/a.pdf"}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	m, err = mediaMessage(MediaArgs{To: "27821234567", Kind: "sticker", MediaID: "123"})
	if err != nil {
		t.Fatal(err)
	}
	if got := asJSON(t, m["sticker"]); got != `{"id":"123"}` {
		t.Errorf("got %s", got)
	}

	for name, a := range map[string]MediaArgs{
		"unknown kind":        {To: "27821234567", Kind: "gif", Link: "https://x.test/a"},
		"neither source":      {To: "27821234567", Kind: "image"},
		"both sources":        {To: "27821234567", Kind: "image", Link: "https://x.test/a", MediaID: "1"},
		"plain http":          {To: "27821234567", Kind: "image", Link: "http://x.test/a"},
		"caption on audio":    {To: "27821234567", Kind: "audio", MediaID: "1", Caption: "c"},
		"filename on a video": {To: "27821234567", Kind: "video", MediaID: "1", Filename: "v.mp4"},
	} {
		if _, err := mediaMessage(a); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestLocationMessage(t *testing.T) {
	m, err := locationMessage(LocationArgs{To: "27821234567", Latitude: -33.9249, Longitude: 18.4241, Name: "Cape Town"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := asJSON(t, m["location"]), `{"latitude":-33.9249,"longitude":18.4241,"name":"Cape Town"}`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := locationMessage(LocationArgs{To: "27821234567", Latitude: 91}); err == nil {
		t.Error("latitude 91 was accepted")
	}
}

func TestButtonsMessage(t *testing.T) {
	m, err := buttonsMessage(ButtonsArgs{
		To: "27821234567", Body: "Confirm?", Footer: "f",
		Buttons: []Choice{{ID: "yes", Title: "Yes"}, {ID: "no", Title: "No"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"action":{"buttons":[{"reply":{"id":"yes","title":"Yes"},"type":"reply"},` +
		`{"reply":{"id":"no","title":"No"},"type":"reply"}]},"body":{"text":"Confirm?"},"footer":{"text":"f"},"type":"button"}`
	if got := asJSON(t, m["interactive"]); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	four := []Choice{{ID: "a", Title: "a"}, {ID: "b", Title: "b"}, {ID: "c", Title: "c"}, {ID: "d", Title: "d"}}
	for name, a := range map[string]ButtonsArgs{
		"no buttons":   {To: "27821234567", Body: "x"},
		"four buttons": {To: "27821234567", Body: "x", Buttons: four},
		"long title":   {To: "27821234567", Body: "x", Buttons: []Choice{{ID: "a", Title: strings.Repeat("t", 21)}}},
		"duplicate id": {To: "27821234567", Body: "x", Buttons: []Choice{{ID: "a", Title: "1"}, {ID: "a", Title: "2"}}},
		"no body":      {To: "27821234567", Buttons: []Choice{{ID: "a", Title: "1"}}},
	} {
		if _, err := buttonsMessage(a); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestListMessage(t *testing.T) {
	m, err := listMessage(ListArgs{
		To: "27821234567", Body: "Pick", Button: "Options",
		Sections: []Section{{Rows: []Choice{{ID: "1", Title: "One", Description: "first"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"action":{"button":"Options","sections":[{"rows":[{"description":"first","id":"1","title":"One"}]}]},` +
		`"body":{"text":"Pick"},"type":"list"}`
	if got := asJSON(t, m["interactive"]); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}

	eleven := make([]Choice, 11)
	for i := range eleven {
		eleven[i] = Choice{ID: string(rune('a' + i)), Title: "r"}
	}
	for name, a := range map[string]ListArgs{
		"no button text":   {To: "27821234567", Body: "x", Sections: []Section{{Rows: eleven[:1]}}},
		"no sections":      {To: "27821234567", Body: "x", Button: "b"},
		"too many rows":    {To: "27821234567", Body: "x", Button: "b", Sections: []Section{{Rows: eleven}}},
		"untitled of many": {To: "27821234567", Body: "x", Button: "b", Sections: []Section{{Rows: eleven[:1]}, {Title: "t", Rows: eleven[1:2]}}},
		"empty sections":   {To: "27821234567", Body: "x", Button: "b", Sections: []Section{{}}},
	} {
		if _, err := listMessage(a); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestReactionAndReadReceipt(t *testing.T) {
	m, err := reactionMessage(ReactArgs{To: "27821234567", MessageID: "wamid.1", Emoji: "👍"})
	if err != nil {
		t.Fatal(err)
	}
	if got := asJSON(t, m["reaction"]); got != `{"emoji":"👍","message_id":"wamid.1"}` {
		t.Errorf("got %s", got)
	}
	if _, err := reactionMessage(ReactArgs{To: "27821234567"}); err == nil {
		t.Error("a reaction without a message id was accepted")
	}

	r, err := readReceipt(MarkReadArgs{MessageID: "wamid.1", Typing: true})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"message_id":"wamid.1","messaging_product":"whatsapp","status":"read","typing_indicator":{"type":"text"}}`
	if got := asJSON(t, r); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestDecodeGraphError(t *testing.T) {
	body := `{"error":{"message":"Re-engagement message","type":"OAuthException","code":131047,` +
		`"error_data":{"details":"More than 24 hours have passed"},"fbtrace_id":"Abc"}}`
	err := decodeGraph(tool.HTTPResponse{Status: 400, Body: []byte(body)}, nil)
	var ge *graphError
	if !errors.As(err, &ge) || ge.Code != 131047 {
		t.Fatalf("got %v, want a graphError with code 131047", err)
	}
	for _, want := range []string{"131047", "More than 24 hours", "send_template", "fbtrace_id: Abc"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	err = decodeGraph(tool.HTTPResponse{Status: 401, Body: []byte("nope")}, nil)
	if err == nil || !strings.Contains(err.Error(), "SDK v0.4.0") {
		t.Errorf("a bare 401 should point at an old forge: %v", err)
	}

	if err := decodeGraph(tool.HTTPResponse{Status: 200, Truncated: true}, nil); err == nil {
		t.Error("a truncated body was accepted")
	}

	var out MarkReadOut
	if err := decodeGraph(tool.HTTPResponse{Status: 200, Body: []byte(`{"success":true}`)}, &out); err != nil || !out.Success {
		t.Errorf("got %+v, %v", out, err)
	}
}

func TestGraphURL(t *testing.T) {
	got := graphURL(config{APIVersion: "v24.0"}, "123/message_templates", map[string][]string{"limit": {"5"}})
	if want := "https://graph.facebook.com/v24.0/123/message_templates?limit=5"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestValidVersion(t *testing.T) {
	for v, want := range map[string]bool{"v24.0": true, "v25.1": true, "24.0": false, "v": false, "vx.0": false, "": false} {
		if got := validVersion(v); got != want {
			t.Errorf("validVersion(%q) = %v, want %v", v, got, want)
		}
	}
}
