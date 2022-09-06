
if [ $# -ne 1 ]; then
cat <<EOT
Usage: hack/update.sh v0.56.0
EOT
exit 1
fi

manifest=$(mktemp)

curl -LfsS "https://github.com/kubevirt/kubevirt/releases/download/$1/kubevirt-operator.yaml" -o "$manifest"
awk -v RS="\n---\n" '/\nkind: CustomResourceDefinition\n/ {print "---\n" $0}' "$manifest"  > crds/kubevirt.yaml

{
  awk -v RS='\n---\n' '/\nkind: ClusterRole\n.*\n  name: kubevirt.io:operator\n/ {print "---\n" $0}' "$manifest"
  awk -v RS='\n---\n' '/\nkind: ServiceAccount\n/ {print "---\n" $0}' "$manifest"
  printf "%s\n" "imagePullSecrets:" "- name: deckhouse-registry"
  awk -v RS='\n---\n' '/\nkind: Role\n/ {print "---\n" $0}' "$manifest"
  awk -v RS='\n---\n' '/\nkind: RoleBinding\n/ {print "---\n" $0}' "$manifest"
  awk -v RS='\n---\n' '/\nkind: ClusterRole\n.*\n  name: kubevirt-operator\n/ {print "---\n" $0}' "$manifest"
  awk -v RS='\n---\n' '/\nkind: ClusterRoleBinding\n/ {print "---\n" $0}' "$manifest"
} > templates/operator/rbac-for-us.yaml

sed -i 's/namespace: kubevirt/namespace: d8-kubevirt/g' templates/operator/rbac-for-us.yaml
sed -zi 's/  labels:\n\(    [^\n]*\n\)\+/  {{- include "helm_lib_module_labels" (list .) | nindent 2 }}\n/g' templates/operator/rbac-for-us.yaml
