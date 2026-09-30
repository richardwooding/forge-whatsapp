package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type mediaType struct {
	Mime string
	Kind string
	Ext  string
}

// WhatsApp accepts only these, so the table doubles as validation.
var mediaTypes = []mediaType{
	{"audio/aac", "audio", ".aac"},
	{"audio/amr", "audio", ".amr"},
	{"audio/mpeg", "audio", ".mp3"},
	{"audio/mp4", "audio", ".m4a"},
	{"audio/ogg", "audio", ".ogg"},
	{"text/plain", "document", ".txt"},
	{"application/pdf", "document", ".pdf"},
	{"application/msword", "document", ".doc"},
	{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "document", ".docx"},
	{"application/vnd.ms-excel", "document", ".xls"},
	{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "document", ".xlsx"},
	{"application/vnd.ms-powerpoint", "document", ".ppt"},
	{"application/vnd.openxmlformats-officedocument.presentationml.presentation", "document", ".pptx"},
	{"image/jpeg", "image", ".jpg"},
	{"image/png", "image", ".png"},
	{"image/webp", "sticker", ".webp"},
	{"video/mp4", "video", ".mp4"},
	{"video/3gpp", "video", ".3gp"},
}

var extAliases = map[string]string{".jpeg": ".jpg", ".3gpp": ".3gp", ".opus": ".ogg"}

var kindLimits = map[string]int64{
	"audio":    16 << 20,
	"document": 100 << 20,
	"image":    5 << 20,
	"sticker":  500 << 10,
	"video":    16 << 20,
}

func typeForPath(path, override string) (mediaType, error) {
	if override != "" {
		return typeForMime(override)
	}
	ext := strings.ToLower(filepath.Ext(path))
	if alias, ok := extAliases[ext]; ok {
		ext = alias
	}
	for _, t := range mediaTypes {
		if t.Ext == ext {
			return t, nil
		}
	}
	return mediaType{}, fmt.Errorf("WhatsApp does not accept %q files; pass mime_type if the extension is misleading, "+
		"or convert it to one of: %s", ext, supportedExts())
}

func typeForMime(mime string) (mediaType, error) {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	for _, t := range mediaTypes {
		if t.Mime == base {
			return t, nil
		}
	}
	return mediaType{}, fmt.Errorf("WhatsApp does not accept %s media", mime)
}

// extForMime names a download; an unknown type still saves, just without a
// telling extension.
func extForMime(mime string) string {
	if t, err := typeForMime(mime); err == nil {
		return t.Ext
	}
	return ".bin"
}

func supportedExts() string {
	exts := make([]string, len(mediaTypes))
	for i, t := range mediaTypes {
		exts[i] = t.Ext
	}
	sort.Strings(exts)
	return strings.Join(exts, " ")
}
