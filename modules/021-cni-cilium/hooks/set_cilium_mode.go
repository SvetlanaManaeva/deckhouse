/*
Copyright 2022 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/flant/addon-operator/pkg/module_manager/go_hook"
	"github.com/flant/addon-operator/sdk"
	"github.com/flant/shell-operator/pkg/kube_events_manager/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type CiliumConfig struct {
	Mode string `json:"mode"`
}

func applyCNIConfigurationSecretFilter(obj *unstructured.Unstructured) (go_hook.FilterResult, error) {
	secret := &v1.Secret{}
	err := sdk.FromUnstructured(obj, secret)
	if err != nil {
		return nil, fmt.Errorf("cannot convert incoming object to Secret: %v", err)
	}

	if string(secret.Data["cni"]) != "cilium" {
		return nil, nil
	}

	var ciliumConfig CiliumConfig
	ciliumConfigJSON, ok := secret.Data["cilium"]
	if !ok {
		return nil, nil
	}

	err = json.Unmarshal(ciliumConfigJSON, &ciliumConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot unmarshal cilium config json: %v", err)
	}
	return ciliumConfig, nil
}

var _ = sdk.RegisterFunc(&go_hook.HookConfig{
	OnBeforeHelm: &go_hook.OrderedConfig{Order: 10},
	Queue:        "/modules/cni-cilium",
	Kubernetes: []go_hook.KubernetesConfig{
		{
			Name:       "cni_configuration_secret",
			ApiVersion: "v1",
			Kind:       "Secret",
			NamespaceSelector: &types.NamespaceSelector{
				NameSelector: &types.NameSelector{
					MatchNames: []string{"kube-system"},
				},
			},
			NameSelector: &types.NameSelector{
				MatchNames: []string{"d8-cni-configuration"},
			},
			FilterFunc: applyCNIConfigurationSecretFilter,
		},
	},
}, setCiliumMode)

func setCiliumMode(input *go_hook.HookInput) error {
	// if secret exists, use it
	cniConfigurationSecrets, ok := input.Snapshots["cni_configuration_secret"]

	if ok && len(cniConfigurationSecrets) > 0 {
		if cniConfigurationSecrets[0] != nil {
			ciliumConfig := cniConfigurationSecrets[0].(CiliumConfig)
			input.Values.Set("cniCilium.internal.mode", ciliumConfig.Mode)
			return nil
		}
	}

	if input.ConfigValues.Exists("cniCilium.tunnelMode") {
		if input.ConfigValues.Get("cniCilium.tunnelMode").String() == "VXLAN" {
			input.Values.Set("cniCilium.internal.mode", "VXLAN")
		}
	}

	value, ok := input.ConfigValues.GetOk("cniCilium.createNodeRoutes")
	if ok && value.Bool() {
		input.Values.Set("cniCilium.internal.mode", "DirectWithNodeRoutes")
	}

	// default = Direct
	return nil
}
