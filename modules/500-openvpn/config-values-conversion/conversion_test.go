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

package config_values_conversion

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	. "github.com/deckhouse/deckhouse/testing/conversion"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "")
}

var _ = Describe("Module :: openvpn :: config values conversions :: v0.0.0", func() {
	f := SetupConverter(``)

	const migratedValues = `
inlet: ExternalIP
hostPort: 2222
`
	Context("config values already migrated", func() {
		BeforeEach(func() {
			f.ValuesSetFromYaml(".", migratedValues)
			f.Convert("v0.0.0")
		})

		It("should convert", func() {
			Expect(f.Error).ShouldNot(HaveOccurred())
			Expect(f.FinalVersion).Should(Equal("v1.0.0"))
			Expect(f.FinalValues.Get("storageClass").String()).Should(BeEmpty())
		})
	})

	const nonMigratedValues = `
inlet: ExternalIP
hostPort: 2222
storageClass: default
`
	Context("config values are non migrated", func() {
		BeforeEach(func() {
			f.ValuesSetFromYaml(".", nonMigratedValues)
			f.Convert("v0.0.0")
		})

		It("should convert", func() {
			Expect(f.Error).ShouldNot(HaveOccurred())
			Expect(f.FinalVersion).Should(Equal("v1.0.0"))
			Expect(f.FinalValues.Get("storageClass").String()).Should(BeEmpty())
		})
	})
})

// Test older values conversion to latest version.
var _ = Describe("Module :: openvpn :: config values conversions :: to latest", func() {
	f := SetupConverter(``)

	Context("v0.0.0", func() {
		const v0Values = `
inlet: ExternalIP
hostPort: 2222
storageClass: default
`

		BeforeEach(func() {
			f.ValuesSetFromYaml(".", v0Values)
			f.ConvertToLatest("v0.0.0")
		})

		It("should convert", func() {
			Expect(f.Error).ShouldNot(HaveOccurred())
		})
	})
})
