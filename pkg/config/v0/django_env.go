package v0

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	api_v0 "django-threeport-module/pkg/api/v0"
	apilib "github.com/threeport/threeport/pkg/api/lib/v0"
	encryption "github.com/threeport/threeport/pkg/encryption/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// DjangoSecretEnvVarValues is the config-facing form of an environment
// variable sourced from an existing Kubernetes secret key.
type DjangoSecretEnvVarValues struct {
	Name       string
	SecretName string
	SecretKey  string
}

// envVarNameRegexp matches the names Kubernetes accepts for container env
// vars that Django apps use in practice.
var envVarNameRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// reservedEnvVarNames are wired by the module itself and cannot be set or
// overridden through Env or SecretEnvVars. PYTHONPATH is set on the migration
// job only.
var reservedEnvVarNames = map[string]bool{
	"DATABASE_URL":           true,
	"SECRET_KEY":             true,
	"DJANGO_SETTINGS_MODULE": true,
	"PYTHONPATH":             true,
}

// validateEnvVars checks the Env and SecretEnvVars of one config object:
// every literal is KEY=VALUE with a usable key, every secret reference is
// complete, no name is reserved, and no name appears more than once across the
// two fields.
func validateEnvVars(env *[]string, secretEnvVars []DjangoSecretEnvVarValues) error {
	multiError := util.MultiError{}
	seen := map[string]bool{}

	check := func(name, where string) {
		switch {
		case !envVarNameRegexp.MatchString(name):
			multiError.AppendError(fmt.Errorf(
				"invalid value in config for %s: %q is not a valid environment variable name", where, name,
			))
		case reservedEnvVarNames[name]:
			multiError.AppendError(fmt.Errorf(
				"invalid value in config for %s: %s is set by the module and cannot be overridden", where, name,
			))
		case seen[name]:
			multiError.AppendError(fmt.Errorf(
				"invalid value in config for %s: %s is set more than once across Env and SecretEnvVars", where, name,
			))
		}
		seen[name] = true
	}

	if env != nil {
		for i, entry := range *env {
			name, _, found := strings.Cut(entry, "=")
			if !found {
				multiError.AppendError(fmt.Errorf(
					"invalid value in config for Env[%d]: entry is not in KEY=VALUE form", i,
				))
				continue
			}
			check(name, fmt.Sprintf("Env[%d]", i))
		}
	}

	for i, secretEnvVar := range secretEnvVars {
		where := fmt.Sprintf("SecretEnvVars[%d]", i)
		for field, value := range map[string]string{
			"Name":       secretEnvVar.Name,
			"SecretName": secretEnvVar.SecretName,
			"SecretKey":  secretEnvVar.SecretKey,
		} {
			if value == "" {
				multiError.AppendError(fmt.Errorf("missing required field in config: %s.%s", where, field))
			}
		}
		if secretEnvVar.Name != "" {
			check(secretEnvVar.Name, where)
		}
	}

	return multiError.Error()
}

// secretEnvVarsToAPI converts config secret references to the API form. Nil
// stays nil so the API treats the field as not sent.
func secretEnvVarsToAPI(values []DjangoSecretEnvVarValues) *[]api_v0.DjangoSecretEnvVar {
	if len(values) == 0 {
		return nil
	}
	out := make([]api_v0.DjangoSecretEnvVar, 0, len(values))
	for _, v := range values {
		out = append(out, api_v0.DjangoSecretEnvVar{
			Name: v.Name, SecretName: v.SecretName, SecretKey: v.SecretKey,
		})
	}
	return &out
}

// secretEnvVarsFromAPI converts API secret references to the config form.
func secretEnvVarsFromAPI(vars *[]api_v0.DjangoSecretEnvVar) []DjangoSecretEnvVarValues {
	if vars == nil {
		return nil
	}
	out := make([]DjangoSecretEnvVarValues, 0, len(*vars))
	for _, v := range *vars {
		out = append(out, DjangoSecretEnvVarValues{
			Name: v.Name, SecretName: v.SecretName, SecretKey: v.SecretKey,
		})
	}
	return out
}

