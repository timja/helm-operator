/*

This package has the algorithm for making sure the Helm releases in
the cluster match what are defined in the HelmRelease resources.

There are several ways they can be mismatched. Here's how they are
reconciled:

 1a. There is a HelmRelease resource, but no corresponding
   release. This can happen when the helm operator is first run, for
   example.

 1b. The release corresponding to a HelmRelease has been updated by
   some other means, perhaps while the operator wasn't running. This
   is also checked, by doing a dry-run release and comparing the result
   to the release.

 2. The chart has changed in git, meaning the release is out of
   date. The ChartChangeSync responds to new git commits by looking up
   each chart that makes use of the mirror that has new commits,
   replacing the clone for that chart, and scheduling a new release.

1a.) and 1b.) run on the same schedule, and 2.) is run when a git
mirror reports it has fetched from upstream _and_ (upon checking) the
head of the branch has changed.

Since both 1*.) and 2.) look at the charts in the git repo, but run on
different schedules (non-deterministically), there's a chance that
they can fight each other. For example, the git mirror may fetch new
commits which are used in 1), then treated as changes subsequently by
2). To keep consistency between the two, the current revision of a
repo is used by 1), and advanced only by 2).

*/
package chartsync

import (
	"fmt"
	"github.com/fluxcd/helm-operator/pkg/helm"
	"path/filepath"

	helmfluxv1 "github.com/fluxcd/helm-operator/pkg/apis/helm.fluxcd.io/v1"
	ifclientset "github.com/fluxcd/helm-operator/pkg/client/clientset/versioned"
	iflister "github.com/fluxcd/helm-operator/pkg/client/listers/helm.fluxcd.io/v1"
	"github.com/fluxcd/helm-operator/pkg/release"
	"github.com/fluxcd/helm-operator/pkg/status"
	"github.com/go-kit/kit/log"
	"github.com/google/go-cmp/cmp"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// condition change reasons
	ReasonDownloadFailed   = "RepoFetchFailed"
	ReasonDownloaded       = "RepoChartInCache"
	ReasonInstallFailed    = "HelmInstallFailed"
	ReasonDependencyFailed = "UpdateDependencyFailed"
	ReasonUpgradeFailed    = "HelmUpgradeFailed"
	ReasonRollbackFailed   = "HelmRollbackFailed"
	ReasonSuccess          = "HelmSuccess"
)

type Clients struct {
	KubeClient  kubernetes.Clientset
	IfClient    ifclientset.Clientset
	HrLister    iflister.HelmReleaseLister
	HelmClients *helm.Clients
}

type Config struct {
	ChartCache string
	LogDiffs   bool
	UpdateDeps bool
}

func (c Config) WithDefaults() Config {
	if c.ChartCache == "" {
		c.ChartCache = "/tmp"
	}
	return c
}

type ChartChangeSync struct {
	logger             log.Logger
	kubeClient         kubernetes.Clientset
	ifClient           ifclientset.Clientset
	helmClients        *helm.Clients
	gitChartSourceSync *GitChartSourceSync
	config             Config
}

func New(logger log.Logger, clients Clients, gitChartSourceSync *GitChartSourceSync, config Config) *ChartChangeSync {
	return &ChartChangeSync{
		logger:             logger,
		kubeClient:         clients.KubeClient,
		ifClient:           clients.IfClient,
		helmClients:        clients.HelmClients,
		gitChartSourceSync: gitChartSourceSync,
		config:             config.WithDefaults(),
	}
}

// CompareValuesChecksum recalculates the checksum of the values
// and compares it to the last recorded checksum.
func (chs *ChartChangeSync) CompareValuesChecksum(hr helmfluxv1.HelmRelease) bool {
	var chartPath string
	if hr.Spec.ChartSource.GitChartSource != nil {
		source, ok := chs.getGitChartSource(hr)
		if !ok {
			return false
		}
		source.Lock()
		defer source.Unlock()
		chartPath = source.ChartPath(hr.Spec.Path)
	} else if hr.Spec.ChartSource.RepoChartSource != nil {
		var ok bool
		chartPath, _, ok = chs.getRepoChartSource(hr)
		if !ok {
			return false
		}
	}

	values, err := release.Values(chs.kubeClient.CoreV1(), hr.Namespace, chartPath, hr.GetValuesFromSources(), hr.Spec.Values)
	if err != nil {
		return false
	}

	strValues, err := values.YAML()
	if err != nil {
		return false
	}

	return hr.Status.ValuesChecksum == release.ValuesChecksum([]byte(strValues))
}

