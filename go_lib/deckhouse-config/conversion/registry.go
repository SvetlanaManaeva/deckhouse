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

package conversion

import (
	"sync"
)

/*
Conversion package is used to support older values layouts.

Module should define conversion functions and register them in conversion
Registry. Conversion webhook will use these functions to convert values in
DeckhouseConfig objects to latest version.
*/

var instance *ConvRegistry
var once sync.Once

func Registry() *ConvRegistry {
	once.Do(func() {
		instance = new(ConvRegistry)
	})
	return instance
}

// Register adds Conversion implementation to Registry. Returns true to use with "var _ =".
func Register(moduleName string, conversion Conversion) bool {
	Registry().Add(moduleName, conversion)
	return true
}

// RegisterFunc adds a function as a Conversion to Registry. Returns true to use with "var _ =".
func RegisterFunc(moduleName string, srcVersion string, targetVersion string, conversionFunc ConversionFunc) bool {
	Registry().Add(moduleName, NewAnonymousConversion(srcVersion, targetVersion, conversionFunc))
	return true
}

type ConvRegistry struct {
	// module name -> module chain
	chains map[string]*ModuleChain

	m sync.RWMutex
}

func (r *ConvRegistry) Add(moduleName string, conversion Conversion) {
	r.m.Lock()
	defer r.m.Unlock()

	if r.chains == nil {
		r.chains = make(map[string]*ModuleChain)
	}
	if _, has := r.chains[moduleName]; !has {
		r.chains[moduleName] = NewModuleChain(moduleName)
	}

	r.chains[moduleName].Add(conversion)
}

func (r *ConvRegistry) Chain(moduleName string) *ModuleChain {
	r.m.RLock()
	defer r.m.RUnlock()

	return r.chains[moduleName]
}

// HasChain returns whether module has registered conversions.
func (r *ConvRegistry) HasChain(moduleName string) bool {
	r.m.RLock()
	defer r.m.RUnlock()

	_, has := r.chains[moduleName]
	return has
}
