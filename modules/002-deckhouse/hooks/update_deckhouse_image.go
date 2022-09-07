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

package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/flant/addon-operator/pkg/module_manager/go_hook"
	"github.com/flant/addon-operator/pkg/module_manager/go_hook/metrics"
	"github.com/flant/addon-operator/sdk"
	"github.com/flant/shell-operator/pkg/kube/object_patch"
	"github.com/flant/shell-operator/pkg/kube_events_manager/types"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/pointer"

	"github.com/deckhouse/deckhouse/go_lib/dependency"
	"github.com/deckhouse/deckhouse/go_lib/dependency/cr"
	"github.com/deckhouse/deckhouse/go_lib/dependency/requirements"
	"github.com/deckhouse/deckhouse/go_lib/hooks/update"
	"github.com/deckhouse/deckhouse/modules/002-deckhouse/hooks/internal/v1alpha1"
)

var _ = sdk.RegisterFunc(&go_hook.HookConfig{
	Queue: "/modules/deckhouse/update_deckhouse_image",
	Schedule: []go_hook.ScheduleConfig{
		{
			Name:    "update_deckhouse_image",
			Crontab: "*/15 * * * * *",
		},
	},
	Settings: &go_hook.HookConfigSettings{
		EnableSchedulesOnStartup: true,
	},
	Kubernetes: []go_hook.KubernetesConfig{
		{
			Name:       "deckhouse_pod",
			ApiVersion: "v1",
			Kind:       "Pod",
			NamespaceSelector: &types.NamespaceSelector{
				NameSelector: &types.NameSelector{
					MatchNames: []string{"d8-system"},
				},
			},
			LabelSelector: &v1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "deckhouse",
				},
			},
			FieldSelector: &types.FieldSelector{
				MatchExpressions: []types.FieldSelectorRequirement{
					{
						Field:    "status.phase",
						Operator: "Equals",
						Value:    "Running",
					},
				},
			},
			ExecuteHookOnEvents:          pointer.BoolPtr(false),
			ExecuteHookOnSynchronization: pointer.BoolPtr(false),
			FilterFunc:                   filterDeckhousePod,
		},
		{
			Name:                         "releases",
			ApiVersion:                   "deckhouse.io/v1alpha1",
			Kind:                         "DeckhouseRelease",
			ExecuteHookOnEvents:          pointer.BoolPtr(false),
			ExecuteHookOnSynchronization: pointer.BoolPtr(false),
			FilterFunc:                   filterDeckhouseRelease,
		},
		{
			Name:       "updating_cm",
			ApiVersion: "v1",
			Kind:       "ConfigMap",
			NamespaceSelector: &types.NamespaceSelector{
				NameSelector: &types.NameSelector{
					MatchNames: []string{"d8-system"},
				},
			},
			NameSelector: &types.NameSelector{
				MatchNames: []string{"d8-release-updating"},
			},
			ExecuteHookOnSynchronization: pointer.BoolPtr(false),
			ExecuteHookOnEvents:          pointer.BoolPtr(false),
			FilterFunc:                   filterUpdatingCM,
		},
	},
}, dependency.WithExternalDependencies(updateDeckhouse))

type deckhousePodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	ImageID   string `json:"imageID"`
	Ready     bool   `json:"ready"`
}

// while cluster bootstrapping we have the tag for deckhouse image like: alpha, beta, early-access, stable, rock-solid
// it is set via dhctl, which does not know anything about releases and tags
// We can use this bootstrap image for applying first release without any requirements (like update windows, canary, etc)
func (dpi deckhousePodInfo) isBootstrapImage() bool {
	colonIndex := strings.LastIndex(dpi.Image, ":")
	if colonIndex == -1 {
		return false
	}

	tag := dpi.Image[colonIndex+1:]

	if tag == "" {
		return false
	}

	switch strings.ToLower(tag) {
	case "alpha", "beta", "early-access", "stable", "rock-solid":
		return true

	default:
		return false
	}
}

const (
	metricReleasesGroup = "d8_releases"
	metricUpdatingGroup = "d8_updating"
)

