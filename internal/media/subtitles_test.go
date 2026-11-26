package media_test

import (
	"strings"
	"testing"

	"personal-server/internal/media"
)

func TestConvertSRTToVTT(t *testing.T) {
	srt := "1\r\n00:00:01,250 --> 00:00:04,500\r\n안녕하세요!\r\n"
	vtt := media.ConvertSRTToVTT(srt)
	if !strings.HasPrefix(vtt, "WEBVTT\n\n") {
		t.Fatalf("missing WEBVTT header: %q", vtt)
	}
	if !strings.Contains(vtt, "00:00:01.250 --> 00:00:04.500") {
		t.Fatalf("timestamps not converted: %q", vtt)
	}
	if !strings.Contains(vtt, "안녕하세요!") {
		t.Fatalf("caption text missing: %q", vtt)
	}
}

func TestConvertSMIToVTT(t *testing.T) {
	smi := `<SAMI><HEAD><TITLE>Sample</TITLE></HEAD><BODY>
<SYNC Start=1500><P Class=KRCC>첫 번째 대사<br>두 번째 줄
<SYNC Start=3200><P Class=KRCC>&nbsp;
<SYNC Start=5000><P Class=KRCC>마지막 대사
</BODY></SAMI>`
	vtt := media.ConvertSMIToVTT(smi)
	if !strings.Contains(vtt, "00:00:01.500 --> 00:00:03.200") {
		t.Fatalf("expected first cue range in VTT, got:\n%s", vtt)
	}
	if !strings.Contains(vtt, "첫 번째 대사\n두 번째 줄") {
		t.Fatalf("expected multiline SMI text in VTT, got:\n%s", vtt)
	}
	if !strings.Contains(vtt, "00:00:05.000 --> 00:00:09.000") {
		t.Fatalf("expected final cue fallback duration in VTT, got:\n%s", vtt)
	}
}
