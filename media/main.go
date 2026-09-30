// Command whatsapp-media is the forge tool that moves files to and from the
// WhatsApp Business Cloud API. It is separate from whatsapp because forge
// grants capabilities per tool: filesystem access here would otherwise be
// asked of every send.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/richardwooding/forge/sdk/tool"
)

const (
	defaultAPIVersion = "v24.0"
	// The upload crosses into forge base64-encoded, inside forge's 64 MiB
	// per-call allowance.
	maxUploadBytes = 40 << 20
	companionTool  = "whatsapp"
	companionNS    = "config"
)

type UploadArgs struct {
	Path          string `json:"path" jsonschema:"absolute path of the file to upload"`
	MimeType      string `json:"mime_type,omitempty" jsonschema:"override the type inferred from the file extension"`
	PhoneNumberID string `json:"phone_number_id,omitempty" jsonschema:"business phone number ID; defaults to the one configured in the whatsapp tool"`
}

type UploadOut struct {
	MediaID  string `json:"media_id"`
	Kind     string `json:"kind"`
	MimeType string `json:"mime_type"`
	Bytes    int64  `json:"bytes"`
}

type DownloadArgs struct {
	MediaID       string `json:"media_id" jsonschema:"ID of the media, from an incoming message's webhook"`
	Path          string `json:"path" jsonschema:"absolute path to save to; a directory saves as <media_id><ext>"`
	Overwrite     bool   `json:"overwrite,omitempty" jsonschema:"replace an existing file"`
	PhoneNumberID string `json:"phone_number_id,omitempty" jsonschema:"business phone number ID the media belongs to"`
}

type DownloadOut struct {
	Path     string `json:"path"`
	MimeType string `json:"mime_type"`
	Bytes    int    `json:"bytes"`
	SHA256   string `json:"sha256"`
}

type DeleteArgs struct {
	MediaID       string `json:"media_id" jsonschema:"ID of uploaded media to delete"`
	PhoneNumberID string `json:"phone_number_id,omitempty" jsonschema:"business phone number ID the media belongs to"`
}

type DeleteOut struct {
	Success bool `json:"success"`
}

type config struct {
	PhoneNumberID string `json:"phone_number_id,omitempty"`
	APIVersion    string `json:"api_version,omitempty"`
}

var _ = tool.Register(
	tool.Spec{
		Name:    "whatsapp-media",
		Version: "0.1.0",
		Summary: "Upload, download and delete WhatsApp Business media files",
		UseWhen: "sending a local file over WhatsApp (upload, then whatsapp send_media with the media_id) " +
			"or saving received media; a public URL needs no upload",
		Description: "Companion to the whatsapp tool. upload sends a local file to WhatsApp and returns a " +
			"media ID; download saves received media to disk, checking its SHA-256; delete removes uploaded " +
			"media. Defaults such as the phone number ID come from the whatsapp tool's configure.",
		Labels: []string{"whatsapp", "messaging", "meta", "file"},
		Needs: []tool.Need{
			{
				Kind:  tool.NetHTTP,
				Scope: []string{graphHost, lookasideHost},
				Reason: "upload files to the WhatsApp API at graph.facebook.com, and download media from " +
					"lookaside.fbsbx.com, where WhatsApp serves it",
			},
			{
				Kind:  tool.Secret,
				Scope: []string{tokenSecret},
				Reason: "authenticate to WhatsApp. forge attaches the token itself; bind it to " +
					"graph.facebook.com and lookaside.fbsbx.com and it can go nowhere else",
			},
			{
				Kind:  tool.FSRead,
				Scope: []string{"*"},
				Reason: "read the files you ask it to upload. It cannot know your directories, so narrow " +
					"this: forge grant allow whatsapp-media --scope fs.read=/path/to/outbox",
			},
			{
				Kind:  tool.FSWrite,
				Scope: []string{"*"},
				Reason: "save the media you ask it to download. Narrow this too: " +
					"--scope fs.write=/path/to/inbox",
			},
			{
				Kind:   tool.Invoke,
				Scope:  []string{companionTool},
				Reason: "read the default phone number ID and API version from the whatsapp tool's configure",
			},
			{
				Kind:  tool.KV,
				Scope: []string{companionNS},
				Reason: "forge only lets a tool call another with capabilities it holds itself, and whatsapp " +
					"keeps its defaults in kv(config); this tool never uses the namespace directly",
			},
		},
	},
	tool.Op("upload", upload,
		tool.Summary("Upload a local file and return its media ID, for whatsapp send_media"), tool.OpenWorld()),
	tool.Op("download", download,
		tool.Summary("Save received media to a file, verifying its SHA-256"), tool.OpenWorld()),
	tool.Op("delete", deleteMedia,
		tool.Summary("Delete uploaded media"), tool.Destructive(), tool.Idempotent(), tool.OpenWorld()),
)