func updateDeckhouse(input *go_hook.HookInput, dc dependency.Container) error {
	deckhousePod := getDeckhousePod(input.Snapshots["deckhouse_pod"])
	if deckhousePod == nil {
		input.LogEntry.Warn("Deckhouse pod does not exist. Skipping update")
		return nil
	}

	if !input.Values.Exists("deckhouse.releaseChannel") {
		// dev upgrade - by tag
		return tagUpdate(input, dc, deckhousePod)
	}

	// production upgrade
	input.MetricsCollector.Expire(metricReleasesGroup)

	if deckhousePod.Ready {
		input.MetricsCollector.Expire(metricUpdatingGroup)
		if isUpdatingCMExists(input) {
			deleteUpdatingCM(input)
		}
	} else if isUpdatingCMExists(input) {
		input.MetricsCollector.Set("d8_is_updating", 1, nil, metrics.WithGroup(metricUpdatingGroup))
	}

	// initialize updater
	approvalMode := input.Values.Get("deckhouse.update.mode").String()
	updater := newDeckhouseUpdater(approvalMode, deckhousePod.Ready, deckhousePod.isBootstrapImage())

	// fetch releases from snapshot and patch initial statuses
	updater.FetchAndPrepareReleases(input)
	if len(updater.releases) == 0 {
		return nil
	}

	// predict next patch for Deploy
	updater.PredictNextRelease()

	// has already Deployed the latest release
	if updater.LastReleaseDeployed() {
		return nil
	}

	// some release is forced, burn everything, apply this patch!
	if updater.HasForceRelease() {
		updater.ApplyForcedRelease(input)
		return nil
	}

	if updater.PredictedReleaseIsPatch() {
		// patch release does not respect update windows or ManualMode
		updater.ApplyPredictedRelease(input)
		return nil
	} else if !updater.inManualMode {
		// update windows works only for Auto deployment mode
		windows, err := getUpdateWindows(input)
		if err != nil {
			return fmt.Errorf("update windows configuration is not valid: %s", err)
		}

		updatePermitted := isUpdatePermitted(windows)

		if !updatePermitted {
			input.LogEntry.Info("Deckhouse update does not get into update windows. Skipping")
			release := updater.PredictedRelease()
			if release != nil {
				updateStatus(input, release, "Release is waiting for update window", v1alpha1.PhasePending)
			}
			return nil
		}
	}

	updater.ApplyPredictedRelease(input)
	return nil
}

// getUpdateWindows return set update windows
func getUpdateWindows(input *go_hook.HookInput) (update.Windows, error) {
	windowsData, exists := input.Values.GetOk("deckhouse.update.windows")
	if !exists {
		return nil, nil
	}

	return update.FromJSON([]byte(windowsData.Raw))
}

// used also in check_deckhouse_release.go
func filterDeckhouseRelease(unstructured *unstructured.Unstructured) (go_hook.FilterResult, error) {
	var release v1alpha1.DeckhouseRelease

	err := sdk.FromUnstructured(unstructured, &release)
	if err != nil {
		return nil, err
	}

	var hasSuspendAnnotation, hasForceAnnotation, hasDisruptionApprovedAnnotation bool

	if v, ok := release.Annotations["release.deckhouse.io/suspended"]; ok {
		if v == "true" {
			hasSuspendAnnotation = true
		}
	}

	if v, ok := release.Annotations["release.deckhouse.io/force"]; ok {
		if v == "true" {
			hasForceAnnotation = true
		}
	}

	if v, ok := release.Annotations["release.deckhouse.io/disruption-approved"]; ok {
		if v == "true" {
			hasDisruptionApprovedAnnotation = true
		}
	}

	var releaseApproved bool
	if v, ok := release.Annotations["release.deckhouse.io/approved"]; ok {
		if v == "true" {
			releaseApproved = true
		}
	} else {
		releaseApproved = release.Approved
	}

	var cooldown *time.Time
	if v, ok := release.Annotations["release.deckhouse.io/cooldown"]; ok {
		cd, err := time.Parse(time.RFC3339, v)
		if err == nil {
			cooldown = &cd
		}
	}

	return deckhouseRelease{
		Name:          release.Name,
		Version:       semver.MustParse(release.Spec.Version),
		ApplyAfter:    release.Spec.ApplyAfter,
		CooldownUntil: cooldown,
		Requirements:  release.Spec.Requirements,
		Disruptions:   release.Spec.Disruptions,
		Status: v1alpha1.DeckhouseReleaseStatus{
			Phase:    release.Status.Phase,
			Approved: release.Status.Approved,
			Message:  release.Status.Message,
		},
		ManuallyApproved:                releaseApproved,
		HasSuspendAnnotation:            hasSuspendAnnotation,
		HasForceAnnotation:              hasForceAnnotation,
		HasDisruptionApprovedAnnotation: hasDisruptionApprovedAnnotation,
	}, nil
}