// ReconcileReleaseDef asks the ChartChangeSync to examine the release
// associated with a HelmRelease, and install or upgrade the
// release if the chart it refers to has changed.
func (chs *ChartChangeSync) ReconcileReleaseDef(r *release.Release, hr helmfluxv1.HelmRelease) {
	defer chs.updateObservedGeneration(hr)

	releaseName := hr.GetReleaseName()
	logger := log.With(chs.logger, "release", releaseName, "targetNamespace", hr.GetTargetNamespace(), "resource", hr.ResourceID().String())

	// Attempt to retrieve an upgradable release, in case no release
	// or error is returned, install it.
	rel, err := r.GetUpgradableRelease(hr.GetTargetNamespace(), releaseName)
	if err != nil {
		logger.Log("warning", "unable to proceed with release", "err", err)
		return
	}

	opts := release.InstallOptions{DryRun: false}

	chartPath, chartRevision := "", ""
	if hr.Spec.ChartSource.GitChartSource != nil {
		source, ok := chs.getGitChartSource(hr)
		if !ok {
			return
		}
		source.Lock()
		defer source.Unlock()
		chartPath, chartRevision = source.ChartPath(hr.Spec.Path), source.Head
	} else if hr.Spec.ChartSource.RepoChartSource != nil {
		var ok bool
		chartPath, chartRevision, ok = chs.getRepoChartSource(hr)
		if !ok {
			return
		}
		chs.setCondition(hr, helmfluxv1.HelmReleaseChartFetched, v1.ConditionTrue, ReasonDownloaded, "chart fetched: "+filepath.Base(chartPath))
	}

	if rel == nil {
		_, checksum, err := r.Install(chartPath, releaseName, hr, release.InstallAction, opts, &chs.kubeClient)
		if err != nil {
			chs.setCondition(hr, helmfluxv1.HelmReleaseReleased, v1.ConditionFalse, ReasonInstallFailed, err.Error())
			chs.logger.Log("warning", "failed to install chart", "err", err)
			return
		}
		chs.setCondition(hr, helmfluxv1.HelmReleaseReleased, v1.ConditionTrue, ReasonSuccess, "helm install succeeded")
		if err = status.SetReleaseRevision(chs.ifClient.HelmV1().HelmReleases(hr.Namespace), hr, chartRevision); err != nil {
			chs.logger.Log("warning", "could not update the release revision", "err", err)
		}
		if err = status.SetValuesChecksum(chs.ifClient.HelmV1().HelmReleases(hr.Namespace), hr, checksum); err != nil {
			chs.logger.Log("warning", "could not update the values checksum", "err", err)
		}
		return
	}

	if !r.ManagedByHelmRelease(rel, hr) {
		msg := fmt.Sprintf("release '%s' does not belong to HelmRelease", releaseName)
		chs.setCondition(hr, helmfluxv1.HelmReleaseReleased, v1.ConditionFalse, ReasonUpgradeFailed, msg)
		chs.logger.Log("warning", msg+", this may be an indication that multiple HelmReleases with the same release name exist")
		return
	}

	changed, err := chs.shouldUpgrade(r, chartPath, rel, hr)
	if err != nil {
		chs.logger.Log("warning", "unable to determine if release has changed", "err", err)
		return
	}
	if changed {
		cHr, err := chs.ifClient.HelmV1().HelmReleases(hr.Namespace).Get(hr.Name, metav1.GetOptions{})
		if err != nil {
			chs.logger.Log("warning", "failed to retrieve HelmRelease scheduled for upgrade", "err", err)
			return
		}
		if diff := cmp.Diff(hr.Spec, cHr.Spec); diff != "" {
			chs.logger.Log("warning", "HelmRelease spec has diverged since we calculated if we should upgrade, skipping upgrade")
			return
		}
		_, checksum, err := r.Install(chartPath, releaseName, hr, release.UpgradeAction, opts, &chs.kubeClient)
		if err != nil {
			chs.setCondition(hr, helmfluxv1.HelmReleaseReleased, v1.ConditionFalse, ReasonUpgradeFailed, err.Error())
			if err = status.SetValuesChecksum(chs.ifClient.HelmV1().HelmReleases(hr.Namespace), hr, checksum); err != nil {
				chs.logger.Log("warning", "could not update the values checksum", "err", err)
			}
			chs.logger.Log("warning", "failed to upgrade chart", "err", err)
			chs.RollbackRelease(r, hr)
			return
		}
		chs.setCondition(hr, helmfluxv1.HelmReleaseReleased, v1.ConditionTrue, ReasonSuccess, "helm upgrade succeeded")
		if err = status.SetReleaseRevision(chs.ifClient.HelmV1().HelmReleases(hr.Namespace), hr, chartRevision); err != nil {
			chs.logger.Log("warning", "could not update the release revision", "err", err)
		}
		if err = status.SetValuesChecksum(chs.ifClient.HelmV1().HelmReleases(hr.Namespace), hr, checksum); err != nil {
			chs.logger.Log("warning", "could not update the values checksum", "err", err)
		}
		return
	}
}

