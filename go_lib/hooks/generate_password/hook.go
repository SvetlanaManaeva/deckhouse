/*
Copyright 2021 Flant JSC

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

package generate_password

import (
	"fmt"
	"strings"

	"github.com/flant/addon-operator/pkg/module_manager/go_hook"
	addonutils "github.com/flant/addon-operator/pkg/utils"
	"github.com/flant/addon-operator/sdk"
	"github.com/flant/shell-operator/pkg/kube_events_manager/types"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/deckhouse/deckhouse/go_lib/pwgen"
)

const (
	secretBindingName          = "password_secret"
	defaultBasicAuthPlainField = "auth"
	defaultBeforeHelmOrder     = 10
)

func NewBasicAuthPlainHook(moduleValuesPath string, ns string, secretName string) *Hook {
	// Ensure camelCase for moduleValuesPath
	valuesKey := addonutils.ModuleNameToValuesKey(moduleValuesPath)
	return &Hook{
		Secret: Secret{
			Namespace: ns,
			Name:      secretName,
		},
		ValuesKey: valuesKey,
	}
}

// RegisterHook returns func to register common hook that generates
// and stores a password in the Secret.
func RegisterHook(moduleValuesPath string, ns string, secretName string) bool {
	hook := NewBasicAuthPlainHook(moduleValuesPath, ns, secretName)
	return sdk.RegisterFunc(&go_hook.HookConfig{
		Queue: fmt.Sprintf("/modules/%s/generate_password", hook.ValuesKey),
		Kubernetes: []go_hook.KubernetesConfig{
			{
				Name:       secretBindingName,
				ApiVersion: "v1",
				Kind:       "Secret",
				NameSelector: &types.NameSelector{
					MatchNames: []string{hook.Secret.Name},
				},
				NamespaceSelector: &types.NamespaceSelector{
					NameSelector: &types.NameSelector{
						MatchNames: []string{hook.Secret.Namespace},
					},
				},
				// Synchronization is redundant because of OnBeforeHelm.
				ExecuteHookOnSynchronization: go_hook.Bool(false),
				ExecuteHookOnEvents:          go_hook.Bool(false),
				FilterFunc:                   hook.Filter,
			},
		},
		OnBeforeHelm: &go_hook.OrderedConfig{Order: float64(defaultBeforeHelmOrder)},
	}, hook.Handle)
}

type Hook struct {
	Secret    Secret
	ValuesKey string
}

type Secret struct {
	Namespace string
	Name      string
}

// Filter extracts password from the Secret. Password can be stored as a raw string or as
// a basic auth plain format (user:{PLAIN}password). Custom FilterFunc is called for custom
// password extraction.
func (h *Hook) Filter(obj *unstructured.Unstructured) (go_hook.FilterResult, error) {
	secret := &v1.Secret{}
	err := sdk.FromUnstructured(obj, secret)
	if err != nil {
		return nil, fmt.Errorf("cannot convert secret to struct: %v", err)
	}

	// Return field with basic auth.
	return string(secret.Data[defaultBasicAuthPlainField]), nil
}

// Handle restores password from the configuration or from the Secret and
// puts it to internal values.
// It generates new password if no password found in the configuration and the
// Secret or no externalAuthentication defined.
func (h *Hook) Handle(input *go_hook.HookInput) error {
	externalAuthKey := h.ExternalAuthKey()
	passwordKey := h.PasswordKey()
	passwordInternalKey := h.PasswordInternalKey()

	// Clear password from internal values if an external authentication is enabled.
	if input.Values.Exists(externalAuthKey) {
		input.Values.Remove(passwordInternalKey)
		return nil
	}

	// Try to set password from config values.
	password, ok := input.ConfigValues.GetOk(passwordKey)
	if ok {
		input.Values.Set(passwordInternalKey, password.String())
		return nil
	}

	// Try to restore password from the Secret.
	snap := input.Snapshots[secretBindingName]
	if len(snap) > 0 {
		secretField, ok := snap[0].(string)
		if !ok {
			return fmt.Errorf("problem getting field '%s' from Secret/%s: got %T, while string is expected", defaultBasicAuthPlainField, h.Secret.Name, snap[0])
		}
		storedPassword, err := h.extractPasswordFromBasicAuth(secretField)
		if err != nil {
			return err
		}
		input.Values.Set(passwordInternalKey, storedPassword)
		return nil
	}

	// No config value, no Secret, generate new password.
	input.Values.Set(passwordInternalKey, GeneratePassword())
	return nil
}

const (
	externalAuthKeyTmpl     = "%s.auth.externalAuthentication"
	passwordKeyTmpl         = "%s.auth.password"
	passwordInternalKeyTmpl = "%s.internal.auth.password"
)

func (h *Hook) ExternalAuthKey() string {
	return fmt.Sprintf(externalAuthKeyTmpl, h.ValuesKey)
}

func (h *Hook) PasswordKey() string {
	return fmt.Sprintf(passwordKeyTmpl, h.ValuesKey)
}

func (h *Hook) PasswordInternalKey() string {
	return fmt.Sprintf(passwordInternalKeyTmpl, h.ValuesKey)
}

// extractPasswordFromBasicAuth extracts password from the plain basic auth string:
// username:{PLAIN}password
func (h *Hook) extractPasswordFromBasicAuth(basicAuth string) (string, error) {
	parts := strings.SplitN(basicAuth, "{PLAIN}", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("field '%s' in Secret/%s is not a basic auth plain password", defaultBasicAuthPlainField, h.Secret.Name)
	}

	return strings.TrimSpace(parts[1]), nil
}

func GeneratePassword() string {
	return pwgen.AlphaNum(20)
}