func filterUpdatingCM(unstructured *unstructured.Unstructured) (go_hook.FilterResult, error) {
	return unstructured.GetName(), nil
}

func filterDeckhousePod(unstructured *unstructured.Unstructured) (go_hook.FilterResult, error) {
	var pod corev1.Pod
	err := sdk.FromUnstructured(unstructured, &pod)
	if err != nil {
		return nil, err
	}

	// ignore evicted and shutdown pods
	if pod.Status.Phase == corev1.PodFailed {
		return nil, nil
	}

	var imageName, imageID string

	if len(pod.Spec.Containers) > 0 {
		imageName = pod.Spec.Containers[0].Image
	}

	var ready bool

	if len(pod.Status.ContainerStatuses) > 0 {
		imageID = pod.Status.ContainerStatuses[0].ImageID
		ready = pod.Status.ContainerStatuses[0].Ready
	}

	return deckhousePodInfo{
		Image:     imageName,
		ImageID:   imageID,
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Ready:     ready,
	}, nil
}

func isUpdatePermitted(windows update.Windows) bool {
	if len(windows) == 0 {
		return true
	}

	now := time.Now()

	if os.Getenv("D8_IS_TESTS_ENVIRONMENT") != "" {
		now = time.Date(2021, 01, 01, 13, 30, 00, 00, time.UTC)
	}

	return windows.IsAllowed(now)
}

// tagUpdate update by tag, in dev mode or specified image
func tagUpdate(input *go_hook.HookInput, dc dependency.Container, deckhousePod *deckhousePodInfo) error {
	if deckhousePod.Image == "" && deckhousePod.ImageID == "" {
		// pod is restarting or something like that, try more in a 15 seconds
		return nil
	}

	if deckhousePod.Image == "" || deckhousePod.ImageID == "" {
		input.LogEntry.Debug("Deckhouse pod is not ready. Try to update later")
		return nil
	}

	idSplitIndex := strings.LastIndex(deckhousePod.ImageID, "@")
	if idSplitIndex == -1 {
		return fmt.Errorf("image hash not found: %s", deckhousePod.ImageID)
	}
	imageHash := deckhousePod.ImageID[idSplitIndex+1:]

	imageSplitIndex := strings.LastIndex(deckhousePod.Image, ":")
	if imageSplitIndex == -1 {
		return fmt.Errorf("image tag not found: %s", deckhousePod.Image)
	}
	repo := deckhousePod.Image[:imageSplitIndex]
	tag := deckhousePod.Image[imageSplitIndex+1:]

	regClient, err := dc.GetRegistryClient(repo, cr.WithCA(getCA(input)), cr.WithInsecureSchema(isHTTP(input)))
	if err != nil {
		input.LogEntry.Errorf("Registry (%s) client init failed: %s", repo, err)
		return nil
	}

	input.MetricsCollector.Inc("deckhouse_registry_check_total", map[string]string{})
	input.MetricsCollector.Inc("deckhouse_kube_image_digest_check_total", map[string]string{})

	repoDigest, err := regClient.Digest(tag)
	if err != nil {
		input.MetricsCollector.Inc("deckhouse_registry_check_errors_total", map[string]string{})
		input.LogEntry.Errorf("Registry (%s) get digest failed: %s", repo, err)
		return nil
	}

	input.MetricsCollector.Set("deckhouse_kube_image_digest_check_success", 1.0, map[string]string{})

	if strings.TrimSpace(repoDigest) == strings.TrimSpace(imageHash) {
		return nil
	}

	input.LogEntry.Info("New deckhouse image found. Restarting")

	input.PatchCollector.Delete("v1", "Pod", deckhousePod.Namespace, deckhousePod.Name)

	return nil
}

// Updater

