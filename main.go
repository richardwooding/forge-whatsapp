// Command whatsapp is a forge tool for the WhatsApp Business Cloud API.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"github.com/richardwooding/forge/sdk/tool"
)

const defaultAPIVersion = "v24.0"

type Sender struct {
	PhoneNumberID string `json:"phone_number_id,omitempty" jsonschema:"business phone number ID to send from; defaults to the configured one"`
}

type TextArgs struct {
	Sender
	To         string `json:"to" jsonschema:"recipient phone number in international format, e.g. +27821234567"`
	Body       string `json:"body" jsonschema:"message text, up to 4096 characters"`
	PreviewURL bool   `json:"preview_url,omitempty" jsonschema:"render a preview for the first URL in the body"`
	ReplyTo    string `json:"reply_to,omitempty" jsonschema:"ID of a message to quote in reply"`
}

type TemplateArgs struct {
	Sender
	To         string           `json:"to" jsonschema:"recipient phone number in international format"`
	Name       string           `json:"name" jsonschema:"name of an approved message template"`
	Language   string           `json:"language,omitempty" jsonschema:"template language code; default en_US"`
	BodyParams []string         `json:"body_params,omitempty" jsonschema:"values for the body's {{1}}, {{2}}... placeholders, in order"`
	Components []map[string]any `json:"components,omitempty" jsonschema:"raw template components, for headers, buttons or typed parameters; excludes body_params"`
}

type MediaArgs struct {
	Sender
	To       string `json:"to" jsonschema:"recipient phone number in international format"`
	Kind     string `json:"kind" jsonschema:"image, video, document, audio or sticker"`
	Link     string `json:"link,omitempty" jsonschema:"public https URL of the media; give this or media_id"`
	MediaID  string `json:"media_id,omitempty" jsonschema:"ID of media already uploaded to WhatsApp; give this or link"`
	Caption  string `json:"caption,omitempty" jsonschema:"caption, for image, video and document"`
	Filename string `json:"filename,omitempty" jsonschema:"file name shown for a document"`
	ReplyTo  string `json:"reply_to,omitempty" jsonschema:"ID of a message to quote in reply"`
}

type LocationArgs struct {
	Sender
	To        string  `json:"to" jsonschema:"recipient phone number in international format"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty" jsonschema:"name of the place"`
	Address   string  `json:"address,omitempty" jsonschema:"address of the place"`
	ReplyTo   string  `json:"reply_to,omitempty" jsonschema:"ID of a message to quote in reply"`
}

type Choice struct {
	ID          string `json:"id" jsonschema:"identifier returned in the webhook when chosen"`
	Title       string `json:"title" jsonschema:"text shown; 20 characters for buttons, 24 for list rows"`
	Description string `json:"description,omitempty" jsonschema:"extra line under a list row; ignored for buttons"`
}

type ButtonsArgs struct {
	Sender
	To      string   `json:"to" jsonschema:"recipient phone number in international format"`
	Body    string   `json:"body" jsonschema:"message text above the buttons"`
	Buttons []Choice `json:"buttons" jsonschema:"one to three reply buttons"`
	Header  string   `json:"header,omitempty" jsonschema:"text header"`
	Footer  string   `json:"footer,omitempty" jsonschema:"footer text"`
	ReplyTo string   `json:"reply_to,omitempty" jsonschema:"ID of a message to quote in reply"`
}

type Section struct {
	Title string   `json:"title,omitempty" jsonschema:"section heading; required when there is more than one section"`
	Rows  []Choice `json:"rows"`
}

type ListArgs struct {
	Sender
	To       string    `json:"to" jsonschema:"recipient phone number in international format"`
	Body     string    `json:"body" jsonschema:"message text"`
	Button   string    `json:"button" jsonschema:"label of the button that opens the list"`
	Sections []Section `json:"sections" jsonschema:"up to ten sections, with at most ten rows in total"`
	Header   string    `json:"header,omitempty" jsonschema:"text header"`
	Footer   string    `json:"footer,omitempty" jsonschema:"footer text"`
	ReplyTo  string    `json:"reply_to,omitempty" jsonschema:"ID of a message to quote in reply"`
}

