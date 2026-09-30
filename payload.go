package main

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

type message map[string]any

func normalizeRecipient(to string) (string, error) {
	var b strings.Builder
	for _, r := range strings.TrimSpace(to) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && b.Len() == 0, r == ' ', r == '-', r == '(', r == ')', r == '.':
		default:
			return "", fmt.Errorf("%q is not a phone number; give it in international format, e.g. +27821234567", to)
		}
	}
	n := b.String()
	if len(n) < 7 || len(n) > 15 {
		return "", fmt.Errorf("%q is not a phone number in international format (7 to 15 digits with country code)", to)
	}
	return n, nil
}

func envelope(to, kind, replyTo string) (message, error) {
	n, err := normalizeRecipient(to)
	if err != nil {
		return nil, err
	}
	m := message{
		"messaging_product": "whatsapp",
		"recipient_type":    "individual",
		"to":                n,
		"type":              kind,
	}
	if replyTo != "" {
		m["context"] = map[string]string{"message_id": replyTo}
	}
	return m, nil
}

func textMessage(a TextArgs) (message, error) {
	if strings.TrimSpace(a.Body) == "" {
		return nil, errors.New("the message body is empty")
	}
	if n := utf8.RuneCountInString(a.Body); n > 4096 {
		return nil, fmt.Errorf("the message body is %d characters; WhatsApp allows 4096", n)
	}
	m, err := envelope(a.To, "text", a.ReplyTo)
	if err != nil {
		return nil, err
	}
	m["text"] = map[string]any{"body": a.Body, "preview_url": a.PreviewURL}
	return m, nil
}

func templateMessage(a TemplateArgs) (message, error) {
	if a.Name == "" {
		return nil, errors.New("the template name is empty")
	}
	if len(a.BodyParams) > 0 && len(a.Components) > 0 {
		return nil, errors.New("give body_params or components, not both")
	}
	lang := a.Language
	if lang == "" {
		lang = "en_US"
	}
	m, err := envelope(a.To, "template", "")
	if err != nil {
		return nil, err
	}
	tpl := map[string]any{"name": a.Name, "language": map[string]string{"code": lang}}
	switch {
	case len(a.Components) > 0:
		tpl["components"] = a.Components
	case len(a.BodyParams) > 0:
		params := make([]map[string]string, len(a.BodyParams))
		for i, p := range a.BodyParams {
			params[i] = map[string]string{"type": "text", "text": p}
		}
		tpl["components"] = []map[string]any{{"type": "body", "parameters": params}}
	}
	m["template"] = tpl
	return m, nil
}

var mediaKinds = map[string]struct{ caption, filename bool }{
	"image":    {caption: true},
	"video":    {caption: true},
	"document": {caption: true, filename: true},
	"audio":    {},
	"sticker":  {},
}

func mediaMessage(a MediaArgs) (message, error) {
	kind, ok := mediaKinds[a.Kind]
	if !ok {
		return nil, fmt.Errorf("%q is not a media kind; use image, video, document, audio or sticker", a.Kind)
	}
	if (a.Link == "") == (a.MediaID == "") {
		return nil, errors.New("give exactly one of link or media_id")
	}
	if a.Link != "" && !strings.HasPrefix(a.Link, "https://") {
		return nil, errors.New("the media link must be an https URL WhatsApp can fetch")
	}
	if a.Caption != "" && !kind.caption {
		return nil, fmt.Errorf("%s messages cannot carry a caption", a.Kind)
	}
	if a.Filename != "" && !kind.filename {
		return nil, errors.New("only document messages carry a filename")
	}
	m, err := envelope(a.To, a.Kind, a.ReplyTo)
	if err != nil {
		return nil, err
	}
	media := map[string]string{}
	if a.Link != "" {
		media["link"] = a.Link
	} else {
		media["id"] = a.MediaID
	}
	if a.Caption != "" {
		media["caption"] = a.Caption
	}
	if a.Filename != "" {
		media["filename"] = a.Filename
	}
	m[a.Kind] = media
	return m, nil
}

func locationMessage(a LocationArgs) (message, error) {
	if a.Latitude < -90 || a.Latitude > 90 || a.Longitude < -180 || a.Longitude > 180 {
		return nil, fmt.Errorf("%v,%v is not a valid latitude,longitude", a.Latitude, a.Longitude)
	}
	m, err := envelope(a.To, "location", a.ReplyTo)
	if err != nil {
		return nil, err
	}
	loc := map[string]any{"latitude": a.Latitude, "longitude": a.Longitude}
	if a.Name != "" {
		loc["name"] = a.Name
	}
	if a.Address != "" {
		loc["address"] = a.Address
	}
	m["location"] = loc
	return m, nil
}