type deckhouseUpdater struct {
	now          time.Time
	inManualMode bool

	// don't modify releases order, logic is based on this sorted slice
	releases                   []deckhouseRelease
	totalPendingManualReleases int

	predictedReleaseIndex       int
	skippedPatchesIndexes       []int
	currentDeployedReleaseIndex int
	forcedReleaseIndex          int

	deckhousePodIsReady      bool
	deckhouseIsBootstrapping bool
}
type deckhouseRelease struct {
	Name    string
	Version *semver.Version

	ManuallyApproved                bool
	HasSuspendAnnotation            bool
	HasForceAnnotation              bool
	HasDisruptionApprovedAnnotation bool

	Requirements  map[string]string
	Disruptions   []string
	ApplyAfter    *time.Time
	CooldownUntil *time.Time

	Status v1alpha1.DeckhouseReleaseStatus // don't set transition time here to avoid snapshot overload
}

func newDeckhouseUpdater(mode string, podIsReady, isBootstrapping bool) *deckhouseUpdater {
	return &deckhouseUpdater{
		now:                         time.Now().UTC(),
		inManualMode:                mode == "Manual",
		predictedReleaseIndex:       -1,
		currentDeployedReleaseIndex: -1,
		forcedReleaseIndex:          -1,
		skippedPatchesIndexes:       make([]int, 0),
		deckhousePodIsReady:         podIsReady,
		deckhouseIsBootstrapping:    isBootstrapping,
	}
}

// ApplyPredictedRelease applies predicted release, checks everything:
//   - Deckhouse is ready (except patch)
//   - Canary settings
//   - Manual approving
//   - Release requirements
func (du *deckhouseUpdater) ApplyPredictedRelease(input *go_hook.HookInput) {
	if du.predictedReleaseIndex == -1 {
		return // has no predicted release
	}

	var currentRelease *deckhouseRelease

	predictedRelease := &(du.releases[du.predictedReleaseIndex])

	if du.currentDeployedReleaseIndex != -1 {
		currentRelease = &(du.releases[du.currentDeployedReleaseIndex])
	}

	// if deckhouse pod has bootstrap image -> apply first release
	// doesn't matter which is update mode
	if du.deckhouseIsBootstrapping && len(du.releases) == 1 {
		du.runReleaseDeploy(input, predictedRelease, currentRelease)
		return
	}

	// check: only for minor versions (Ignore patches)
	if !du.PredictedReleaseIsPatch() {
		// check: release cooldown
		if predictedRelease.CooldownUntil != nil {
			if du.now.Before(*predictedRelease.CooldownUntil) {
				input.LogEntry.Infof("Release %s in cooldown", predictedRelease.Name)
				updateStatus(input, predictedRelease, fmt.Sprintf("Release is in cooldown until: %s", predictedRelease.CooldownUntil.Format(time.RFC822)), v1alpha1.PhasePending)
				return
			}
		}

		// check: Deckhouse pod is ready
		if !du.deckhousePodIsReady {
			input.LogEntry.Info("Deckhouse is not ready. Skipping upgrade")
			updateStatus(input, predictedRelease, "Waiting for Deckhouse pod to be ready", v1alpha1.PhasePending)
			return
		}
	}

	// check: canary settings
	if predictedRelease.ApplyAfter != nil {
		if du.now.Before(*predictedRelease.ApplyAfter) {
			input.LogEntry.Infof("Release %s is postponed by canary process. Waiting", predictedRelease.Name)
			updateStatus(input, predictedRelease, fmt.Sprintf("Waiting for canary apply time: %s", predictedRelease.ApplyAfter.Format(time.RFC822)), v1alpha1.PhasePending)
			return
		}
	}

	// check: release is approved or it's a patch
	if !predictedRelease.Status.Approved && !du.PredictedReleaseIsPatch() {
		input.LogEntry.Infof("Release %s is waiting for manual approval", predictedRelease.Name)
		input.MetricsCollector.Set("d8_release_waiting_manual", float64(du.totalPendingManualReleases), map[string]string{"name": predictedRelease.Name}, metrics.WithGroup(metricReleasesGroup))
		updateStatus(input, predictedRelease, "Waiting for manual approval", v1alpha1.PhasePending)
		return
	}

	// check: release requirements
	passed := du.checkReleaseRequirements(input, predictedRelease)
	if !passed {
		input.MetricsCollector.Set("d8_release_blocked", 1, map[string]string{"name": predictedRelease.Name, "reason": "requirement"}, metrics.WithGroup(metricReleasesGroup))
		input.LogEntry.Warningf("Release %s requirements are not met", predictedRelease.Name)
		return
	}

	// check: release disruptions
	passed = du.checkReleaseDisruptions(input, predictedRelease)
	if !passed {
		input.MetricsCollector.Set("d8_release_blocked", 1, map[string]string{"name": predictedRelease.Name, "reason": "disruption"}, metrics.WithGroup(metricReleasesGroup))
		input.LogEntry.Warningf("Release %s disruption approval required", predictedRelease.Name)
		return
	}

	// all checks are passed, deploy release
	du.runReleaseDeploy(input, predictedRelease, currentRelease)
}

