// Command deploy creates a Django definition and instance through the module's
// client library.
//
// It exists because the config abstractions in pkg/config are still the SDK
// scaffold, so `tptctl django create django-definition -c config.yaml` cannot
// express the real fields yet. Once those are filled in this program becomes
// redundant.
//
// Usage:
//
//	go run ./examples/deploy -name myapp -image myorg/myapp:v1
//	go run ./examples/deploy -name myapp -delete
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	tpapi "github.com/threeport/threeport/pkg/api/v0"
	cli "github.com/threeport/threeport/pkg/cli/v0"
	util "github.com/threeport/threeport/pkg/util/v0"

	v0 "django-threeport-module/pkg/api/v0"
	client_v0 "django-threeport-module/pkg/client/v0"
)

func main() {
	name := flag.String("name", "", "name for the django definition and instance")
	image := flag.String("image", "", "container image for the django application")
	settings := flag.String("settings", "", "value for DJANGO_SETTINGS_MODULE")
	environment := flag.String("environment", "dev", "environment type, drives replica and storage defaults")
	remove := flag.Bool("delete", false, "delete the named definition and instance instead of creating them")
	flag.Parse()

	if *name == "" {
		fmt.Fprintln(os.Stderr, "-name is required")
		os.Exit(1)
	}
	if !*remove && *image == "" {
		fmt.Fprintln(os.Stderr, "-image is required when creating")
		os.Exit(1)
	}

	cli.InitConfig(nil, "")
	cfg, _, err := cli.GetThreeportConfig("")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read threeport config:", err)
		os.Exit(1)
	}
	apiClient, err := cfg.GetHTTPClient(cfg.CurrentControlPlane)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to build api client:", err)
		os.Exit(1)
	}
	controlPlane, err := cfg.GetControlPlaneConfig(cfg.CurrentControlPlane)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to read control plane config:", err)
		os.Exit(1)
	}
	endpoint := controlPlane.APIServer

	if *remove {
		if err := deleteByName(apiClient, endpoint, *name); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	definition := &v0.DjangoDefinition{
		Definition:  tpapi.Definition{Name: name},
		Image:       image,
		Environment: environment,
	}
	if *settings != "" {
		definition.SettingsModule = settings
	}

	created, err := client_v0.CreateDjangoDefinition(apiClient, endpoint, definition)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create django definition:", err)
		os.Exit(1)
	}
	fmt.Printf("created django definition %s\n", *created.Name)

	instance, err := client_v0.CreateDjangoInstance(apiClient, endpoint, &v0.DjangoInstance{
		Instance:           tpapi.Instance{Name: util.Ptr(*name)},
		DjangoDefinitionID: created.ID,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to create django instance:", err)
		os.Exit(1)
	}
	fmt.Printf("created django instance %s\n", *instance.Name)
}

// deleteByName removes the instance before the definition: the API refuses to
// delete a definition that still has instances, and the instance's own removal
// is asynchronous.
func deleteByName(apiClient *http.Client, endpoint string, name string) error {
	instances, err := client_v0.GetDjangoInstances(apiClient, endpoint)
	if err != nil {
		return fmt.Errorf("failed to list django instances: %w", err)
	}
	for _, instance := range *instances {
		if *instance.Name != name {
			continue
		}
		if _, err := client_v0.DeleteDjangoInstance(apiClient, endpoint, *instance.ID); err != nil {
			return fmt.Errorf("failed to delete django instance %s: %w", name, err)
		}
		fmt.Printf("deleted django instance %s\n", name)
	}

	definitions, err := client_v0.GetDjangoDefinitions(apiClient, endpoint)
	if err != nil {
		return fmt.Errorf("failed to list django definitions: %w", err)
	}
	for _, definition := range *definitions {
		if *definition.Name != name {
			continue
		}
		if _, err := client_v0.DeleteDjangoDefinition(apiClient, endpoint, *definition.ID); err != nil {
			return fmt.Errorf(
				"failed to delete django definition %s, the instance may still be terminating: %w",
				name, err,
			)
		}
		fmt.Printf("deleted django definition %s\n", name)
	}

	return nil
}
