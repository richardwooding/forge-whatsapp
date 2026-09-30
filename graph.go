package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/richardwooding/forge/sdk/tool"
)

const (
	graphHost   = "graph.facebook.com"
	tokenSecret = "whatsapp-token"
	configNS    = "config"
)

type graphError struct {
	Message   string `json:"message"`
	Type      string `json:"type"`
	Code      int    `json:"code"`
	Subcode   int    `json:"error_subcode"`
	FBTraceID string `json:"fbtrace_id"`
	Data      struct {
		Details string `json:"details"`
	} `json:"error_data"`
}

func (e *graphError) Error() string {
	msg := fmt.Sprintf("WhatsApp API error %d: %s", e.Code, e.Message)
	if e.Data.Details != "" && e.Data.Details != e.Message {
		msg += " (" + e.Data.Details + ")"
	}
	if hint := hintFor(e.Code); hint != "" {
		msg += "\n\n" + hint
	}
	if e.FBTraceID != "" {
		msg += "\n\nfbtrace_id: " + e.FBTraceID
	}
	return msg
}

func hintFor(code int) string {
	switch code {
	case 190:
		return "The access token is invalid or has expired. Store a new one with:\n" +
			"    forge secret set " + tokenSecret + " --host " + graphHost
	case 10, 200:
		return "The token lacks a permission this call needs; a system-user token needs " +
			"whatsapp_business_messaging and whatsapp_business_management."
	case 131047:
		return "More than 24 hours have passed since the recipient last messaged you. " +
			"Only a template message can be sent now: use send_template."
	case 131026:
		return "The message could not be delivered; the number may not be on WhatsApp, " +
			"or the recipient has not accepted the latest terms."
	case 132000, 132001:
		return "The template or its parameters do not match an approved template; " +
			"check the name, language and parameter count with the templates operation."
	case 130429, 131056, 80007:
		return "Rate limited by Meta; slow down and retry."
	case 133010:
		return "The phone number is not registered with Cloud API."
	}
	return ""
}

func graphURL(cfg config, path string, query url.Values) string {
	u := "https://" + graphHost + "/" + cfg.APIVersion + "/" + strings.TrimPrefix(path, "/")
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func graphGet(cfg config, path string, query url.Values, out any) error {
	return graphDo(tool.HTTPRequest{Method: "GET", URL: graphURL(cfg, path, query)}, out)
}

func graphPost(cfg config, path string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("cannot encode the request: %w", err)
	}
	return graphDo(tool.HTTPRequest{
		Method:  "POST",
		URL:     graphURL(cfg, path, nil),
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    raw,
	}, out)
}

func graphDo(req tool.HTTPRequest, out any) error {
	req.Credential = tool.Bearer(tokenSecret)
	res, err := tool.HTTP(req)
	if err != nil {
		return explainDenial(err)
	}
	return decodeGraph(res, out)
}

func decodeGraph(res tool.HTTPResponse, out any) error {
	if res.Truncated {
		return errors.New("the WhatsApp API response was cut at forge's size limit")
	}
	if !res.OK() {
		var wrapped struct {
			Error *graphError `json:"error"`
		}
		if json.Unmarshal(res.Body, &wrapped) == nil && wrapped.Error != nil {
			return wrapped.Error
		}
		if res.Status == 401 {
			return fmt.Errorf("the WhatsApp API answered 401 with no detail; if this forge predates SDK v0.4.0 it "+
				"sent the request without the token: %s", strings.TrimSpace(string(res.Body)))
		}
		return fmt.Errorf("the WhatsApp API answered %d: %s", res.Status, strings.TrimSpace(string(res.Body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(res.Body, out); err != nil {
		return fmt.Errorf("the WhatsApp API returned a response this tool could not read: %w", err)
	}
	return nil
}

func explainDenial(err error) error {
	d, ok := tool.Denied(err)
	if !ok {
		return err
	}
	switch d.Code {
	case tool.DenyNoGrant, tool.DenyOutOfScope:
		return fmt.Errorf("%s\n\nwhatsapp needs net.http(%s) and secret(%s), and the secret must be bound to %s:\n"+
			"    forge secret set %s --host %s\n"+
			"    forge grant allow whatsapp",
			d.Reason, graphHost, tokenSecret, graphHost, tokenSecret, graphHost)
	case tool.DenyBudget:
		return fmt.Errorf("%s\n\nThis is forge's per-request allowance; split the work across calls", d.Reason)
	}
	return errors.New(d.Reason)
}
