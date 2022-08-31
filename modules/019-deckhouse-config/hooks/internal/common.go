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

package internal

import (
	"fmt"

	"github.com/flant/addon-operator/pkg/module_manager/go_hook"

	d8config_v1 "github.com/deckhouse/deckhouse/go_lib/deckhouse-config/v1"
	"github.com/deckhouse/deckhouse/go_lib/set"
)

func SetFromArrayValue(input *go_hook.HookInput, path string) (set.Set, error) {
	value, ok := input.Values.GetOk(path)
	if !ok {
		return nil, fmt.Errorf("%s value is required", path)
	}
	list := value.Array()
	if len(list) == 0 {
		return nil, fmt.Errorf("%s value should not be empty", path)
	}
	return set.NewFromValues(input.Values, path), nil
}

func KnownConfigsFromSnapshot(snapshot []go_hook.FilterResult, possibleNames set.Set) []*d8config_v1.DeckhouseConfig {
	configs := make([]*d8config_v1.DeckhouseConfig, 0)
	for _, item := range snapshot {
		cfg := item.(*d8config_v1.DeckhouseConfig)
		// Ignore unknown names.
		if !possibleNames.Has(cfg.GetName()) {
			continue
		}
		configs = append(configs, cfg)
	}
	return configs
}

func ConfigsFromSnapshot(snapshot []go_hook.FilterResult) []*d8config_v1.DeckhouseConfig {
	configs := make([]*d8config_v1.DeckhouseConfig, 0)
	for _, item := range snapshot {
		cfg := item.(*d8config_v1.DeckhouseConfig)
		configs = append(configs, cfg)
	}
	return configs
}

// MergeEnabled merges enabled flags. Enabled flag can be nil.
//
// If all flags are nil, then false is returned — module is disabled by default.
// Note: copy-paste from AddonOperator.ModuleManager
func MergeEnabled(enabledFlags ...*bool) bool {
	result := false
	for _, enabled := range enabledFlags {
		if enabled == nil {
			continue
		} else {
			result = *enabled
		}
	}

	return result
}