func (du *deckhouseUpdater) PredictedRelease() *deckhouseRelease {
	if du.predictedReleaseIndex == -1 {
		return nil // has no predicted release
	}

	predictedRelease := &(du.releases[du.predictedReleaseIndex])

	return predictedRelease
}

func (du *deckhouseUpdater) checkReleaseDisruptions(input *go_hook.HookInput, rl *deckhouseRelease) bool {
	dMode, ok := input.Values.GetOk("deckhouse.update.disruptionApprovalMode")
	if !ok || dMode.String() == "Auto" {
		return true
	}

	for _, key := range rl.Disruptions {
		hasDisruptionUpdate, reason := requirements.HasDisruption(key)
		if hasDisruptionUpdate {
			if !rl.HasDisruptionApprovedAnnotation {
				msg := fmt.Sprintf("Release requires disruption approval (`kubectl annotate DeckhouseRelease %s release.deckhouse.io/disruption-approved=true`): %s", rl.Name, reason)
				updateStatus(input, rl, msg, v1alpha1.PhasePending)
				return false
			}
		}
	}

	return true
}

func (du *deckhouseUpdater) runReleaseDeploy(input *go_hook.HookInput, predictedRelease, currentRelease *deckhouseRelease) {
	input.LogEntry.Infof("Applying release %s", predictedRelease.Name)

	repo := input.Values.Get("global.modulesImages.registry").String()

	createUpdatingCM(input, predictedRelease.Version.String())

	// patch deckhouse deployment is faster then set internal values and then upgrade by helm
	// we can set "deckhouse.internal.currentReleaseImageName" value but lets left it this way
	input.PatchCollector.Filter(func(u *unstructured.Unstructured) (*unstructured.Unstructured, error) {
		var depl appsv1.Deployment
		err := sdk.FromUnstructured(u, &depl)
		if err != nil {
			return nil, err
		}

		depl.Spec.Template.Spec.Containers[0].Image = repo + ":" + predictedRelease.Version.Original()

		return sdk.ToUnstructured(&depl)
	}, "apps/v1", "Deployment", "d8-system", "deckhouse")

	updateStatus(input, predictedRelease, "", v1alpha1.PhaseDeployed, true)

	if currentRelease != nil {
		updateStatus(input, currentRelease, "Last Deployed release outdated", v1alpha1.PhaseOutdated)
	}

	if len(du.skippedPatchesIndexes) > 0 {
		for _, index := range du.skippedPatchesIndexes {
			release := du.releases[index]
			updateStatus(input, &release, "Skipped because of new patches", v1alpha1.PhaseOutdated, true)
		}
	}
}

// PredictNextRelease runs prediction of the next release to deploy.
// it skips patch releases and save only the latest one
func (du *deckhouseUpdater) PredictNextRelease() {
	for i, release := range du.releases {
		switch release.Status.Phase {
		case v1alpha1.PhaseOutdated, v1alpha1.PhaseSuspended:
			// pass

		case v1alpha1.PhasePending:
			du.processPendingRelease(i, release)

		case v1alpha1.PhaseDeployed:
			du.currentDeployedReleaseIndex = i
		}

		if release.HasForceAnnotation {
			du.forcedReleaseIndex = i
		}
	}
}

// LastReleaseDeployed returns the equality of the latest existed release with the latest deployed
func (du *deckhouseUpdater) LastReleaseDeployed() bool {
	return du.currentDeployedReleaseIndex == len(du.releases)-1
}

