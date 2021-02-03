// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	iopv1alpha1 "istio.io/istio/operator/pkg/apis/istio/v1alpha1"
)

type JWTPolicy string

const (
	FirstPartyJWT JWTPolicy = "first-party-jwt"
	ThirdPartyJWT JWTPolicy = "third-party-jwt"
)

// DetectSupportedJWTPolicy queries the api-server to detect whether it has TokenRequest support
func DetectSupportedJWTPolicy(config *rest.Config) (JWTPolicy, error) {
	if config == nil {
		// this happens in unit tests- there's no such thing as a fake config
		// TODO(dgn): refactor to use Client instead of Config, so this can be faked
		return ThirdPartyJWT, nil
	}

	d, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return "", err
	}
	_, s, err := d.ServerGroupsAndResources()
	// This may fail if any api service is down. We should only fail if the specific API we care about failed
	if err != nil {
		if discovery.IsGroupDiscoveryFailedError(err) {
			derr := err.(*discovery.ErrGroupDiscoveryFailed)
			if _, f := derr.Groups[schema.GroupVersion{Group: "authentication.k8s.io", Version: "v1"}]; f {
				return "", err
			}
		} else {
			return "", err
		}
	}
	for _, res := range s {
		for _, api := range res.APIResources {
			// Appearance of this API indicates we do support third party jwt token
			if api.Name == "serviceaccounts/token" {
				return ThirdPartyJWT, nil
			}
		}
	}
	return FirstPartyJWT, nil
}

// GKString differs from default representation of GroupKind
func GKString(gvk schema.GroupKind) string {
	return fmt.Sprintf("%s/%s", gvk.Group, gvk.Kind)
}

// ValidateIOPCAConfig validates if the IstioOperator CA configs are applicable to the K8s cluster
func ValidateIOPCAConfig(client *kubernetes.Clientset, iop *iopv1alpha1.IstioOperator) error {
	globalI := iop.Spec.Values["global"]
	global, ok := globalI.(map[string]interface{})
	if !ok {
		// This means no explicit global configuration. Still okay
		return nil
	}
	ca, ok := global["pilotCertProvider"].(string)
	if !ok {
		// This means the default pilotCertProvider is being used
		return nil
	}
	if ca == "kubernetes" {
		versionInfo, err := client.Discovery().ServerVersion()
		if err != nil {
			return fmt.Errorf("failed to determine support for K8s legacy signer. Use the --force flag to ignore this: %v", err)
		}
		minor, _ := strconv.Atoi(versionInfo.Minor)
		major, _ := strconv.Atoi(versionInfo.Major)

		if minor >= 22 || major > 1 {
			return fmt.Errorf("configuration PILOT_CERT_PROVIDER=%s not supported in k8s minor version %v."+
				"Please pick another value for PILOT_CERT_PROVIDER", ca, minor)
		}
	}
	return nil
}
