package v0

import (
	"fmt"
	"net/http"

	tpapi_v0 "github.com/threeport/threeport/pkg/api/v0"
	tpclient_v0 "github.com/threeport/threeport/pkg/client/v0"
	tpconfig_v0 "github.com/threeport/threeport/pkg/config/v0"
)

// getKubernetesRuntimeInstanceByNameOrDefault returns the Kubernetes runtime a
// config names, or the control plane's default when it names none.
//
// Threeport has this helper in pkg/config/v0 but does not export it, so it is
// repeated here rather than leaving every caller to resolve the default itself.
// The values type is Threeport's, so a config file names a runtime the same way
// it does for a workload.
func getKubernetesRuntimeInstanceByNameOrDefault(
	apiClient *http.Client,
	apiEndpoint string,
	values *tpconfig_v0.KubernetesRuntimeInstanceValues,
) (*tpapi_v0.KubernetesRuntimeInstance, error) {
	if values != nil && values.Name != nil {
		kubernetesRuntimeInstance, err := tpclient_v0.GetKubernetesRuntimeInstanceByName(
			apiClient,
			apiEndpoint,
			*values.Name,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get kubernetes runtime instance with name %s: %w",
				*values.Name, err,
			)
		}

		return kubernetesRuntimeInstance, nil
	}

	kubernetesRuntimeInstance, err := tpclient_v0.GetDefaultKubernetesRuntimeInstance(
		apiClient,
		apiEndpoint,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"no kubernetes runtime instance named in config and failed to get the default: %w",
			err,
		)
	}

	return kubernetesRuntimeInstance, nil
}