type ReactArgs struct {
	Sender
	To        string `json:"to" jsonschema:"phone number of the chat the message is in"`
	MessageID string `json:"message_id" jsonschema:"ID of the message to react to"`
	Emoji     string `json:"emoji,omitempty" jsonschema:"the reaction; empty removes an earlier one"`
}

type MarkReadArgs struct {
	Sender
	MessageID string `json:"message_id" jsonschema:"ID of the received message"`
	Typing    bool   `json:"typing,omitempty" jsonschema:"also show a typing indicator for up to 25 seconds"`
}

type SendOut struct {
	To        string `json:"to"`
	WaID      string `json:"wa_id,omitempty"`
	MessageID string `json:"message_id"`
	Status    string `json:"status,omitempty"`
}

type MarkReadOut struct {
	Success bool `json:"success"`
}

type ConfigureArgs struct {
	PhoneNumberID string `json:"phone_number_id,omitempty" jsonschema:"default business phone number ID to send from"`
	WABAID        string `json:"waba_id,omitempty" jsonschema:"default WhatsApp Business Account ID, for templates and phone numbers"`
	APIVersion    string `json:"api_version,omitempty" jsonschema:"Graph API version, e.g. v24.0"`
}

type config struct {
	PhoneNumberID string `json:"phone_number_id,omitempty"`
	WABAID        string `json:"waba_id,omitempty"`
	APIVersion    string `json:"api_version"`
}

type TemplatesArgs struct {
	WABAID string `json:"waba_id,omitempty" jsonschema:"WhatsApp Business Account ID; defaults to the configured one"`
	Name   string `json:"name,omitempty" jsonschema:"only templates whose name contains this"`
	Status string `json:"status,omitempty" jsonschema:"only templates in this status, e.g. APPROVED"`
	Limit  int    `json:"limit,omitempty" jsonschema:"page size; default 25"`
	After  string `json:"after,omitempty" jsonschema:"cursor from a previous page's next"`
}

type Template struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Status     string           `json:"status"`
	Language   string           `json:"language"`
	Category   string           `json:"category"`
	Components []map[string]any `json:"components,omitempty"`
}

type TemplatesOut struct {
	Templates []Template `json:"templates"`
	Next      string     `json:"next,omitempty"`
}

type PhoneNumbersArgs struct {
	WABAID string `json:"waba_id,omitempty" jsonschema:"WhatsApp Business Account ID; defaults to the configured one"`
}

type PhoneNumber struct {
	ID                     string `json:"id"`
	DisplayPhoneNumber     string `json:"display_phone_number"`
	VerifiedName           string `json:"verified_name"`
	QualityRating          string `json:"quality_rating,omitempty"`
	CodeVerificationStatus string `json:"code_verification_status,omitempty"`
	PlatformType           string `json:"platform_type,omitempty"`
	Throughput             *struct {
		Level string `json:"level"`
	} `json:"throughput,omitempty"`
}

type PhoneNumbersOut struct {
	PhoneNumbers []PhoneNumber `json:"phone_numbers"`
}

type ProfileArgs struct {
	Sender
}

type Profile struct {
	About             string   `json:"about,omitempty"`
	Address           string   `json:"address,omitempty"`
	Description       string   `json:"description,omitempty"`
	Email             string   `json:"email,omitempty"`
	ProfilePictureURL string   `json:"profile_picture_url,omitempty"`
	Websites          []string `json:"websites,omitempty"`
	Vertical          string   `json:"vertical,omitempty"`
}

type MediaInfoArgs struct {
	Sender
	MediaID string `json:"media_id" jsonschema:"ID of the media, from a webhook or an upload"`
}

