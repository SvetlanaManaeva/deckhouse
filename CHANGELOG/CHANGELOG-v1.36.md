# Changelog v1.36

## Know before update


 - A node in the `master` nodeGroup with a single node and `Automatic` disruption approval mode will not be drained before approval.
    If Deckhouse works not on a master node and this node is single (or one node in Ready status) in the nodeGroup, and for this nodeGroup the `Automatic` disruption approval mode is set, then disruption operations will be approved for this node without draining.
 - All ingress nginx controllers with not-specified version (0.33) will restart and upgrade to 1.1
 - All of the controllers will be restarted.
 - Etcd pods' restart, triggering leader elections and affecting Kubernetes API performance until all etcd pods use new configuration.
 - Migrate to the Kubernetes version v1.20+ to upgrade to the current Deckhouse release.
 - Updating the patch version of istio will cause the `D8IstioDataPlaneVersionMismatch` alert to appear. To make the alert disappear, you must recreate all workloads so that the sidecar runs with the current version.

## Features


 - **[candi]** Add an `additionalRolePolicies` parameter to AWSClusterConfiguration (#1005) [#2256](https://github.com/deckhouse/deckhouse/pull/2256)
 - **[candi]** Added support for Rocky Linux `9.0`. [#2232](https://github.com/deckhouse/deckhouse/pull/2232)
 - **[candi]** Add support for the kubernetes 1.24 version [#2210](https://github.com/deckhouse/deckhouse/pull/2210)
 - **[candi]** Set `maxAllowed` and `minAllowed` to all VPA objects. Set resources requests for all controllers if VPA is off.   Added `global.modules.resourcesRequests.controlPlane` values. `global.modules.resourcesRequests.EveryNode` and `global.modules.resourcesRequests.masterNode` values are deprecated. [#1918](https://github.com/deckhouse/deckhouse/pull/1918)
    All of the controllers will be restarted.
 - **[ingress-nginx]** Change default ingress nginx controller version to 1.1 [#2267](https://github.com/deckhouse/deckhouse/pull/2267)
    All ingress nginx controllers with not-specified version (0.33) will restart and upgrade to 1.1
 - **[linstor]** Automate kernel headers installation [#2287](https://github.com/deckhouse/deckhouse/pull/2287)
 - **[log-shipper]** Refactor transforms composition, improve efficiency and fix destination transforms. [#2050](https://github.com/deckhouse/deckhouse/pull/2050)
 - **[monitoring-kubernetes]** New Capacity Planning dashboard. [#2365](https://github.com/deckhouse/deckhouse/pull/2365)
 - **[monitoring-kubernetes]** Add GRPC request handling time and etcd peer RTT graphs to etcd dashboard [#2360](https://github.com/deckhouse/deckhouse/pull/2360)
    Etcd pods' restart, triggering leader elections and affecting Kubernetes API performance until all etcd pods use new configuration.
 - **[monitoring-kubernetes]** Add nodes count panel to the Nodes dashboard. [#2196](https://github.com/deckhouse/deckhouse/pull/2196)
 - **[node-manager]** Switched early-oom to PSI metrics [#2358](https://github.com/deckhouse/deckhouse/pull/2358)
 - **[prometheus]** Create mTLS secret to scrape metrics from workloads with PeerAuthentication mtls.mode = STRICT. [#2332](https://github.com/deckhouse/deckhouse/pull/2332)

## Fixes


 - **[dhctl]** Fail if there is empty host for ssh connection [#2346](https://github.com/deckhouse/deckhouse/pull/2346)
 - **[ingress-nginx]** Improve metrics collection script [#2350](https://github.com/deckhouse/deckhouse/pull/2350)
 - **[ingress-nginx]** Change defaultControllerVersion without deckhouse reboot [#2338](https://github.com/deckhouse/deckhouse/pull/2338)
 - **[istio]** maxUnavailable strategy for istiod Deployment instead of default one 25%. [#2202](https://github.com/deckhouse/deckhouse/pull/2202)
 - **[linstor]** Bump DRBD version to `9.1.9`. [#2359](https://github.com/deckhouse/deckhouse/pull/2359)
 - **[linstor]** Change module order. [#2323](https://github.com/deckhouse/deckhouse/pull/2323)
 - **[log-shipper]** Rewrite Elasticsearch dedot rule in VRL to improve performance. [#2192](https://github.com/deckhouse/deckhouse/pull/2192)
 - **[log-shipper]** Prevent Vector from stopping logs processing if Kubernetes API server was restarted. [#2192](https://github.com/deckhouse/deckhouse/pull/2192)
 - **[log-shipper]** Fix memory leak for internal metrics. [#2192](https://github.com/deckhouse/deckhouse/pull/2192)
 - **[monitoring-kubernetes]** Change steppedLine to false for CPU panels and add sorting. [#2371](https://github.com/deckhouse/deckhouse/pull/2371)
 - **[node-manager]** Do not drain single-master and single standalone nodes where Deckhouse works with automatic approve mode for disruption. [#2386](https://github.com/deckhouse/deckhouse/pull/2386)
    A node in the `master` nodeGroup with a single node and `Automatic` disruption approval mode will not be drained before approval.
    If Deckhouse works not on a master node and this node is single (or one node in Ready status) in the nodeGroup, and for this nodeGroup the `Automatic` disruption approval mode is set, then disruption operations will be approved for this node without draining.
 - **[node-manager]** Change cluster autoscaler timeouts to avoid node flapping [#2279](https://github.com/deckhouse/deckhouse/pull/2279)
 - **[upmeter]** Bundled CSS into status page for desired look in restricted environments [#2349](https://github.com/deckhouse/deckhouse/pull/2349)

## Chore


 - **[candi]** Remove Kubernetes version 1.19 support [#2255](https://github.com/deckhouse/deckhouse/pull/2255)
    Migrate to the Kubernetes version v1.20+ to upgrade to the current Deckhouse release.
 - **[cni-cilium]** cilium various fixes [#2252](https://github.com/deckhouse/deckhouse/pull/2252)
    cilium-agent restart
 - **[flant-integration]** Added new distros supported by Deckhouse. [#2284](https://github.com/deckhouse/deckhouse/pull/2284)
 - **[istio]** bump istio version from 1.13.3 to 1.13.7 [#2400](https://github.com/deckhouse/deckhouse/pull/2400)
    Updating the patch version of istio will cause the `D8IstioDataPlaneVersionMismatch` alert to appear. To make the alert disappear, you must recreate all workloads so that the sidecar runs with the current version.
 - **[istio]** Added `D8IstioDeprecatedIstioVersionInstalled` alert for depricated istio versions. [#2389](https://github.com/deckhouse/deckhouse/pull/2389)
 - **[istio]** refactor istio revision monitoring [#2273](https://github.com/deckhouse/deckhouse/pull/2273)
 - **[log-shipper]** Update Vector to 0.23 [#2192](https://github.com/deckhouse/deckhouse/pull/2192)
 - **[monitoring-kubernetes]** Bump kube-state-metrics 2.6.0 [#2291](https://github.com/deckhouse/deckhouse/pull/2291)