func secretEnvVarKey(v DjangoSecretEnvVarValues) string {
	return v.Name + "\x00" + v.SecretName + "\x00" + v.SecretKey
}

// envEntriesChanged reports whether the literal Env in config differs from the
// stored Env. Stored values are encrypted, so they are decrypted with
// encryptionKey and compared in full, values included: a replace that changed
// only a value would otherwise be accepted and then never applied. An entry
// whose value is the redacted placeholder, as `get` prints it, is taken to be
// the stored value rather than an edit.
func envEntriesChanged(
	env *[]string,
	existingEnv *[]string,
	encryptionKey string,
) (bool, error) {
	var want, have []string
	if env != nil {
		want = *env
	}
	if existingEnv != nil {
		have = *existingEnv
	}
	if len(want) == 0 && len(have) == 0 {
		return false, nil
	}
	if encryptionKey == "" {
		return false, errors.New(
			"the encryption key is needed to check that Env is unchanged, but none was provided",
		)
	}

	decrypted, err := encryption.DecryptEnvSlice(have, encryptionKey)
	if err != nil {
		return false, fmt.Errorf("failed to decrypt the stored Env: %w", err)
	}
	stored := make(map[string]string, len(decrypted))
	for _, entry := range decrypted {
		k, v, _ := strings.Cut(entry, "=")
		stored[k] = v
	}

	if len(want) != len(stored) {
		return true, nil
	}
	for _, entry := range want {
		k, v, _ := strings.Cut(entry, "=")
		storedValue, ok := stored[k]
		if !ok {
			return true, nil
		}
		if v != encryption.RedactedValuePlaceholder && v != storedValue {
			return true, nil
		}
	}

	return false, nil
}

// checkEnvUnchanged rejects a replace that would change environment variables
// after creation. They are rendered into the workload once, when the object is
// created, so an edit would be stored and then never applied. Both Env,
// compared value for value, and SecretEnvVars are checked.
func checkEnvUnchanged(
	object string,
	env *[]string,
	secretEnvVars []DjangoSecretEnvVarValues,
	existingEnv *[]string,
	existingSecretEnvVars *[]api_v0.DjangoSecretEnvVar,
	encryptionKey string,
) error {
	changed, err := envEntriesChanged(env, existingEnv, encryptionKey)
	if err != nil {
		return err
	}

	existing := secretEnvVarsFromAPI(existingSecretEnvVars)
	if len(existing) != len(secretEnvVars) {
		changed = true
	} else {
		have := map[string]bool{}
		for _, v := range existing {
			have[secretEnvVarKey(v)] = true
		}
		for _, v := range secretEnvVars {
			if !have[secretEnvVarKey(v)] {
				changed = true
			}
		}
	}

	if changed {
		return errors.New(object + " environment variables cannot be changed after creation: " +
			"they are applied when the workload is created - create a new " + object + " instead")
	}
	return nil
}

// decryptOrRedactDefinition decrypts encrypted values when a key is given and
// redacts them otherwise, the same handling MachineWorkloadDefinitionConfig.Get
// gives its own Env.
func decryptOrRedactDefinition(
	d api_v0.DjangoDefinition,
	encryptionKey string,
) (api_v0.DjangoDefinition, error) {
	if encryptionKey != "" {
		out, err := apilib.DecryptValues(&d, encryptionKey)
		if err != nil {
			return d, fmt.Errorf("failed to decrypt django definition secret values: %w", err)
		}
		return *(out.(*api_v0.DjangoDefinition)), nil
	}
	return *(apilib.RedactEncryptedValues(&d).(*api_v0.DjangoDefinition)), nil
}

// decryptOrRedactInstance is the DjangoInstance equivalent.
func decryptOrRedactInstance(
	i api_v0.DjangoInstance,
	encryptionKey string,
) (api_v0.DjangoInstance, error) {
	if encryptionKey != "" {
		out, err := apilib.DecryptValues(&i, encryptionKey)
		if err != nil {
			return i, fmt.Errorf("failed to decrypt django instance secret values: %w", err)
		}
		return *(out.(*api_v0.DjangoInstance)), nil
	}
	return *(apilib.RedactEncryptedValues(&i).(*api_v0.DjangoInstance)), nil
}