type MediaInfo struct {
	ID       string      `json:"id"`
	URL      string      `json:"url"`
	MimeType string      `json:"mime_type"`
	SHA256   string      `json:"sha256"`
	FileSize json.Number `json:"file_size"`
}

var _ = tool.Register(
	tool.Spec{
		Name:    "whatsapp",
		Version: "0.1.0",
		Summary: "Send and manage messages through the WhatsApp Business Cloud API",
		UseWhen: "you need to send a WhatsApp message from a business number, or look up its templates, " +
			"phone numbers or profile; not for reading incoming messages, which arrive by webhook",
		Description: "Talks to Meta's WhatsApp Business Cloud API at graph.facebook.com. It sends text, " +
			"template, media, location, reply-button, list and reaction messages, marks messages read, " +
			"and lists templates and phone numbers. The access token is attached by forge and never " +
			"seen by the tool. Set defaults once with the configure operation.",
		Labels: []string{"whatsapp", "messaging", "meta"},
		Needs: []tool.Need{
			{
				Kind:  tool.NetHTTP,
				Scope: []string{graphHost},
				Reason: "call the WhatsApp Business Cloud API at graph.facebook.com, sending the messages " +
					"and recipients you ask it to",
			},
			{
				Kind:  tool.Secret,
				Scope: []string{tokenSecret},
				Reason: "authenticate to the WhatsApp API. forge attaches the token to requests itself; " +
					"bind it with --host graph.facebook.com and it can go nowhere else",
			},
			{
				Kind:   tool.KV,
				Scope:  []string{configNS},
				Reason: "remember your default phone number ID, business account ID and API version",
			},
		},
	},
	tool.Op("configure", configure,
		tool.Summary("Set the default phone number ID, business account ID and API version; with no arguments, show them"),
		tool.Idempotent()),
	tool.Op("send_text", sendText, tool.Summary("Send a text message"), tool.OpenWorld()),
	tool.Op("send_template", sendTemplate,
		tool.Summary("Send an approved template message; the only kind allowed outside the 24-hour window"),
		tool.OpenWorld()),
	tool.Op("send_media", sendMedia,
		tool.Summary("Send an image, video, document, audio or sticker by link or media ID"), tool.OpenWorld()),
	tool.Op("send_location", sendLocation, tool.Summary("Send a location pin"), tool.OpenWorld()),
	tool.Op("send_buttons", sendButtons, tool.Summary("Send a message with up to three reply buttons"), tool.OpenWorld()),
	tool.Op("send_list", sendList, tool.Summary("Send a message with a list of choices"), tool.OpenWorld()),
	tool.Op("react", react, tool.Summary("React to a message with an emoji, or remove a reaction"), tool.OpenWorld()),
	tool.Op("mark_read", markRead,
		tool.Summary("Mark a received message as read, optionally showing a typing indicator"),
		tool.Idempotent(), tool.OpenWorld()),
	tool.Op("templates", templates,
		tool.Summary("List the business account's message templates"), tool.ReadOnly(), tool.OpenWorld()),
	tool.Op("phone_numbers", phoneNumbers,
		tool.Summary("List the business account's phone numbers and their quality ratings"),
		tool.ReadOnly(), tool.OpenWorld()),
	tool.Op("profile", profile,
		tool.Summary("Show the business profile of a phone number"), tool.ReadOnly(), tool.OpenWorld()),
	tool.Op("media_info", mediaInfo,
		tool.Summary("Look up a media ID's download URL, type and size"), tool.ReadOnly(), tool.OpenWorld()),
)

func main() {}

func loadConfig() (config, error) {
	var c config
	if _, err := tool.OpenKV(configNS).GetJSON("defaults", &c); err != nil {
		return config{}, explainDenial(err)
	}
	if c.APIVersion == "" {
		c.APIVersion = defaultAPIVersion
	}
	return c, nil
}