// RollbackRelease rolls back a helm release
func (chs *ChartChangeSync) RollbackRelease(r *release.Release, hr helmfluxv1.HelmRelease) {
	defer chs.updateObservedGeneration(hr)

	if !hr.Spec.Rollback.Enable {
		return
	}

	_, err := r.Rollback(hr)
	if err != nil {
		log.With(
			chs.logger,
			"release", hr.GetReleaseName(), "targetNamespace", hr.GetTargetNamespace(), "resource", hr.ResourceID().String(),
		).Log("warning", "unable to rollback chart release", "err", err)
		chs.setCondition(hr, helmfluxv1.HelmReleaseRolledBack, v1.ConditionFalse, ReasonRollbackFailed, err.Error())
	}
	chs.setCondition(hr, helmfluxv1.HelmReleaseRolledBack, v1.ConditionTrue, ReasonSuccess, "helm rollback succeeded")
}

// DeleteRelease deletes the helm release associated with a
// HelmRelease. This exists mainly so that the operator code can
// call it when it is handling a resource deletion.
func (chs *ChartChangeSync) DeleteRelease(r *release.Release, hr helmfluxv1.HelmRelease) {
	err := r.Uninstall(hr)
	if err != nil {
		log.With(
			chs.logger,
			"release", hr.GetReleaseName(), "targetNamespace", hr.GetTargetNamespace(), "resource", hr.ResourceID().String(),
		).Log("warning", "chart release not deleted", "err", err)
	}
	chs.gitChartSourceSync.Delete(&hr)
}

// SyncMirrors instructs all mirrors to refresh from their upstream.
func (chs *ChartChangeSync) SyncMirrors() {
	chs.gitChartSourceSync.SyncMirrors()
}

// setCondition saves the status of a condition.
func (chs *ChartChangeSync) setCondition(hr helmfluxv1.HelmRelease, typ helmfluxv1.HelmReleaseConditionType, st v1.ConditionStatus, reason, message string) error {
	hrClient := chs.ifClient.HelmV1().HelmReleases(hr.Namespace)
	condition := status.NewCondition(typ, st, reason, message)
	return status.SetCondition(hrClient, hr, condition)
}

// updateObservedGeneration updates the observed generation of the
// given HelmRelease to the generation.
func (chs *ChartChangeSync) updateObservedGeneration(hr helmfluxv1.HelmRelease) error {
	hrClient := chs.ifClient.HelmV1().HelmReleases(hr.Namespace)

	return status.SetObservedGeneration(hrClient, hr, hr.Generation)
}

