# Patches

## `000-bundle-extra-images.patch`

Iternal patch which adds `libguestfs`, `virt-exportserver` and `virt-exportproxy`
to images bundle target.

## `001-allow-specify-image.patch`

Allows specifying exact image names for components

- https://github.com/kubevirt/kubevirt/pull/8390

## `002-serviceaccount.patch`

Internal patch which adds deckhouse ImagePullSecrets to kubevirt generated ServiceAccounts

- https://github.com/kubevirt/kubevirt/issues/8302

## `003-network-aware-livemigration.patch`

Allow live-migration for pod network in bridge mode

- https://github.com/kubevirt/kubevirt/pull/7768

## `004-macvtap-binding.patch`

Macvtap binding mode for pod network

- https://github.com/kubevirt/kubevirt/pull/7648

## `005-patch-serviceaccount.patch`

Allow patching ServiceAccounts via customizeComponents feature