func interactiveFrame(body, header, footer string) (map[string]any, error) {
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("the message body is empty")
	}
	in := map[string]any{"body": map[string]string{"text": body}}
	if header != "" {
		in["header"] = map[string]string{"type": "text", "text": header}
	}
	if footer != "" {
		in["footer"] = map[string]string{"text": footer}
	}
	return in, nil
}

func buttonsMessage(a ButtonsArgs) (message, error) {
	if len(a.Buttons) == 0 || len(a.Buttons) > 3 {
		return nil, fmt.Errorf("a reply-button message needs 1 to 3 buttons, got %d", len(a.Buttons))
	}
	in, err := interactiveFrame(a.Body, a.Header, a.Footer)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	buttons := make([]map[string]any, len(a.Buttons))
	for i, b := range a.Buttons {
		if err := checkChoice(b.ID, b.Title, 20, seen); err != nil {
			return nil, err
		}
		buttons[i] = map[string]any{"type": "reply", "reply": map[string]string{"id": b.ID, "title": b.Title}}
	}
	in["type"] = "button"
	in["action"] = map[string]any{"buttons": buttons}

	m, err := envelope(a.To, "interactive", a.ReplyTo)
	if err != nil {
		return nil, err
	}
	m["interactive"] = in
	return m, nil
}

func listMessage(a ListArgs) (message, error) {
	if a.Button == "" {
		return nil, errors.New("a list message needs button text to open it")
	}
	if len(a.Sections) == 0 || len(a.Sections) > 10 {
		return nil, fmt.Errorf("a list message needs 1 to 10 sections, got %d", len(a.Sections))
	}
	in, err := interactiveFrame(a.Body, a.Header, a.Footer)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	rows := 0
	sections := make([]map[string]any, len(a.Sections))
	for i, s := range a.Sections {
		if len(a.Sections) > 1 && s.Title == "" {
			return nil, errors.New("every section needs a title when there is more than one")
		}
		out := make([]map[string]string, len(s.Rows))
		for j, r := range s.Rows {
			if err := checkChoice(r.ID, r.Title, 24, seen); err != nil {
				return nil, err
			}
			out[j] = map[string]string{"id": r.ID, "title": r.Title}
			if r.Description != "" {
				out[j]["description"] = r.Description
			}
		}
		rows += len(s.Rows)
		sec := map[string]any{"rows": out}
		if s.Title != "" {
			sec["title"] = s.Title
		}
		sections[i] = sec
	}
	if rows == 0 || rows > 10 {
		return nil, fmt.Errorf("a list message needs 1 to 10 rows in total, got %d", rows)
	}
	in["type"] = "list"
	in["action"] = map[string]any{"button": a.Button, "sections": sections}

	m, err := envelope(a.To, "interactive", a.ReplyTo)
	if err != nil {
		return nil, err
	}
	m["interactive"] = in
	return m, nil
}

func checkChoice(id, title string, maxTitle int, seen map[string]bool) error {
	if id == "" || title == "" {
		return errors.New("every choice needs an id and a title")
	}
	if seen[id] {
		return fmt.Errorf("the choice id %q is used twice", id)
	}
	seen[id] = true
	if n := utf8.RuneCountInString(title); n > maxTitle {
		return fmt.Errorf("the title %q is %d characters; WhatsApp allows %d", title, n, maxTitle)
	}
	return nil
}

func reactionMessage(a ReactArgs) (message, error) {
	if a.MessageID == "" {
		return nil, errors.New("give the id of the message to react to")
	}
	m, err := envelope(a.To, "reaction", "")
	if err != nil {
		return nil, err
	}
	m["reaction"] = map[string]string{"message_id": a.MessageID, "emoji": a.Emoji}
	return m, nil
}

func readReceipt(a MarkReadArgs) (message, error) {
	if a.MessageID == "" {
		return nil, errors.New("give the id of the message to mark as read")
	}
	m := message{"messaging_product": "whatsapp", "status": "read", "message_id": a.MessageID}
	if a.Typing {
		m["typing_indicator"] = map[string]string{"type": "text"}
	}
	return m, nil
}