func main() {}

// loadConfig always returns usable settings; the error says why the whatsapp
// tool's defaults were not read, for the callers that cannot do without them.
func loadConfig(override string) (config, error) {
	var c config
	err := tool.Call(companionTool, "configure", nil, &c)
	if override != "" {
		c.PhoneNumberID = override
	}
	if c.APIVersion == "" {
		c.APIVersion = defaultAPIVersion
	}
	return c, err
}

func absolute(path string) (string, error) {
	if path == "" {
		return "", errors.New("give a path")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%q is not an absolute path; forge shows a tool only the directories it was granted, at their full paths", path)
	}
	return filepath.Clean(path), nil
}

func upload(ctx *tool.Context, a UploadArgs) (UploadOut, error) {
	path, err := absolute(a.Path)
	if err != nil {
		return UploadOut{}, err
	}
	t, err := typeForPath(path, a.MimeType)
	if err != nil {
		return UploadOut{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return UploadOut{}, explainFS(err, path, "fs.read")
	}
	if !info.Mode().IsRegular() {
		return UploadOut{}, fmt.Errorf("%s is not a regular file", path)
	}
	if err := checkSize(t, info.Size()); err != nil {
		return UploadOut{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return UploadOut{}, explainFS(err, path, "fs.read")
	}

	cfg, cfgErr := loadConfig(a.PhoneNumberID)
	if cfg.PhoneNumberID == "" {
		if cfgErr != nil {
			return UploadOut{}, fmt.Errorf("no phone_number_id given, and the whatsapp tool's defaults could not be read: %w", cfgErr)
		}
		return UploadOut{}, errors.New("no phone number ID: pass phone_number_id, or set one with whatsapp configure")
	}

	body, contentType, err := uploadBody(filepath.Base(path), t, data)
	if err != nil {
		return UploadOut{}, err
	}
	ctx.Progress(0, int64(len(data)), "uploading "+filepath.Base(path))
	var res struct {
		ID string `json:"id"`
	}
	if err := graphJSON(tool.HTTPRequest{
		Method:  "POST",
		URL:     graphURL(cfg.APIVersion, url.PathEscape(cfg.PhoneNumberID)+"/media", nil),
		Headers: map[string]string{"Content-Type": contentType},
		Body:    body,
	}, &res); err != nil {
		return UploadOut{}, err
	}
	if res.ID == "" {
		return UploadOut{}, errors.New("the WhatsApp API accepted the upload but returned no media ID")
	}
	return UploadOut{MediaID: res.ID, Kind: t.Kind, MimeType: t.Mime, Bytes: info.Size()}, nil
}

func checkSize(t mediaType, size int64) error {
	if size == 0 {
		return errors.New("the file is empty")
	}
	if limit := kindLimits[t.Kind]; size > limit {
		return fmt.Errorf("the file is %s; WhatsApp allows %s for %s", human(size), human(limit), t.Kind)
	}
	if size > maxUploadBytes {
		return fmt.Errorf("the file is %s; this tool uploads at most %s, which is what fits in forge's "+
			"per-call allowance", human(size), human(maxUploadBytes))
	}
	return nil
}

func uploadBody(filename string, t mediaType, data []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("messaging_product", "whatsapp"); err != nil {
		return nil, "", err
	}
	if err := w.WriteField("type", t.Mime); err != nil {
		return nil, "", err
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", t.Mime)
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(data); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func download(ctx *tool.Context, a DownloadArgs) (DownloadOut, error) {
	if a.MediaID == "" {
		return DownloadOut{}, errors.New("give the media_id to download")
	}
	target, err := absolute(a.Path)
	if err != nil {
		return DownloadOut{}, err
	}
	cfg, _ := loadConfig(a.PhoneNumberID)

	var q url.Values
	if cfg.PhoneNumberID != "" {
		q = url.Values{"phone_number_id": {cfg.PhoneNumberID}}
	}
	var info struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
		SHA256   string `json:"sha256"`
	}
	if err := graphJSON(tool.HTTPRequest{Method: "GET", URL: graphURL(cfg.APIVersion, url.PathEscape(a.MediaID), q)}, &info); err != nil {
		return DownloadOut{}, err
	}
	if info.URL == "" {
		return DownloadOut{}, errors.New("the WhatsApp API returned no download URL for " + a.MediaID)
	}

	target, err = resolveTarget(target, a.MediaID, info.MimeType, a.Overwrite)
	if err != nil {
		return DownloadOut{}, err
	}

	res, err := do(tool.HTTPRequest{Method: "GET", URL: info.URL})
	if err != nil {
		return DownloadOut{}, err
	}
	sum := sha256.Sum256(res.Body)
	got := hex.EncodeToString(sum[:])
	if info.SHA256 != "" && !strings.EqualFold(got, info.SHA256) {
		return DownloadOut{}, fmt.Errorf("the downloaded file's SHA-256 is %s but WhatsApp said %s; nothing was saved", got, info.SHA256)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if a.Overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(target, flags, 0o644)
	if err != nil {
		return DownloadOut{}, explainFS(err, target, "fs.write")
	}
	if _, err := f.Write(res.Body); err != nil {
		_ = f.Close()
		return DownloadOut{}, explainFS(err, target, "fs.write")
	}
	if err := f.Close(); err != nil {
		return DownloadOut{}, explainFS(err, target, "fs.write")
	}
	return DownloadOut{Path: target, MimeType: info.MimeType, Bytes: len(res.Body), SHA256: got}, nil
}

func resolveTarget(path, mediaID, mime string, overwrite bool) (string, error) {
	info, err := os.Stat(path)
	switch {
	case err == nil && info.IsDir():
		path = filepath.Join(path, safeName(mediaID)+extForMime(mime))
		if _, err := os.Stat(path); err == nil && !overwrite {
			return "", fmt.Errorf("%s already exists; pass overwrite to replace it", path)
		}
	case err == nil && !overwrite:
		return "", fmt.Errorf("%s already exists; pass overwrite to replace it", path)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", explainFS(err, path, "fs.write")
	}
	return path, nil
}

func safeName(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, s)
}

func deleteMedia(ctx *tool.Context, a DeleteArgs) (DeleteOut, error) {
	if a.MediaID == "" {
		return DeleteOut{}, errors.New("give the media_id to delete")
	}
	cfg, _ := loadConfig(a.PhoneNumberID)
	var q url.Values
	if cfg.PhoneNumberID != "" {
		q = url.Values{"phone_number_id": {cfg.PhoneNumberID}}
	}
	var out DeleteOut
	if err := graphJSON(tool.HTTPRequest{Method: "DELETE", URL: graphURL(cfg.APIVersion, url.PathEscape(a.MediaID), q)}, &out); err != nil {
		return DeleteOut{}, err
	}
	return out, nil
}

// explainFS names the grant, because an ungranted directory has no mount in
// the sandbox, and the raw error reads like a missing file or, as EBADF, like
// nothing a person could act on.
func explainFS(err error, path, kind string) error {
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EBADF) {
		return fmt.Errorf("%w\n\nIf %s exists, whatsapp-media may not have been granted its directory:\n"+
			"    forge grant allow whatsapp-media --scope %s=%s", err, path, kind, filepath.Dir(path))
	}
	return err
}

func human(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
