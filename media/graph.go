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
	graphHost     = "graph.facebook.com"
	lookasideHost = "lookaside.fbsbx.com"
	tokenSecret   = "whatsapp-token"
)

type graphError struct {
	Message   string `json:"message"`
	Code      int    `json:"code"`
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
			"    forge secret set " + tokenSecret + " --host " + graphHost + " --host " + lookasideHost
	case 10, 200:
		return "The token lacks a permission this call needs; a system-user token needs " +
			"whatsapp_business_messaging and whatsapp_business_management."
	case 131052, 131053:
		return "WhatsApp could not process the media; check its type and size against " +
			"the limits for its kind."
	case 100:
		return "The media ID may have expired: uploaded media lasts 30 days, and a received " +
			"media URL only 5 minutes, so fetch it again rather than reusing one."
	}
	return ""
}

func graphURL(version, path string, query url.Values) string {
	u := "https://" + graphHost + "/" + version + "/" + strings.TrimPrefix(path, "/")
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func do(req tool.HTTPRequest) (tool.HTTPResponse, error) {
	req.Credential = tool.Bearer(tokenSecret)
	res, err := tool.HTTP(req)
	if err != nil {
		return tool.HTTPResponse{}, explainDenial(err)
	}
	if res.Truncated {
		return tool.HTTPResponse{}, errors.New("the response was larger than forge's 16 MiB limit")
	}
	if !res.OK() {
		return tool.HTTPResponse{}, statusError(res)
	}
	return res, nil
}

func graphJSON(req tool.HTTPRequest, out any) error {
	res, err := do(req)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(res.Body, out); err != nil {
		return fmt.Errorf("the WhatsApp API returned a response this tool could not read: %w", err)
	}
	return nil
}

func statusError(res tool.HTTPResponse) error {
	var wrapped struct {
		Error *graphError `json:"error"`
	}
	if json.Unmarshal(res.Body, &wrapped) == nil && wrapped.Error != nil {
		return wrapped.Error
	}
	body := strings.TrimSpace(string(res.Body))
	if len(body) > 512 {
		body = body[:512] + "…"
	}
	return fmt.Errorf("the WhatsApp API answered %d: %s", res.Status, body)
}

func explainDenial(err error) error {
	d, ok := tool.Denied(err)
	if !ok {
		return err
	}
	switch d.Code {
	case tool.DenyNoGrant, tool.DenyOutOfScope:
		return fmt.Errorf("%s\n\nwhatsapp-media needs net.http(%s, %s), secret(%s) bound to both hosts, "+
			"and the directories it reads and writes:\n"+
			"    forge secret bind %s %s %s\n"+
			"    forge grant allow whatsapp-media --scope fs.read=/path/to/outbox --scope fs.write=/path/to/inbox",
			d.Reason, graphHost, lookasideHost, tokenSecret, tokenSecret, graphHost, lookasideHost)
	case tool.DenyBudget:
		return fmt.Errorf("%s\n\nThis is forge's per-request allowance; the file is probably too large", d.Reason)
	}
	return errors.New(d.Reason)
}
