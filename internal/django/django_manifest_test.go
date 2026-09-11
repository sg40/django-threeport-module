package django

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

// kindsIn returns the kind of every document in a multi-document YAML string,
// in order. Splitting on the separator is enough here because the manifest is
// built from structured objects, so no document body can contain one.
func kindsIn(t *testing.T, doc string) []string {
	t.Helper()

	var kinds []string
	for _, chunk := range strings.Split(doc, "\n---\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		var parsed struct {
			Kind string `json:"kind"`
		}
		require.NoError(t, yaml.Unmarshal([]byte(chunk), &parsed), "every document must be valid YAML")
		kinds = append(kinds, parsed.Kind)
	}

	return kinds
}

// TestDjangoYaml_Resources covers the shape of a default deployment: a
// database with its storage and credentials, the migration job, and the
// application itself.
func TestDjangoYaml_Resources(t *testing.T) {
	doc, err := djangoYaml("myapp", "myorg/myapp:v1", "myapp.settings.production", 2, "dev", 20, true)
	require.NoError(t, err)

	kinds := kindsIn(t, doc)

	assert.Contains(t, kinds, "Secret")
	assert.Contains(t, kinds, "PersistentVolumeClaim")
	assert.Contains(t, kinds, "Job", "migrations were requested")
	assert.Contains(t, kinds, "Service")
	assert.Equal(t, 2, countOf(kinds, "Deployment"), "one for postgres and one for the app")
	assert.Equal(t, 2, countOf(kinds, "Service"), "one for postgres and one for the app")
}

// TestDjangoYaml_MigrationsDisabled covers RunMigrations being false: the job
// is the only resource that should disappear.
func TestDjangoYaml_MigrationsDisabled(t *testing.T) {
	doc, err := djangoYaml("myapp", "myorg/myapp:v1", "", 1, "dev", 20, false)
	require.NoError(t, err)

	kinds := kindsIn(t, doc)

	assert.NotContains(t, kinds, "Job", "no migration job when migrations are turned off")
	assert.Contains(t, kinds, "Deployment", "the application is still deployed")
}

// TestDjangoYaml_SettingsModuleOmitted covers the optional settings module.
// Django falls back to its own default when the variable is absent, so an
// empty value must not be set rather than set to "".
func TestDjangoYaml_SettingsModuleOmitted(t *testing.T) {
	withSettings, err := djangoYaml("myapp", "myorg/myapp:v1", "myapp.settings.production", 1, "dev", 20, false)
	require.NoError(t, err)
	assert.Contains(t, withSettings, "DJANGO_SETTINGS_MODULE")

	withoutSettings, err := djangoYaml("myapp", "myorg/myapp:v1", "", 1, "dev", 20, false)
	require.NoError(t, err)
	assert.NotContains(t, withoutSettings, "DJANGO_SETTINGS_MODULE")
}

// TestGeneratePassword covers the credential the database and the application
// share. A predictable value here would be recoverable by anyone who knows
// roughly when the definition was created.
func TestGeneratePassword(t *testing.T) {
	first, err := generatePassword(32)
	require.NoError(t, err)
	second, err := generatePassword(32)
	require.NoError(t, err)

	assert.NotEmpty(t, first)
	assert.NotEqual(t, first, second, "two passwords generated in the same instant must differ")
	assert.NotContains(t, first, "=", "the encoding must be URL-safe and unpadded")
}

func countOf(items []string, want string) int {
	var n int
	for _, item := range items {
		if item == want {
			n++
		}
	}

	return n
}
