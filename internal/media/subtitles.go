package media

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/transform"
)

type SubtitleTrack struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Rel    string `json:"relPath"`
}

func FindSiblingSubtitles(videoRealPath, videoCleanRel string) []SubtitleTrack {
	dir := filepath.Dir(videoRealPath)
	base := filepath.Base(videoRealPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	relDir := filepath.ToSlash(filepath.Dir(filepath.FromSlash(videoCleanRel)))
	if relDir == "." {
		relDir = ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var tracks []SubtitleTrack
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".vtt" && ext != ".srt" && ext != ".smi" {
			continue
		}
		entryStem := strings.TrimSuffix(name, filepath.Ext(name))
		if !strings.EqualFold(entryStem, stem) && !strings.HasPrefix(strings.ToLower(entryStem), strings.ToLower(stem)+".") {
			continue
		}

		label := "한국어 / 기본"
		if len(entryStem) > len(stem)+1 {
			label = entryStem[len(stem)+1:]
		} else {
			label = strings.TrimPrefix(ext, ".")
		}
		subRel := name
		if relDir != "" {
			subRel = relDir + "/" + name
		}
		tracks = append(tracks, SubtitleTrack{
			Name:   label,
			Format: strings.TrimPrefix(ext, "."),
			Rel:    subRel,
		})
	}

	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].Rel < tracks[j].Rel
	})
	return tracks
}

func LoadAsWebVTT(realPath string) ([]byte, error) {
	raw, err := os.ReadFile(realPath)
	if err != nil {
		return nil, err
	}
	text := decodeTextUTF8(raw)
	ext := strings.ToLower(filepath.Ext(realPath))
	switch ext {
	case ".vtt":
		return []byte(text), nil
	case ".srt":
		return []byte(ConvertSRTToVTT(text)), nil
	case ".smi":
		return []byte(ConvertSMIToVTT(text)), nil
	default:
		return nil, fmt.Errorf("unsupported subtitle format: %s", ext)
	}
}

func decodeTextUTF8(raw []byte) string {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(raw) {
		return string(raw)
	}
	decoded, _, err := transform.Bytes(korean.EUCKR.NewDecoder(), raw)
	if err == nil && utf8.Valid(decoded) {
		return string(decoded)
	}
	return string(raw)
}

var srtTimestampRe = regexp.MustCompile(`(\d{2}:\d{2}:\d{2}),(\d{3})\s*-->\s*(\d{2}:\d{2}:\d{2}),(\d{3})`)

func ConvertSRTToVTT(srt string) string {
	normalized := strings.ReplaceAll(srt, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	converted := srtTimestampRe.ReplaceAllString(normalized, "$1.$2 --> $3.$4")
	return "WEBVTT\n\n" + strings.TrimSpace(converted) + "\n"
}

var (
	smiSyncRe    = regexp.MustCompile(`(?i)<sync\b[^>]*start\s*=\s*["']?(\d+)["']?[^>]*>`)
	smiBrRe      = regexp.MustCompile(`(?i)<br\s*/?>`)
	smiCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	smiStyleRe   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	smiHeadRe    = regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head>`)
	smiTagRe     = regexp.MustCompile(`<[^>]+>`)
)

type smiCue struct {
	startMs int64
	text    string
}

func formatVTTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	hours := ms / 3_600_000
	ms %= 3_600_000
	mins := ms / 60_000
	ms %= 60_000
	secs := ms / 1_000
	millis := ms % 1_000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, mins, secs, millis)
}

func ConvertSMIToVTT(smi string) string {
	clean := smiCommentRe.ReplaceAllString(smi, "")
	clean = smiHeadRe.ReplaceAllString(clean, "")
	clean = smiStyleRe.ReplaceAllString(clean, "")

	matches := smiSyncRe.FindAllStringSubmatchIndex(clean, -1)
	if len(matches) == 0 {
		return "WEBVTT\n\n"
	}

	cues := make([]smiCue, 0, len(matches))
	for i, m := range matches {
		startStr := clean[m[2]:m[3]]
		startMs, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			continue
		}
		bodyStart := m[1]
		bodyEnd := len(clean)
		if i+1 < len(matches) {
			bodyEnd = matches[i+1][0]
		}
		segment := clean[bodyStart:bodyEnd]
		segment = smiBrRe.ReplaceAllString(segment, "\n")
		segment = smiTagRe.ReplaceAllString(segment, "")
		segment = strings.ReplaceAll(segment, "&nbsp;", " ")
		segment = html.UnescapeString(segment)

		lines := strings.Split(segment, "\n")
		kept := make([]string, 0, len(lines))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				kept = append(kept, trimmed)
			}
		}
		cues = append(cues, smiCue{
			startMs: startMs,
			text:    strings.Join(kept, "\n"),
		})
	}

	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for i, cue := range cues {
		if cue.text == "" {
			continue
		}
		endMs := cue.startMs + 4000
		if i+1 < len(cues) && cues[i+1].startMs > cue.startMs {
			endMs = cues[i+1].startMs
		}
		b.WriteString(formatVTTTime(cue.startMs))
		b.WriteString(" --> ")
		b.WriteString(formatVTTTime(endMs))
		b.WriteString("\n")
		b.WriteString(cue.text)
		b.WriteString("\n\n")
	}
	return b.String()
}
