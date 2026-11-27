package media

import (
	"slices"
	"testing"
)

func TestDecidePlaybackMode(t *testing.T) {
	cases := []struct {
		name            string
		ext             string
		video           string
		audio           string
		ffmpegAvailable bool
		want            string
	}{
		{"mp4 h264 aac direct", ".mp4", "h264", "aac", true, "direct"},
		{"webm vp9 opus direct", ".webm", "vp9", "opus", true, "direct"},
		{"mp4 unknown codecs direct", ".mp4", "", "", true, "direct"},
		{"mkv hevc ac3 hls", ".mkv", "hevc", "ac3", true, "hls"},
		{"mp4 hevc aac hls", ".mp4", "hevc", "aac", true, "hls"},
		{"mp4 h264 ac3 hls", ".mp4", "h264", "ac3", true, "hls"},
		{"mkv without ffmpeg falls back to direct", ".mkv", "hevc", "ac3", false, "direct"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DecidePlaybackMode(tc.ext, tc.video, tc.audio, tc.ffmpegAvailable)
			if got != tc.want {
				t.Fatalf("DecidePlaybackMode(%q, %q, %q, %v) = %q, want %q", tc.ext, tc.video, tc.audio, tc.ffmpegAvailable, got, tc.want)
			}
		})
	}
}

func TestHLSFilenameAndSessionValidation(t *testing.T) {
	validFiles := []string{"index.m3u8", "seg_0000.ts", "seg_0123.ts"}
	for _, f := range validFiles {
		if !IsValidHLSFile(f) {
			t.Errorf("expected valid HLS file %q", f)
		}
	}

	invalidFiles := []string{"../index.m3u8", "seg_1.ts", "seg_00001.ts", "other.m3u8", "seg_0000.mp4", ""}
	for _, f := range invalidFiles {
		if IsValidHLSFile(f) {
			t.Errorf("expected invalid HLS file %q", f)
		}
	}

	if !IsValidSessionID("0123456789abcdef0123456789abcdef") {
		t.Error("expected valid hex session ID")
	}
	for _, bad := range []string{"", "short", "../escape", "0123456789ABCDEF0123456789ABCDEF", "0123456789abcdef;id"} {
		if IsValidSessionID(bad) {
			t.Errorf("expected invalid session ID %q", bad)
		}
	}
}

func TestBuildFFmpegHLSArgs(t *testing.T) {
	vaapiArgs := BuildFFmpegHLSArgs("/media/movie.mkv", "/tmp/hls/abc", true)
	for _, required := range []string{"-hwaccel", "vaapi", "/dev/dri/renderD128", "h264_vaapi", "format=nv12|vaapi,hwupload", "/tmp/hls/abc/index.m3u8"} {
		if !slices.Contains(vaapiArgs, required) {
			t.Errorf("VAAPI args missing %q: %v", required, vaapiArgs)
		}
	}

	swArgs := BuildFFmpegHLSArgs("/media/movie.mkv", "/tmp/hls/abc", false)
	for _, required := range []string{"libx264", "veryfast", "yuv420p", "/tmp/hls/abc/seg_%04d.ts", "/tmp/hls/abc/index.m3u8"} {
		if !slices.Contains(swArgs, required) {
			t.Errorf("software args missing %q: %v", required, swArgs)
		}
	}
	if slices.Contains(swArgs, "h264_vaapi") {
		t.Errorf("software args should not contain h264_vaapi: %v", swArgs)
	}
}
