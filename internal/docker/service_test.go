package docker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestBoundaryValidators(t *testing.T) {
	validContainers := []string{"nginx", "ps-wg-a1b2c3d4e5f6", "app_v1.2", "a1b2c3d4e5f6"}
	for _, ref := range validContainers {
		if !ValidateContainerRef(ref) {
			t.Errorf("expected valid container ref %q", ref)
		}
	}
	invalidContainers := []string{"", "-evil", "foo;id", "a|b", "a b", "../escape", "foo..bar", strings.Repeat("a", 129)}
	for _, ref := range invalidContainers {
		if ValidateContainerRef(ref) {
			t.Errorf("expected invalid container ref %q", ref)
		}
	}

	validImages := []string{
		"nginx:latest",
		"ghcr.io/org/app:v1.2.3",
		"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"registry.local:5000/team/app@sha256:abcdef123456",
	}
	for _, ref := range validImages {
		if !ValidateImageRef(ref) {
			t.Errorf("expected valid image ref %q", ref)
		}
	}
	invalidImages := []string{"", "-flag", "nginx;rm", "repo/../evil", "has space", strings.Repeat("a", 257)}
	for _, ref := range invalidImages {
		if ValidateImageRef(ref) {
			t.Errorf("expected invalid image ref %q", ref)
		}
	}

	validStacks := []string{"npm", "media-stack", "app_01", "a"}
	for _, name := range validStacks {
		if !ValidateStackName(name) {
			t.Errorf("expected valid stack name %q", name)
		}
	}
	invalidStacks := []string{"", "Upper", "-bad", "_bad", "a.b", "a/b", "../x", "foo;bar", strings.Repeat("a", 65)}
	for _, name := range invalidStacks {
		if ValidateStackName(name) {
			t.Errorf("expected invalid stack name %q", name)
		}
	}

	validVolNet := []string{"pgdata", "app_net-1.0", "a1b2c3d4"}
	for _, ref := range validVolNet {
		if !ValidateVolumeOrNetwork(ref) {
			t.Errorf("expected valid volume/network ref %q", ref)
		}
	}
	invalidVolNet := []string{"", "-net", "a..b", "a/b", "a;b", "a b", strings.Repeat("a", 129)}
	for _, ref := range invalidVolNet {
		if ValidateVolumeOrNetwork(ref) {
			t.Errorf("expected invalid volume/network ref %q", ref)
		}
	}
}

