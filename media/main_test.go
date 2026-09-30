package main

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/richardwooding/forge/sdk/tool"
)

func TestTypeForPath(t *testing.T) {
	for path, want := range map[string]string{
		"/a/photo.JPG":    "image/jpeg",
		"/a/photo.jpeg":   "image/jpeg",
		"/a/note.opus":    "audio/ogg",
		"/a/sticker.webp": "image/webp",
		"/a/deck.pptx":    "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"/a/clip.3gpp":    "video/3gpp",
	} {
		got, err := typeForPath(path, "")
		if err != nil || got.Mime != want {
			t.Errorf("typeForPath(%q) = %q, %v; want %q", path, got.Mime, err, want)
		}
	}
	if _, err := typeForPath("/a/anim.gif", ""); err == nil || !strings.Contains(err.Error(), ".pdf") {
		t.Errorf("a .gif should be refused with the supported list: %v", err)
	}
	if got, err := typeForPath("/a/blob", "application/pdf; charset=binary"); err != nil || got.Kind != "document" {
		t.Errorf("an explicit mime type should win: %+v, %v", got, err)
	}
	if _, err := typeForPath("/a/x.pdf", "image/gif"); err == nil {
		t.Error("an unsupported explicit mime type was accepted")
	}
}

func TestCheckSize(t *testing.T) {
	image, _ := typeForMime("image/png")
	sticker, _ := typeForMime("image/webp")
	doc, _ := typeForMime("application/pdf")
	for name, tc := range map[string]struct {
		t    mediaType
		size int64
		ok   bool
		want string
	}{
		"small image":         {image, 1 << 20, true, ""},
		"empty":               {image, 0, false, "empty"},
		"image over 5 MiB":    {image, 6 << 20, false, "5.0 MiB for image"},
		"sticker over 500KiB": {sticker, 600 << 10, false, "500 KiB for sticker"},
		"document over forge": {doc, 50 << 20, false, "forge's per-call allowance"},
		"document under both": {doc, 30 << 20, true, ""},
	} {
		err := checkSize(tc.t, tc.size)
		if (err == nil) != tc.ok || (err != nil && !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestUploadBody(t *testing.T) {
	typ, _ := typeForMime("image/png")
	payload := []byte("\x89PNG not really")
	body, contentType, err := uploadBody(`we"ird.png`, typ, payload)
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(contentType)
	if err != nil || mt != "multipart/form-data" {
		t.Fatalf("content type %q: %v", contentType, err)
	}
	r := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	fields := map[string]string{}
	for {
		p, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(p)
		fields[p.FormName()] = string(data)
		if p.FormName() == "file" {
			if p.FileName() != `we"ird.png` {
				t.Errorf("filename = %q", p.FileName())
			}
			if ct := p.Header.Get("Content-Type"); ct != "image/png" {
				t.Errorf("file part Content-Type = %q, want image/png", ct)
			}
		}
	}
	if fields["messaging_product"] != "whatsapp" || fields["type"] != "image/png" || fields["file"] != string(payload) {
		t.Errorf("fields = %q", fields)
	}
}

func TestResolveTarget(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "taken.jpg")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveTarget(dir, "123/../456", "image/jpeg", false)
	if want := filepath.Join(dir, "123_.._456.jpg"); err != nil || got != want {
		t.Errorf("into a directory: %q, %v; want %q", got, err, want)
	}
	if got, err := resolveTarget(dir, "789", "application/x-unknown", false); err != nil || filepath.Ext(got) != ".bin" {
		t.Errorf("an unknown type should save as .bin: %q, %v", got, err)
	}
	if _, err := resolveTarget(existing, "1", "image/jpeg", false); err == nil {
		t.Error("an existing file was going to be overwritten")
	}
	if got, err := resolveTarget(existing, "1", "image/jpeg", true); err != nil || got != existing {
		t.Errorf("overwrite: %q, %v", got, err)
	}
	fresh := filepath.Join(dir, "new.jpg")
	if got, err := resolveTarget(fresh, "1", "image/jpeg", false); err != nil || got != fresh {
		t.Errorf("a new file: %q, %v", got, err)
	}
}

func TestAbsolute(t *testing.T) {
	if _, err := absolute("photo.jpg"); err == nil {
		t.Error("a relative path was accepted")
	}
	if _, err := absolute(""); err == nil {
		t.Error("an empty path was accepted")
	}
	if got, err := absolute("/a/b/../c.jpg"); err != nil || got != "/a/c.jpg" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestStatusError(t *testing.T) {
	err := statusError(tool.HTTPResponse{Status: 400, Body: []byte(`{"error":{"message":"bad","code":131053,"fbtrace_id":"T"}}`)})
	for _, want := range []string{"131053", "bad", "type and size", "fbtrace_id: T"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q does not mention %q", err, want)
		}
	}
	long := statusError(tool.HTTPResponse{Status: 502, Body: bytes.Repeat([]byte("x"), 2000)})
	if len(long.Error()) > 700 {
		t.Errorf("a non-JSON error body was not trimmed: %d bytes", len(long.Error()))
	}
}

func TestExplainFSNamesTheGrant(t *testing.T) {
	err := explainFS(os.ErrNotExist, "/home/me/out/a.jpg", "fs.read")
	if !strings.Contains(err.Error(), "--scope fs.read=/home/me/out") || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("got %v", err)
	}
	if err := explainFS(&os.PathError{Op: "stat", Path: "/x", Err: syscall.EBADF}, "/x/a.jpg", "fs.read"); !strings.Contains(err.Error(), "--scope") {
		t.Errorf("EBADF, which is how an unmounted directory fails, should name the grant: %v", err)
	}
	other := errors.New("disk on fire")
	if explainFS(other, "/x", "fs.write") != other {
		t.Error("an unrelated error should pass through")
	}
}
