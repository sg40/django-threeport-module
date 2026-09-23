// hand-written helper type for DjangoDefinition.EnvVars - not an API object
// in its own right, so it is kept out of django.go (the group file the SDK
// generator scans for object declarations) to avoid being mistaken for one.
package v0

// DjangoEnvVar sets one additional environment variable on the Django
// application and, when RunMigrations is enabled, its migration job. Set
// either Value, or both SecretName and SecretKey to reference a key in an
// existing Kubernetes secret - not both. SecretName/SecretKey exist mainly
// to let an app read this module's own generated database secret (see
// DbSecretName) under a name of the app's choosing, since that secret's
// values - particularly the randomly-generated password - are not knowable
// ahead of time as a literal.
type DjangoEnvVar struct {
	Name       string `json:"Name"`
	Value      string `json:"Value,omitempty"`
	SecretName string `json:"SecretName,omitempty"`
	SecretKey  string `json:"SecretKey,omitempty"`
}
