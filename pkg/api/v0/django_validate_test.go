package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// jsonSlice is a small helper to build the pointer-to-JSONSlice shape EnvVars
// is stored as, since a composite literal can't take a slice's address
// directly.
func jsonSlice(envVars ...DjangoEnvVar) *datatypes.JSONSlice[DjangoEnvVar] {
	s := datatypes.JSONSlice[DjangoEnvVar](envVars)
	return &s
}

// TestValidateDjangoEnvVars covers the invariants the API boundary has to
// enforce itself, since a direct REST or generated-client caller bypasses
// the CLI config validator (pkg/config/v0) entirely and writes this model
// straight through.
func TestValidateDjangoEnvVars(t *testing.T) {
	tests := []struct {
		name    string
		envVars *datatypes.JSONSlice[DjangoEnvVar]
		wantErr string
	}{
		{
			name:    "nil is valid - nothing to check",
			envVars: nil,
		},
		{
			name:    "empty is valid",
			envVars: jsonSlice(),
		},
		{
			name:    "a literal value is valid",
			envVars: jsonSlice(DjangoEnvVar{Name: "QLOPS_DB_HOST", Value: "myapp-postgres"}),
		},
		{
			name: "a complete secret reference is valid",
			envVars: jsonSlice(DjangoEnvVar{
				Name: "QLOPS_DB_PASSWORD", SecretName: "myapp-db", SecretKey: "POSTGRES_PASSWORD",
			}),
		},
		{
			name:    "an empty name is rejected",
			envVars: jsonSlice(DjangoEnvVar{Value: "myapp-postgres"}),
			wantErr: "EnvVars[0]",
		},
		{
			name: "both a value and a secret reference is rejected",
			envVars: jsonSlice(DjangoEnvVar{
				Name: "QLOPS_DB_HOST", Value: "myapp-postgres", SecretName: "myapp-db", SecretKey: "POSTGRES_HOST",
			}),
			wantErr: "EnvVars[0]",
		},
		{
			name:    "neither a value nor a secret reference is rejected",
			envVars: jsonSlice(DjangoEnvVar{Name: "QLOPS_DB_HOST"}),
			wantErr: "EnvVars[0]",
		},
		{
			name:    "a secret reference missing SecretKey is rejected",
			envVars: jsonSlice(DjangoEnvVar{Name: "QLOPS_DB_PASSWORD", SecretName: "myapp-db"}),
			wantErr: "EnvVars[0]",
		},
		{
			name:    "a secret reference missing SecretName is rejected",
			envVars: jsonSlice(DjangoEnvVar{Name: "QLOPS_DB_PASSWORD", SecretKey: "POSTGRES_PASSWORD"}),
			wantErr: "EnvVars[0]",
		},
		{
			name: "a valid entry does not mask an invalid one later in the list",
			envVars: jsonSlice(
				DjangoEnvVar{Name: "QLOPS_DB_HOST", Value: "myapp-postgres"},
				DjangoEnvVar{Value: "no-name"},
			),
			wantErr: "EnvVars[1]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDjangoEnvVars(test.envVars)
			if test.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}