func (chs *ChartChangeSync) getGitChartSource(hr helmfluxv1.HelmRelease) (*GitChartSource, bool) {
	chartSource, _ := chs.gitChartSourceSync.Load(&hr)
	if chartSource == nil {
		return nil, false
	}
	chartSource.Lock()
	defer chartSource.Unlock()

	releaseName := hr.GetReleaseName()
	logger := log.With(chs.logger, "release", releaseName, "targetNamespace", hr.GetTargetNamespace(), "resource", hr.ResourceID().String())

	if chs.config.UpdateDeps && !hr.Spec.ChartSource.GitChartSource.SkipDepUpdate {
		c, ok := chs.helmClients.Load(hr.GetHelmVersion())
		if !ok {
			err := "no Helm client for " + hr.GetHelmVersion()
			chs.setCondition(hr, helmfluxv1.HelmReleaseReleased, v1.ConditionFalse, ReasonDependencyFailed, err)
			logger.Log("warning", "failed to update chart dependencies", "err", err)
			return nil, false
		}
		chartPath := filepath.Join(chartSource.Export.Dir(), hr.Spec.Path)
		if err := c.DependencyUpdate(chartPath); err != nil {
			chs.setCondition(hr, helmfluxv1.HelmReleaseReleased, v1.ConditionFalse, ReasonDependencyFailed, err.Error())
			logger.Log("warning", "failed to update chart dependencies", "err", err)
			return nil, false
		}
	}

	return chartSource, true
}

func (chs *ChartChangeSync) getRepoChartSource(hr helmfluxv1.HelmRelease) (string, string, bool) {
	chartPath, chartRevision := "", ""
	chartSource := hr.Spec.ChartSource.RepoChartSource
	if chartSource == nil {
		return chartPath, chartRevision, false
	}

	path, err := ensureChartFetched(chs.config.ChartCache, chartSource)
	if err != nil {
		chs.setCondition(hr, helmfluxv1.HelmReleaseChartFetched, v1.ConditionFalse, ReasonDownloadFailed, "chart download failed: "+err.Error())
		chs.logger.Log("info", "chart download failed", "resource", hr.ResourceID().String(), "err", err)
		return chartPath, chartRevision, false
	}

	chartPath = path
	chartRevision = chartSource.Version

	return chartPath, chartRevision, true
}

// shouldUpgrade returns true if the current running manifests or chart
// don't match what the repo says we ought to be running, based on
// doing a dry run install from the chart in the git repo.
func (chs *ChartChangeSync) shouldUpgrade(r *release.Release, chartsRepo string, currRel *helm.Release,
	hr helmfluxv1.HelmRelease) (bool, error) {

	if currRel == nil {
		return false, fmt.Errorf("no chart release provided for [%s]", hr.GetName())
	}

	currVals := currRel.Values
	currChart := currRel.Chart

	// Get the desired release state
	opts := release.InstallOptions{DryRun: true}
	tempRelName := string(hr.UID)
	desRel, _, err := r.Install(chartsRepo, tempRelName, hr, release.InstallAction, opts, &chs.kubeClient)
	if err != nil {
		return false, err
	}
	desVals := desRel.Values
	desChart := desRel.Chart

	// compare manifests
	if diff := cmp.Diff(currVals, desVals); diff != "" {
		if chs.config.LogDiffs {
			log.With(
				chs.logger,
				"release", hr.GetReleaseName(), "targetNamespace", hr.GetTargetNamespace(), "resource", hr.ResourceID().String(),
			).Log("info", fmt.Sprintf("release %s: values have diverged", currRel.Name), "diff", diff)
		}
		return true, nil
	}

	// compare chart
	if diff := cmp.Diff(currChart, desChart); diff != "" {
		if chs.config.LogDiffs {
			log.With(
				chs.logger,
				"release", hr.GetReleaseName(), "targetNamespace", hr.GetTargetNamespace(), "resource", hr.ResourceID().String(),
			).Log("info", fmt.Sprintf("release %s: chart has diverged", currRel.Name), "resource", hr.ResourceID().String(), "diff", diff)
		}
		return true, nil
	}

	return false, nil
}