// HasForceRelease check the existence of the forced release
func (du *deckhouseUpdater) HasForceRelease() bool {
	return du.forcedReleaseIndex != -1
}

// ApplyForcedRelease deploys forced release without any checks (windows, requirements, approvals and so on)
func (du *deckhouseUpdater) ApplyForcedRelease(input *go_hook.HookInput) {
	if du.forcedReleaseIndex == -1 {
		return
	}
	forcedRelease := &(du.releases[du.forcedReleaseIndex])
	var currentRelease *deckhouseRelease
	if du.currentDeployedReleaseIndex != -1 {
		currentRelease = &(du.releases[du.currentDeployedReleaseIndex])
	}

	input.LogEntry.Warnf("Forcing release %s", forcedRelease.Name)

	du.runReleaseDeploy(input, forcedRelease, currentRelease)

	annotationsPatch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				"release.deckhouse.io/force": nil,
			},
		},
	}
	// remove annotation
	input.PatchCollector.MergePatch(annotationsPatch, "deckhouse.io/v1alpha1", "DeckhouseRelease", "", forcedRelease.Name)

	// Outdate all previous releases

	for i, release := range du.releases {
		if i < du.forcedReleaseIndex {
			updateStatus(input, &release, "", v1alpha1.PhaseOutdated, true)
		}
	}
}

// PredictedReleaseIsPatch shows if the predicted release is a patch with respect to the Deployed one
func (du *deckhouseUpdater) PredictedReleaseIsPatch() bool {
	if du.currentDeployedReleaseIndex == -1 {
		return false
	}

	if du.predictedReleaseIndex == -1 {
		return false
	}

	current := du.releases[du.currentDeployedReleaseIndex]
	predicted := du.releases[du.predictedReleaseIndex]

	if current.Version.Major() != predicted.Version.Major() {
		return false
	}

	if current.Version.Minor() != predicted.Version.Minor() {
		return false
	}

	return true
}

func (du *deckhouseUpdater) processPendingRelease(index int, release deckhouseRelease) {
	// check: already has predicted release and current is a patch
	if du.predictedReleaseIndex >= 0 {
		previousPredictedRelease := du.releases[du.predictedReleaseIndex]
		if previousPredictedRelease.Version.Major() != release.Version.Major() {
			return
		}

		if previousPredictedRelease.Version.Minor() != release.Version.Minor() {
			return
		}
		// it's a patch for predicted release, continue
		du.skippedPatchesIndexes = append(du.skippedPatchesIndexes, du.predictedReleaseIndex)
	}

	// release is predicted to be Deployed
	du.predictedReleaseIndex = index
}

func (du *deckhouseUpdater) patchInitialStatus(input *go_hook.HookInput, release deckhouseRelease) deckhouseRelease {
	if release.Status.Phase != "" {
		return release
	}

	updateStatus(input, &release, "", v1alpha1.PhasePending)

	return release
}

func (du *deckhouseUpdater) patchSuspendedStatus(input *go_hook.HookInput, release deckhouseRelease) deckhouseRelease {
	if !release.HasSuspendAnnotation {
		return release
	}

	annotationsPatch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				"release.deckhouse.io/suspended": nil,
			},
		},
	}

	input.PatchCollector.MergePatch(annotationsPatch, "deckhouse.io/v1alpha1", "DeckhouseRelease", "", release.Name)
	updateStatus(input, &release, "Release is suspended", v1alpha1.PhaseSuspended, false)

	return release
}

func (du *deckhouseUpdater) patchManualRelease(input *go_hook.HookInput, release deckhouseRelease) deckhouseRelease {
	if release.Status.Phase != v1alpha1.PhasePending {
		return release
	}

	var statusChanged bool

	statusPatch := statusPatch{
		Phase:          release.Status.Phase,
		Approved:       release.Status.Approved,
		TransitionTime: du.now,
	}

	// check and set .status.approved for pending releases
	if du.inManualMode && !release.ManuallyApproved {
		statusPatch.Approved = false
		statusPatch.Message = "Release is waiting for manual approval"
		du.totalPendingManualReleases++
		if release.Status.Approved {
			statusChanged = true
		}
	} else {
		statusPatch.Approved = true
		if !release.Status.Approved {
			statusChanged = true
		}
	}

	if statusChanged {
		input.PatchCollector.MergePatch(statusPatch, "deckhouse.io/v1alpha1", "DeckhouseRelease", "", release.Name, object_patch.WithSubresource("/status"))
		release.Status.Approved = statusPatch.Approved
	}

	return release
}

