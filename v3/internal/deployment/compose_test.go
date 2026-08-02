package deployment

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComposeDefinesIndependentProcesses(t *testing.T) {
	root := repositoryRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Services map[string]struct {
			Command     []string               `yaml:"command"`
			Environment map[string]interface{} `yaml:"environment"`
			Ports       []string               `yaml:"ports"`
			DependsOn   map[string]interface{} `yaml:"depends_on"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("parse compose.yaml: %v", err)
	}
	for _, service := range []string{"postgres", "redis", "migrate", "api", "worker", "bot", "web"} {
		if _, ok := document.Services[service]; !ok {
			t.Fatalf("Compose is missing %s", service)
		}
	}
	commands := map[string]string{
		"migrate": "/app/sakura-migrate",
		"api":     "/app/sakura-api",
		"worker":  "/app/sakura-worker",
		"bot":     "/app/sakura-bot",
	}
	for service, expected := range commands {
		actual := document.Services[service].Command
		if len(actual) != 1 || actual[0] != expected {
			t.Fatalf("%s command = %v, expected %s", service, actual, expected)
		}
	}
	if len(document.Services["api"].Ports) == 0 {
		t.Fatal("API must expose its HTTP port")
	}
	if len(document.Services["web"].Ports) == 0 {
		t.Fatal("Web must expose its HTTP port")
	}
	if _, ok := document.Services["web"].DependsOn["bot"]; ok {
		t.Fatal("Web must not depend on Bot")
	}
	if _, ok := document.Services["bot"].DependsOn["web"]; ok {
		t.Fatal("Bot must not depend on Web")
	}
	if len(document.Services["worker"].Ports) != 0 || len(document.Services["bot"].Ports) != 0 {
		t.Fatal("Worker and Bot health ports must remain internal")
	}
	botEnvironment := document.Services["bot"].Environment
	for key := range botEnvironment {
		if strings.Contains(key, "DATABASE") || strings.Contains(key, "REDIS") {
			t.Fatalf("Bot must not receive %s", key)
		}
	}
	if _, ok := botEnvironment["SAKURA_V3_INTERNAL_BOT_TOKEN"]; !ok {
		t.Fatal("Bot must receive its scoped internal API token")
	}
	if _, ok := botEnvironment["SAKURA_V3_CREDENTIAL_MASTER_KEY"]; ok {
		t.Fatal("Bot must not receive the credential encryption master key")
	}
	workerEnvironment := document.Services["worker"].Environment
	if _, ok := workerEnvironment["SAKURA_V3_CREDENTIAL_MASTER_KEY"]; !ok {
		t.Fatal("Worker must receive the credential encryption master key for Emby connections")
	}
	if _, ok := workerEnvironment["SAKURA_V3_INTERNAL_BOT_TOKEN"]; ok {
		t.Fatal("Worker must not receive the Bot internal API token")
	}
	apiEnvironment := document.Services["api"].Environment
	for _, key := range []string{"SAKURA_V3_CREDENTIAL_MASTER_KEY", "SAKURA_V3_INTERNAL_BOT_TOKEN", "SAKURA_V3_SESSION_COOKIE"} {
		if _, ok := apiEnvironment[key]; !ok {
			t.Fatalf("API is missing %s", key)
		}
	}
	migrateEnvironment := document.Services["migrate"].Environment
	for key := range migrateEnvironment {
		if strings.Contains(key, "REDIS") {
			t.Fatalf("Migrate must not receive %s", key)
		}
	}
}

func TestDockerfileUsesOverridableCommand(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if strings.Contains(text, "ENTRYPOINT") {
		t.Fatal("Compose commands cannot select independent binaries when the image fixes an ENTRYPOINT")
	}
	if !strings.Contains(text, `CMD ["/app/sakura-api"]`) {
		t.Fatal("Dockerfile must default to the API binary")
	}
}

func TestPhase8BlueGreenAndMaintenanceContracts(t *testing.T) {
	root := repositoryRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "deploy", "compose.color.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var color struct {
		Services map[string]struct {
			Profiles []string `yaml:"profiles"`
			Ports    []string `yaml:"ports"`
		} `yaml:"services"`
		Networks map[string]struct {
			External bool `yaml:"external"`
		} `yaml:"networks"`
	}
	if err = yaml.Unmarshal(content, &color); err != nil {
		t.Fatalf("parse color compose: %v", err)
	}
	for _, service := range []string{"api", "web", "worker", "bot"} {
		if _, ok := color.Services[service]; !ok {
			t.Fatalf("color Compose is missing %s", service)
		}
	}
	for _, singleton := range []string{"worker", "bot"} {
		if len(color.Services[singleton].Profiles) != 1 || color.Services[singleton].Profiles[0] != "active" {
			t.Fatalf("%s must only run in the active color profile", singleton)
		}
		if len(color.Services[singleton].Ports) != 0 {
			t.Fatalf("%s must not publish a port", singleton)
		}
	}
	if !color.Networks["shared"].External {
		t.Fatal("color deployments must use the shared data-plane network")
	}
	maintenance, err := os.ReadFile(filepath.Join(root, "deploy", "maintenance-nginx.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(maintenance), "return 503") || !strings.Contains(string(maintenance), "Retry-After") {
		t.Fatal("maintenance endpoint must return 503 and Retry-After")
	}
}

func TestPhase8CutoverHasSafetyGates(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), "ops", "cutover.sh"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, marker := range []string{"execute --confirm", "maintenance", "backup.sh", "reconcile-v2", "emby.reconcile", "switch_proxy_target"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("cutover is missing safety gate %q", marker)
		}
	}
	rollback, err := os.ReadFile(filepath.Join(repositoryRoot(t), "ops", "rollback.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rollback), "writable v2 rollback would lose v3 changes") {
		t.Fatal("rollback must reject unsafe post-traffic v2 activation")
	}
}

func TestPhase8YAMLDocumentsParse(t *testing.T) {
	root := repositoryRoot(t)
	paths := []string{
		filepath.Join(root, "compose.yaml"),
		filepath.Join(root, "deploy", "compose.color.yaml"),
		filepath.Join(root, "deploy", "compose.maintenance.yaml"),
		filepath.Join(root, "deploy", "compose.restore-drill.yaml"),
		filepath.Join(root, "..", ".github", "workflows", "publish-v3-docker.yml"),
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var document any
		if err = yaml.Unmarshal(content, &document); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
