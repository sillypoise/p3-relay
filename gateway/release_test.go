package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Mock only external commands: the real just recipes must reject bad selectors, failed scans,
// existing public tags and denied metadata access before requesting registry credentials or pushes.
func TestReleaseGuards(t *testing.T) {
	for _, scenario := range []string{"scan", "existing", "denied", "digest", "component"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			writeReleaseCommands(t, directory)
			t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("XDG_RUNTIME_DIR", directory)
			t.Setenv("RELEASE_TEST_LOG", filepath.Join(directory, "calls"))
			t.Setenv("RELEASE_TEST_SCENARIO", scenario)
			arguments := []string{"--justfile", "../justfile", "gateway-container-release",
				strings.Repeat("a", 40), "sha256:" + strings.Repeat("b", 64)}
			if scenario == "digest" {
				arguments[len(arguments)-1] = "sha256:short"
			}
			if scenario == "component" {
				arguments = []string{"--justfile", "../justfile", "container-publish", "unknown"}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := exec.CommandContext(ctx, "just", arguments...).Run(); err == nil {
				t.Fatal("unsafe release accepted")
			}
			if ctx.Err() != nil {
				t.Fatal("guard timed out instead of rejecting")
			}
			calls, err := os.ReadFile(filepath.Join(directory, "calls"))
			if os.IsNotExist(err) {
				if scenario != "digest" && scenario != "component" {
					t.Fatal("guard not exercised")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "digest" || scenario == "component" {
				t.Fatal("invalid selector reached external commands")
			}
			expected := "ecr-public describe-images"
			if scenario == "scan" {
				expected = "describe-image-scan-findings"
			}
			if !strings.Contains(string(calls), expected) {
				t.Fatal("expected guard not reached")
			}
			if strings.Contains(string(calls), "get-login-password") {
				t.Fatal("rejection requested registry credentials")
			}
			if strings.Contains(string(calls), "podman") {
				t.Fatal("rejection touched images")
			}
		})
	}
}

func writeReleaseCommands(t *testing.T, directory string) {
	t.Helper()
	commands := map[string]string{
		"tofu": `#!/bin/sh
set -eu
case "$*" in
*gateway_repository_url*) echo public.ecr.aws/abcdefgh/p3-relay-gateway;;
*repository_url*) echo 123456789012.dkr.ecr.us-east-1.amazonaws.com/p3-relay;;
*) exit 90;;
esac
`,
		"aws": `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$RELEASE_TEST_LOG"
case "$*" in
*get-caller-identity*) echo 123456789012;;
*ecr-public*describe-repositories*) echo public.ecr.aws/abcdefgh/p3-relay-gateway;;
*ecr-public*describe-images*)
    if test "$RELEASE_TEST_SCENARIO" = existing; then echo '{}'; exit 0; fi
    echo AccessDeniedException >&2; exit 254;;
*ecr*describe-images*)
    echo sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb;;
*image-scan-complete*) exit 0;;
*describe-image-scan-findings*)
    if test "$RELEASE_TEST_SCENARIO" = scan; then echo False; else echo True; fi;;
*) exit 90;;
esac
`,
		"podman": "#!/bin/sh\nprintf 'podman\\n' >> \"$RELEASE_TEST_LOG\"\nexit 90\n",
	}
	for name, content := range commands {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o700); err != nil {
			t.Fatal("mock command creation failed")
		}
	}
}