func configure(ctx *tool.Context, a ConfigureArgs) (config, error) {
	c, err := loadConfig()
	if err != nil {
		return config{}, err
	}
	if a == (ConfigureArgs{}) {
		return c, nil
	}
	if a.PhoneNumberID != "" {
		c.PhoneNumberID = a.PhoneNumberID
	}
	if a.WABAID != "" {
		c.WABAID = a.WABAID
	}
	if a.APIVersion != "" {
		if !validVersion(a.APIVersion) {
			return config{}, fmt.Errorf("%q is not a Graph API version; use the form v24.0", a.APIVersion)
		}
		c.APIVersion = a.APIVersion
	}
	if err := tool.OpenKV(configNS).SetJSON("defaults", c); err != nil {
		return config{}, explainDenial(err)
	}
	return c, nil
}

func validVersion(v string) bool {
	if len(v) < 4 || v[0] != 'v' {
		return false
	}
	_, err := strconv.ParseFloat(v[1:], 64)
	return err == nil
}

func (s Sender) resolve() (config, string, error) {
	c, err := loadConfig()
	if err != nil {
		return config{}, "", err
	}
	id := s.PhoneNumberID
	if id == "" {
		id = c.PhoneNumberID
	}
	if id == "" {
		return config{}, "", errors.New("no phone number ID: pass phone_number_id, or set a default with " +
			"the configure operation's phone_number_id (find it with phone_numbers, or in Meta's WhatsApp Manager)")
	}
	return c, id, nil
}

func resolveWABA(override string) (config, string, error) {
	c, err := loadConfig()
	if err != nil {
		return config{}, "", err
	}
	id := override
	if id == "" {
		id = c.WABAID
	}
	if id == "" {
		return config{}, "", errors.New("no business account ID: pass waba_id, or set a default with the configure operation's waba_id")
	}
	return c, id, nil
}

func send(s Sender, build func() (message, error)) (SendOut, error) {
	msg, err := build()
	if err != nil {
		return SendOut{}, err
	}
	cfg, id, err := s.resolve()
	if err != nil {
		return SendOut{}, err
	}
	var res struct {
		Contacts []struct {
			Input string `json:"input"`
			WaID  string `json:"wa_id"`
		} `json:"contacts"`
		Messages []struct {
			ID     string `json:"id"`
			Status string `json:"message_status"`
		} `json:"messages"`
	}
	if err := graphPost(cfg, url.PathEscape(id)+"/messages", msg, &res); err != nil {
		return SendOut{}, err
	}
	out := SendOut{To: fmt.Sprint(msg["to"])}
	if len(res.Contacts) > 0 {
		out.WaID = res.Contacts[0].WaID
	}
	if len(res.Messages) > 0 {
		out.MessageID, out.Status = res.Messages[0].ID, res.Messages[0].Status
	}
	return out, nil
}

func sendText(ctx *tool.Context, a TextArgs) (SendOut, error) {
	return send(a.Sender, func() (message, error) { return textMessage(a) })
}

func sendTemplate(ctx *tool.Context, a TemplateArgs) (SendOut, error) {
	return send(a.Sender, func() (message, error) { return templateMessage(a) })
}

func sendMedia(ctx *tool.Context, a MediaArgs) (SendOut, error) {
	return send(a.Sender, func() (message, error) { return mediaMessage(a) })
}

func sendLocation(ctx *tool.Context, a LocationArgs) (SendOut, error) {
	return send(a.Sender, func() (message, error) { return locationMessage(a) })
}

func sendButtons(ctx *tool.Context, a ButtonsArgs) (SendOut, error) {
	return send(a.Sender, func() (message, error) { return buttonsMessage(a) })
}

func sendList(ctx *tool.Context, a ListArgs) (SendOut, error) {
	return send(a.Sender, func() (message, error) { return listMessage(a) })
}

func react(ctx *tool.Context, a ReactArgs) (SendOut, error) {
	return send(a.Sender, func() (message, error) { return reactionMessage(a) })
}