// FetchAndPrepareReleases fetches releases from snapshot and then:
//   - patch releases with empty status (just created)
//   - handle suspended releases (patch status and remove annotation)
//   - patch manual releases (change status)
func (du *deckhouseUpdater) FetchAndPrepareReleases(input *go_hook.HookInput) {
	snap := input.Snapshots["releases"]
	if len(snap) == 0 {
		return
	}

	releases := make([]deckhouseRelease, 0, len(snap))

	for _, rl := range snap {
		release := rl.(deckhouseRelease)

		release = du.patchInitialStatus(input, release)

		release = du.patchSuspendedStatus(input, release)

		release = du.patchManualRelease(input, release)

		releases = append(releases, release)
	}

	sort.Sort(byVersion(releases))

	du.releases = releases
}

func (du *deckhouseUpdater) checkReleaseRequirements(input *go_hook.HookInput, rl *deckhouseRelease) bool {
	for key, value := range rl.Requirements {
		passed, err := requirements.CheckRequirement(key, value, input.Values)
		if !passed {
			msg := fmt.Sprintf("%q requirement for deckhouseRelease %q not met: %s", key, rl.Version, err)
			if errors.Is(err, requirements.ErrNotRegistered) {
				input.LogEntry.Error(err)
				msg = fmt.Sprintf("%q requirement not registered", key)
			}
			updateStatus(input, rl, msg, v1alpha1.PhasePending, false)
			return false
		}
	}

	return true
}

func createUpdatingCM(input *go_hook.HookInput, version string) {
	cm := &corev1.ConfigMap{
		TypeMeta: v1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "d8-release-updating",
			Namespace: "d8-system",
			Labels: map[string]string{
				"heritage": "deckhouse",
			},
		},
		Data: map[string]string{
			"version": version,
		},
	}

	input.PatchCollector.Create(cm, object_patch.UpdateIfExists())
}

func isUpdatingCMExists(input *go_hook.HookInput) bool {
	snap := input.Snapshots["updating_cm"]
	return len(snap) > 0
}

func deleteUpdatingCM(input *go_hook.HookInput) {
	input.PatchCollector.Delete("v1", "ConfigMap", "d8-system", "d8-release-updating", object_patch.InBackground())
}

func updateStatus(input *go_hook.HookInput, release *deckhouseRelease, msg, phase string, approvedFlag ...bool) {
	approved := release.Status.Approved
	if len(approvedFlag) > 0 {
		approved = approvedFlag[0]
	}

	if phase == release.Status.Phase && msg == release.Status.Message && approved == release.Status.Approved {
		return
	}

	st := statusPatch{
		Phase:          phase,
		Message:        msg,
		Approved:       approved,
		TransitionTime: time.Now().UTC(),
	}
	input.PatchCollector.MergePatch(st, "deckhouse.io/v1alpha1", "DeckhouseRelease", "", release.Name, object_patch.WithSubresource("/status"))

	release.Status.Phase = phase
	release.Status.Message = msg
	release.Status.Approved = approved
}

func getDeckhousePod(snap []go_hook.FilterResult) *deckhousePodInfo {
	var deckhousePod deckhousePodInfo

	switch len(snap) {
	case 0:
		return nil

	case 1:
		deckhousePod = snap[0].(deckhousePodInfo)

	default:
		for _, sn := range snap {
			if sn == nil {
				continue
			}
			deckhousePod = sn.(deckhousePodInfo)
			break
		}
	}

	return &deckhousePod
}

type byVersion []deckhouseRelease

func (a byVersion) Len() int {
	return len(a)
}
func (a byVersion) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a byVersion) Less(i, j int) bool {
	return a[i].Version.LessThan(a[j].Version)
}

type statusPatch v1alpha1.DeckhouseReleaseStatus

func (sp statusPatch) MarshalJSON() ([]byte, error) {
	m := map[string]interface{}{
		"status": v1alpha1.DeckhouseReleaseStatus(sp),
	}

	return json.Marshal(m)
}