func TestNDJSONAndInspectParsers(t *testing.T) {
	dfRaw := `{"Type":"Images","TotalCount":"5","Active":"2","Size":"1.2GB","Reclaimable":"400MB (33%)"}
{"Type":"Containers","TotalCount":"3","Active":"1","Size":"20MB","Reclaimable":"10MB (50%)"}
`
	df := ParseDiskUsageNDJSON(dfRaw)
	if len(df) != 2 || df[0].Type != "Images" || df[1].Active != "1" {
		t.Fatalf("unexpected disk usage parsed: %+v", df)
	}

	psRaw := `{"ID":"abcdef1234567890","Names":"/nginx-proxy","Image":"jc21/nginx-proxy-manager:latest","State":"running","Status":"Up 2 hours","Ports":"0.0.0.0:80->80/tcp","CreatedAt":"2026-11-27 10:00:00 +0900 KST","Labels":"com.docker.compose.project=npm"}
{"ID":"1111222233334444","Names":"ps-wg-a1b2","Image":"ps-wg:1","State":"exited","Status":"Exited (0)","Ports":"","CreatedAt":"2026-11-27 11:00:00 +0900 KST","Labels":"personal-server.wargame.id=a1b2"}
`
	containers := ParseContainersNDJSON(psRaw)
	if len(containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(containers))
	}
	if containers[0].Name != "nginx-proxy" || containers[0].IsWargame {
		t.Fatalf("unexpected container[0]: %+v", containers[0])
	}
	if !containers[1].IsWargame {
		t.Fatalf("expected container[1] IsWargame=true, got %+v", containers[1])
	}

	statsRaw := `{"ID":"abcdef123456","Container":"abcdef123456","Name":"nginx-proxy","CPUPerc":"1.25%","MemUsage":"45MiB / 8GiB","MemPerc":"0.55%"}`
	MergeContainerStats(containers, ParseStatsNDJSON(statsRaw))
	if containers[0].CPUPercent != "1.25%" || containers[0].MemPercent != "0.55%" {
		t.Fatalf("expected merged stats on container[0], got %+v", containers[0])
	}

	imgRaw := `{"ID":"sha256:aaaa1111bbbb2222","Repository":"nginx","Tag":"latest","Size":"187MB","CreatedSince":"2 weeks ago"}
{"ID":"sha256:cccc3333dddd4444","Repository":"alpine","Tag":"3.20","Size":"7.8MB","CreatedSince":"1 month ago"}
`
	images := ParseImagesNDJSON(imgRaw, map[string]bool{"sha256:aaaa1111bbbb2222": true})
	if len(images) != 2 || !images[0].InUse || images[1].InUse {
		t.Fatalf("unexpected images parsed: %+v", images)
	}

	if r, tot := ParseComposePSJSON(`[{"State":"running"},{"State":"exited"}]`); r != 1 || tot != 2 {
		t.Fatalf("expected array compose ps (1,2), got (%d,%d)", r, tot)
	}
	if r, tot := ParseComposePSJSON("{\"State\":\"running\"}\n{\"State\":\"running\"}\n"); r != 2 || tot != 2 {
		t.Fatalf("expected ndjson compose ps (2,2), got (%d,%d)", r, tot)
	}

	inspectRaw := `[{
		"Id": "abcdef123456",
		"Name": "/demo",
		"State": {"Status": "running"},
		"Config": {"Image": "nginx:latest", "Env": ["A=1"]},
		"HostConfig": {"RestartPolicy": {"Name": "unless-stopped"}},
		"Mounts": [{"Type": "bind", "Source": "/host", "Destination": "/data", "RW": true}],
		"NetworkSettings": {
			"Networks": {"bridge": {"IPAddress": "172.17.0.2"}},
			"Ports": {"80/tcp": [{"HostIp": "0.0.0.0", "HostPort": "8080"}]}
		}
	}]`
	insp, err := ParseContainerInspect(inspectRaw)
	if err != nil {
		t.Fatalf("unexpected inspect parse error: %v", err)
	}
	if insp.Name != "demo" || insp.RestartPolicy != "unless-stopped" || len(insp.Mounts) != 1 || len(insp.Networks) != 1 || len(insp.Ports) != 1 || insp.Ports[0] != "0.0.0.0:8080->80/tcp" {
		t.Fatalf("unexpected inspect struct: %+v", insp)
	}
}

func TestStackLifecycleAndProtectedNetwork(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.SaveStack("BadName", "services: {}"); !errors.Is(err, ErrInvalidRef) {
		t.Fatalf("expected ErrInvalidRef for bad stack name, got %v", err)
	}
	if err := svc.SaveStack("demo", ":\n  - [\ninvalid"); !errors.Is(err, ErrInvalidYAML) {
		t.Fatalf("expected ErrInvalidYAML for malformed yaml, got %v", err)
	}

	composeYAML := "services:\n  web:\n    image: nginx:alpine\n"
	if err := svc.SaveStack("demo", composeYAML); err != nil {
		t.Fatalf("SaveStack failed: %v", err)
	}

	got, err := svc.GetStack("demo")
	if err != nil || got != composeYAML {
		t.Fatalf("GetStack mismatch: err=%v got=%q", err, got)
	}

	stacks, err := svc.ListStacks(context.Background())
	if err != nil || len(stacks) != 1 || stacks[0].Name != "demo" {
		t.Fatalf("unexpected ListStacks result: err=%v stacks=%+v", err, stacks)
	}

	if err := svc.DeleteStack(context.Background(), "demo"); err != nil {
		t.Fatalf("DeleteStack failed: %v", err)
	}
	if _, err := svc.GetStack("demo"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist after delete, got %v", err)
	}

	for _, protected := range []string{"bridge", "host", "none", "BRIDGE"} {
		if err := svc.RemoveNetwork(context.Background(), protected); !errors.Is(err, ErrProtectedNetwork) {
			t.Fatalf("expected ErrProtectedNetwork for %q, got %v", protected, err)
		}
	}
}