func markRead(ctx *tool.Context, a MarkReadArgs) (MarkReadOut, error) {
	msg, err := readReceipt(a)
	if err != nil {
		return MarkReadOut{}, err
	}
	cfg, id, err := a.Sender.resolve()
	if err != nil {
		return MarkReadOut{}, err
	}
	var out MarkReadOut
	if err := graphPost(cfg, url.PathEscape(id)+"/messages", msg, &out); err != nil {
		return MarkReadOut{}, err
	}
	return out, nil
}

func templates(ctx *tool.Context, a TemplatesArgs) (TemplatesOut, error) {
	cfg, waba, err := resolveWABA(a.WABAID)
	if err != nil {
		return TemplatesOut{}, err
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 25
	}
	q := url.Values{
		"fields": {"id,name,status,language,category,components"},
		"limit":  {strconv.Itoa(limit)},
	}
	if a.Name != "" {
		q.Set("name", a.Name)
	}
	if a.Status != "" {
		q.Set("status", a.Status)
	}
	if a.After != "" {
		q.Set("after", a.After)
	}
	var res struct {
		Data   []Template `json:"data"`
		Paging struct {
			Cursors struct {
				After string `json:"after"`
			} `json:"cursors"`
			Next string `json:"next"`
		} `json:"paging"`
	}
	if err := graphGet(cfg, url.PathEscape(waba)+"/message_templates", q, &res); err != nil {
		return TemplatesOut{}, err
	}
	out := TemplatesOut{Templates: res.Data}
	if out.Templates == nil {
		out.Templates = []Template{}
	}
	if res.Paging.Next != "" {
		out.Next = res.Paging.Cursors.After
	}
	return out, nil
}

func phoneNumbers(ctx *tool.Context, a PhoneNumbersArgs) (PhoneNumbersOut, error) {
	cfg, waba, err := resolveWABA(a.WABAID)
	if err != nil {
		return PhoneNumbersOut{}, err
	}
	q := url.Values{"fields": {"id,display_phone_number,verified_name,quality_rating," +
		"code_verification_status,platform_type,throughput"}}
	var res struct {
		Data []PhoneNumber `json:"data"`
	}
	if err := graphGet(cfg, url.PathEscape(waba)+"/phone_numbers", q, &res); err != nil {
		return PhoneNumbersOut{}, err
	}
	out := PhoneNumbersOut{PhoneNumbers: res.Data}
	if out.PhoneNumbers == nil {
		out.PhoneNumbers = []PhoneNumber{}
	}
	return out, nil
}

func profile(ctx *tool.Context, a ProfileArgs) (Profile, error) {
	cfg, id, err := a.Sender.resolve()
	if err != nil {
		return Profile{}, err
	}
	q := url.Values{"fields": {"about,address,description,email,profile_picture_url,websites,vertical"}}
	var res struct {
		Data []Profile `json:"data"`
	}
	if err := graphGet(cfg, url.PathEscape(id)+"/whatsapp_business_profile", q, &res); err != nil {
		return Profile{}, err
	}
	if len(res.Data) == 0 {
		return Profile{}, errors.New("the WhatsApp API returned no business profile for " + id)
	}
	return res.Data[0], nil
}

func mediaInfo(ctx *tool.Context, a MediaInfoArgs) (MediaInfo, error) {
	if a.MediaID == "" {
		return MediaInfo{}, errors.New("give the media_id to look up")
	}
	cfg, err := loadConfig()
	if err != nil {
		return MediaInfo{}, err
	}
	var q url.Values
	if id := a.PhoneNumberID; id != "" {
		q = url.Values{"phone_number_id": {id}}
	} else if cfg.PhoneNumberID != "" {
		q = url.Values{"phone_number_id": {cfg.PhoneNumberID}}
	}
	var out MediaInfo
	if err := graphGet(cfg, url.PathEscape(a.MediaID), q, &out); err != nil {
		return MediaInfo{}, err
	}
	return out, nil
}
