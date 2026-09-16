package django

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kubeerrors "k8s.io/apimachinery/pkg/api/errors"
	kubemetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubetesting "k8s.io/client-go/testing"
)

// fakeKubeClient returns a dynamic client backed by an in-memory object
// tracker, seeded with the given objects.
func fakeKubeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Version: "v1", Kind: "SecretList"},
		&unstructured.UnstructuredList{},
	)

	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{secretResource: "SecretList"},
		objects...,
	)
}

// readSecret returns the stringData or data a secret holds under a key.
func readSecret(t *testing.T, kubeClient dynamic.Interface, namespace, name, key string) string {
	t.Helper()

	secret, err := kubeClient.Resource(secretResource).
		Namespace(namespace).
		Get(context.TODO(), name, kubemetav1.GetOptions{})
	require.NoError(t, err)

	for _, field := range []string{"stringData", "data"} {
		values, found, err := unstructured.NestedStringMap(secret.Object, field)
		require.NoError(t, err)
		if found {
			if value, ok := values[key]; ok {
				return value
			}
		}
	}

	return ""
}

// TestEnsureSecret_CreatesWhenAbsent covers the ordinary path: nothing is there
// and the reconciler puts the credential in place.
func TestEnsureSecret_CreatesWhenAbsent(t *testing.T) {
	kubeClient := fakeKubeClient()

	created, err := ensureSecret(
		kubeClient, "app-abc123", "app-db",
		map[string]string{"app.kubernetes.io/name": "postgres"},
		map[string]string{"POSTGRES_PASSWORD": "first"},
	)
	require.NoError(t, err)
	assert.True(t, created, "an absent secret has to be reported as created")
	assert.Equal(t, "first", readSecret(t, kubeClient, "app-abc123", "app-db", "POSTGRES_PASSWORD"))
}

// TestEnsureSecret_PreservesExisting is the contract the whole design rests on.
// Reconciliation runs again on every requeue, and the database has already been
// initialised with the password in the secret: rewriting it would hand Postgres
// a credential it no longer accepts.
func TestEnsureSecret_PreservesExisting(t *testing.T) {
	kubeClient := fakeKubeClient()

	_, err := ensureSecret(
		kubeClient, "app-abc123", "app-db", nil,
		map[string]string{"POSTGRES_PASSWORD": "the-one-postgres-knows"},
	)
	require.NoError(t, err)

	created, err := ensureSecret(
		kubeClient, "app-abc123", "app-db", nil,
		map[string]string{"POSTGRES_PASSWORD": "a-freshly-generated-one"},
	)
	require.NoError(t, err)
	assert.False(t, created, "an existing secret must not be reported as created")
	assert.Equal(
		t, "the-one-postgres-knows",
		readSecret(t, kubeClient, "app-abc123", "app-db", "POSTGRES_PASSWORD"),
		"the running database's credential must survive a second reconcile pass",
	)
}

// TestEnsureSecret_LosesTheCreateRace covers two reconcile passes reaching the
// create at once: the get says the secret is absent and the create then fails
// with AlreadyExists. The secret is there either way, which is the outcome the
// caller asked for.
func TestEnsureSecret_LosesTheCreateRace(t *testing.T) {
	kubeClient := fakeKubeClient()
	kubeClient.PrependReactor("create", "secrets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, nil, kubeerrors.NewAlreadyExists(
			schema.GroupResource{Resource: "secrets"}, "app-db",
		)
	})

	created, err := ensureSecret(
		kubeClient, "app-abc123", "app-db", nil,
		map[string]string{"POSTGRES_PASSWORD": "first"},
	)
	require.NoError(t, err, "losing the race is not a failure")
	assert.False(t, created)
}

// TestEnsureSecret_PropagatesGetErrors covers an API that is failing rather than
// reporting the secret absent. Treating that as absent would send the caller on
// to create a secret over one it could not read.
func TestEnsureSecret_PropagatesGetErrors(t *testing.T) {
	kubeClient := fakeKubeClient()
	kubeClient.PrependReactor("get", "secrets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, nil, kubeerrors.NewInternalError(assertError{})
	})

	_, err := ensureSecret(
		kubeClient, "app-abc123", "app-db", nil,
		map[string]string{"POSTGRES_PASSWORD": "first"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check for existing secret")
}

// TestEnsureSecret_PropagatesCreateErrors covers a create that fails for a
// reason other than the secret already existing.
func TestEnsureSecret_PropagatesCreateErrors(t *testing.T) {
	kubeClient := fakeKubeClient()
	kubeClient.PrependReactor("create", "secrets", func(kubetesting.Action) (bool, runtime.Object, error) {
		return true, nil, kubeerrors.NewForbidden(
			schema.GroupResource{Resource: "secrets"}, "app-db", assertError{},
		)
	})

	created, err := ensureSecret(
		kubeClient, "app-abc123", "app-db", nil,
		map[string]string{"POSTGRES_PASSWORD": "first"},
	)
	require.Error(t, err)
	assert.False(t, created)
	assert.Contains(t, err.Error(), "failed to create secret")
}

type assertError struct{}

func (assertError) Error() string { return "the api said no" }
