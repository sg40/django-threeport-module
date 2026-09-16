package v0

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	util "github.com/threeport/threeport/pkg/util/v0"
	yaml "sigs.k8s.io/yaml"
)

// TestDjangoDefinitionConfig_Validate covers the fields a user can get wrong in
// a config file. Each one is otherwise rejected far later: Image by a database
// constraint, Environment and Replicas by the kube API once Threeport tries to
// apply the manifest, long after the definition was accepted.
func TestDjangoDefinitionConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		values  DjangoDefinitionValues
		wantErr string
	}{
		{
			name:   "a name and an image are enough",
			values: DjangoDefinitionValues{Name: util.Ptr("myapp"), Image: util.Ptr("myorg/myapp:v1")},
		},
		{
			name:    "an image is required",
			values:  DjangoDefinitionValues{Name: util.Ptr("myapp")},
			wantErr: "Image",
		},
		{
			name:    "a name is required",
			values:  DjangoDefinitionValues{Image: util.Ptr("myorg/myapp:v1")},
			wantErr: "Name",
		},
		{
			name: "the environment has to be usable as a label value",
			values: DjangoDefinitionValues{
				Name:        util.Ptr("myapp"),
				Image:       util.Ptr("myorg/myapp:v1"),
				Environment: util.Ptr("staging/eu-west"),
			},
			wantErr: "Environment",
		},
		{
			name: "an ordinary environment is accepted",
			values: DjangoDefinitionValues{
				Name:        util.Ptr("myapp"),
				Image:       util.Ptr("myorg/myapp:v1"),
				Environment: util.Ptr("prod"),
			},
		},
		{
			name: "replicas cannot be negative",
			values: DjangoDefinitionValues{
				Name:     util.Ptr("myapp"),
				Image:    util.Ptr("myorg/myapp:v1"),
				Replicas: util.Ptr(-1),
			},
			wantErr: "Replicas",
		},
		{
			name: "zero replicas is a legitimate way to scale down",
			values: DjangoDefinitionValues{
				Name:     util.Ptr("myapp"),
				Image:    util.Ptr("myorg/myapp:v1"),
				Replicas: util.Ptr(0),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DjangoDefinitionConfig{DjangoDefinition: test.values}
			err := config.Validate()
			if test.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

// TestDjangoInstanceConfig_Validate covers the definition reference. Create
// dereferences DjangoDefinition.Name to look the definition up, so a config
// without one panics rather than reporting a missing field.
func TestDjangoInstanceConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		values  DjangoInstanceValues
		wantErr string
	}{
		{
			name: "a name and a definition are enough",
			values: DjangoInstanceValues{
				Name:             util.Ptr("myapp"),
				DjangoDefinition: &DjangoDefinitionValues{Name: util.Ptr("myapp")},
			},
		},
		{
			name:    "the definition is required",
			values:  DjangoInstanceValues{Name: util.Ptr("myapp")},
			wantErr: "DjangoDefinition.Name",
		},
		{
			name: "a definition without a name is no better than none",
			values: DjangoInstanceValues{
				Name:             util.Ptr("myapp"),
				DjangoDefinition: &DjangoDefinitionValues{},
			},
			wantErr: "DjangoDefinition.Name",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := DjangoInstanceConfig{DjangoInstance: test.values}
			err := config.Validate()
			if test.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

// TestMapToDjangoDefinedInstances covers the pairing a defined instance rests
// on, and that it carries the attributes from both halves rather than the name
// alone.
func TestMapToDjangoDefinedInstances(t *testing.T) {
	definitions := []DjangoDefinitionConfig{
		{DjangoDefinition: DjangoDefinitionValues{
			Name:           util.Ptr("myapp"),
			Image:          util.Ptr("myorg/myapp:v1"),
			SettingsModule: util.Ptr("myapp.settings"),
			Environment:    util.Ptr("prod"),
			Replicas:       util.Ptr(3),
			RunMigrations:  util.Ptr(true),
		}},
		{DjangoDefinition: DjangoDefinitionValues{Name: util.Ptr("other")}},
	}
	instances := []DjangoInstanceConfig{
		{DjangoInstance: DjangoInstanceValues{
			Name:             util.Ptr("myapp"),
			SubDomain:        util.Ptr("www"),
			DjangoDefinition: &DjangoDefinitionValues{Name: util.Ptr("myapp")},
			Age:              util.Ptr("2d"),
		}},
	}

	configs := mapToDjangoDefinedInstances(&definitions, &instances)
	require.Len(t, *configs, 1, "only the instance whose definition shares its name is a defined instance")

	values := (*configs)[0].Django
	assert.Equal(t, "myapp", *values.Name)
	assert.Equal(t, "myorg/myapp:v1", *values.Image, "the definition's attributes have to survive the mapping")
	assert.Equal(t, "prod", *values.Environment)
	assert.Equal(t, 3, *values.Replicas)
	assert.Equal(t, "www", *values.SubDomain, "the instance's attributes have to survive it too")
	assert.Equal(t, "2d", *values.Age)
}

// TestMapToDjangoDefinedInstances_SkipsIncompleteInstances covers the
// dereferences the pairing does. An instance missing a name or a definition
// reference used to panic here rather than be skipped.
func TestMapToDjangoDefinedInstances_SkipsIncompleteInstances(t *testing.T) {
	definitions := []DjangoDefinitionConfig{
		{DjangoDefinition: DjangoDefinitionValues{Name: util.Ptr("myapp")}},
		{DjangoDefinition: DjangoDefinitionValues{}},
	}
	instances := []DjangoInstanceConfig{
		{DjangoInstance: DjangoInstanceValues{Name: nil}},
		{DjangoInstance: DjangoInstanceValues{Name: util.Ptr("myapp")}},
		{DjangoInstance: DjangoInstanceValues{
			Name:             util.Ptr("myapp"),
			DjangoDefinition: &DjangoDefinitionValues{},
		}},
	}

	assert.NotPanics(t, func() {
		configs := mapToDjangoDefinedInstances(&definitions, &instances)
		assert.Empty(t, *configs, "none of these instances is half of a defined instance")
	})
}

// TestSampleConfigsParse covers the files under samples/. The CLI unmarshals
// strictly, so a field the values structs do not carry is an error rather than
// something quietly ignored — which means a sample can go stale the moment a
// field is renamed, and the user is the one who finds out.
func TestSampleConfigsParse(t *testing.T) {
	tests := []struct {
		file   string
		into   interface{}
		assert func(t *testing.T, into interface{})
	}{
		{
			file: "django.yaml",
			into: &DjangoConfig{},
			assert: func(t *testing.T, into interface{}) {
				values := into.(*DjangoConfig).Django
				require.NotNil(t, values.Name)
				require.NotNil(t, values.Image, "a sample without an image would not pass Validate")
			},
		},
		{
			file: "django-definition.yaml",
			into: &DjangoDefinitionConfig{},
			assert: func(t *testing.T, into interface{}) {
				values := into.(*DjangoDefinitionConfig).DjangoDefinition
				require.NotNil(t, values.Name)
				require.NotNil(t, values.Image)
				require.NotNil(t, values.Replicas)
				require.NotNil(t, values.RunMigrations)
			},
		},
		{
			file: "django-instance.yaml",
			into: &DjangoInstanceConfig{},
			assert: func(t *testing.T, into interface{}) {
				values := into.(*DjangoInstanceConfig).DjangoInstance
				require.NotNil(t, values.Name)
				require.NotNil(t, values.DjangoDefinition)
				require.Nil(
					t, values.KubernetesRuntimeInstance,
					"the sample documents omitting the runtime to get the default; setting one would contradict its own comment",
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", "..", "samples", test.file))
			require.NoError(t, err)
			require.NoError(t, yaml.UnmarshalStrict(content, test.into), "the CLI would reject this file")
			test.assert(t, test.into)
		})
	}
}
