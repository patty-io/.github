package alias_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/expr-lang/expr"
	"gopkg.in/yaml.v3"
)

type image struct {
	Tag         string
	Digest      string
	Annotations map[string]string
}

func metadataTask(t *testing.T) map[string]any {
	t.Helper()
	path := os.Getenv("RELEASE_METADATA_TASK_PATH")
	if path == "" {
		path = "../../kargo/release-metadata-task.yaml"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var task map[string]any
	if err := yaml.Unmarshal(raw, &task); err != nil {
		t.Fatal(err)
	}
	if task["metadata"].(map[string]any)["name"] != "patty-release-metadata" {
		t.Fatal("release metadata task absent")
	}
	return task
}

func evaluate(t *testing.T, source string, value image) string {
	t.Helper()
	env := map[string]any{
		"vars": map[string]any{"imageRepo": "registry.patty.io/patty-accounts/api"},
		"imageFrom": func(repo string) image {
			if repo != "registry.patty.io/patty-accounts/api" {
				t.Fatalf("unexpected repository %q", repo)
			}
			return value
		},
	}
	source = strings.TrimSpace(source)
	if !strings.HasPrefix(source, "${{") || !strings.HasSuffix(source, "}}") {
		t.Fatalf("not a Kargo expression: %q", source)
	}
	source = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(source, "${{"), "}}"))
	program, err := expr.Compile(source, expr.Env(env))
	if err != nil {
		t.Fatal(err)
	}
	result, err := expr.Run(program, env)
	if err != nil {
		t.Fatal(err)
	}
	text, ok := result.(string)
	if !ok {
		t.Fatalf("expression returned %T instead of string", result)
	}
	return text
}

func aliasExpression(t *testing.T) string {
	t.Helper()
	steps := metadataTask(t)["spec"].(map[string]any)["steps"].([]any)
	first := steps[0].(map[string]any)
	if first["uses"] != "set-freight-alias" {
		t.Fatal("first step must record the Freight alias")
	}
	return first["config"].(map[string]any)["alias"].(string)
}

func TestFreightAliasFitsKubernetesLabel(t *testing.T) {
	source := aliasExpression(t)
	digest := "sha256:" + strings.Repeat("a", 64)
	label := regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_.-]*[A-Za-z0-9])?$`)
	for _, tc := range []struct {
		name string
		tag  string
	}{
		{"short", "v1"},
		{"prefix_boundary", strings.Repeat("x", 50)},
		{"beyond_prefix_boundary", strings.Repeat("x", 51)},
		{"observed_accounts_release", "sha-00833b09f451bfaf409151a07be40c640b4c7198-run-37340457794-1"},
		{"observed_admin_release", "sha-d6f3092d248ed41e00fae5498124a4975468f059-run-37340464077-1"},
		{"large_run_attempt", "sha-" + strings.Repeat("b", 40) + "-run-37340457794-100"},
		{"maximum_registry_tag", strings.Repeat("x", 128)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			alias := evaluate(t, source, image{Tag: tc.tag, Digest: digest})
			if len(alias) > 63 || !label.MatchString(alias) {
				t.Fatalf("alias is not a valid Kubernetes label (%d bytes): %q", len(alias), alias)
			}
			if !strings.HasSuffix(alias, "-aaaaaaaaaaaa") {
				t.Fatalf("digest identity missing: %q", alias)
			}
			if len(tc.tag) <= 50 && alias != tc.tag+"-aaaaaaaaaaaa" {
				t.Fatalf("short tag changed: %q", alias)
			}
		})
	}
}

func TestFreightAliasRetainsDigestUniqueness(t *testing.T) {
	source := aliasExpression(t)
	tag := "sha-00833b09f451bfaf409151a07be40c640b4c7198-run-37340457794-1"
	first := evaluate(t, source, image{Tag: tag, Digest: "sha256:" + strings.Repeat("a", 64)})
	second := evaluate(t, source, image{Tag: tag, Digest: "sha256:" + strings.Repeat("b", 64)})
	if first == second {
		t.Fatal("different digest fragments must produce distinct aliases")
	}
}

func TestDisplayAliasDoesNotTruncateReleaseMetadata(t *testing.T) {
	tag := "sha-00833b09f451bfaf409151a07be40c640b4c7198-run-37340457794-100"
	digest := "sha256:" + strings.Repeat("a", 64)
	revision := "00833b09f451bfaf409151a07be40c640b4c7198"
	value := image{Tag: tag, Digest: digest, Annotations: map[string]string{"org.opencontainers.image.revision": revision}}
	steps := metadataTask(t)["spec"].(map[string]any)["steps"].([]any)
	metadata := steps[1].(map[string]any)
	if metadata["uses"] != "set-metadata" {
		t.Fatal("full release metadata step absent")
	}
	updates := metadata["config"].(map[string]any)["updates"].([]any)
	values := updates[0].(map[string]any)["values"].(map[string]any)
	for field, expected := range map[string]string{"imageTag": tag, "imageDigest": digest, "releaseRevision": revision} {
		t.Run(field, func(t *testing.T) {
			if actual := evaluate(t, values[field].(string), value); actual != expected {
				t.Fatalf("%s identity was changed: got %q, want %q", field, actual, expected)
			}
		})
	}
}
