/*
Copyright 2017 The Kubernetes Authors.

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

package routines

import (
	"time"

	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/api/resource"
	vpa_types "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/poc.autoscaling.k8s.io/v1alpha1"
	vpa_clientset "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	vpa_api "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/typed/poc.autoscaling.k8s.io/v1alpha1"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/checkpoint"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/input"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/logic"
	"k8s.io/autoscaler/vertical-pod-autoscaler/pkg/recommender/model"
	metrics_recommender "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/utils/metrics/recommender"
	vpa_api_util "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/utils/vpa"
	"k8s.io/client-go/rest"
	api "k8s.io/api/core/v1"
)

const (
	// AggregateContainerStateGCInterval defines how often expired AggregateContainerStates are garbage collected.
	AggregateContainerStateGCInterval = 1 * time.Hour
)

// Recommender recommend resources for certain containers, based on utilization periodically got from metrics api.
type Recommender interface {
	// RunOnce performs one iteration of recommender duties followed by update of recommendations in VPA objects.
	RunOnce()
	// GetClusterState returns ClusterState used by Recommender
	GetClusterState() *model.ClusterState
	// GetClusterStateFeeder returns ClusterStateFeeder used by Recommender
	GetClusterStateFeeder() input.ClusterStateFeeder
	// UpdateVPAs computes recommendations and sends VPAs status updates to API Server
	UpdateVPAs()
	// MaintainCheckpoints stores current checkpoints in API Server and garbage collect old ones
	MaintainCheckpoints()
	// GarbageCollect removes old AggregateCollectionStates
	GarbageCollect()
}

type recommender struct {
	clusterState                  *model.ClusterState
	clusterStateFeeder            input.ClusterStateFeeder
	checkpointWriter              checkpoint.CheckpointWriter
	checkpointsGCInterval         time.Duration
	lastCheckpointGC              time.Time
	vpaClient                     vpa_api.VerticalPodAutoscalersGetter
	podResourceRecommender        logic.PodResourceRecommender
	useCheckpoints                bool
	lastAggregateContainerStateGC time.Time
}

func (r *recommender) GetClusterState() *model.ClusterState {
	return r.clusterState
}

func (r *recommender) GetClusterStateFeeder() input.ClusterStateFeeder {
	return r.clusterStateFeeder
}

// Updates VPA CRD objects' statuses.
func (r *recommender) UpdateVPAs() {
	cnt := metrics_recommender.NewObjectCounter()
	defer cnt.Observe()

	for _, observedVpa := range r.clusterState.ObservedVpas {
		key := model.VpaID{
			Namespace: observedVpa.Namespace,
			VpaName:   observedVpa.Name}

		vpa, found := r.clusterState.Vpas[key]
		if !found {
			continue
		}

		/* hack a fake recommendation */
		//resources := r.podResourceRecommender.GetRecommendedPodResources(GetContainerNameToAggregateStateMap(vpa))


		containerResources := make([]vpa_types.RecommendedContainerResources, 0, 2)

/*
		rlCpuTarget := api.ResourceList{}
		rlCpuTarget["cpu"] = resource.MustParse("500m")
		rlCpuLower := api.ResourceList{}
		rlCpuLower["cpu"] = resource.MustParse("600m")
		rlCpuUpper := api.ResourceList{}
		rlCpuUpper["cpu"] = resource.MustParse("700m")
*/

		rlMemTarget := api.ResourceList{}
		rlMemTarget["memory"] = resource.MustParse("128Mi")
		rlMemLower := api.ResourceList{}
		rlMemLower["memory"] = resource.MustParse("64Mi")
		rlMemUpper := api.ResourceList{}
		rlMemUpper["memory"] = resource.MustParse("256Mi")

/*
		containerResources = append(containerResources, vpa_types.RecommendedContainerResources{
				ContainerName: "hamster",
				Target:        rlCpuTarget,
				LowerBound:    rlCpuLower,
				UpperBound:    rlCpuUpper,
			})
*/

		containerResources = append(containerResources, vpa_types.RecommendedContainerResources{
				ContainerName: "hamster",
				Target:        rlMemTarget,
				LowerBound:    rlMemLower,
				UpperBound:    rlMemUpper,
			})

/*
		containerResources := make([]vpa_types.RecommendedContainerResources, 0, len(resources))
		for containerName, res := range resources {
			containerResources = append(containerResources, vpa_types.RecommendedContainerResources{
				ContainerName: containerName,
				Target:        model.ResourcesAsResourceList(res.Target),
				LowerBound:    model.ResourcesAsResourceList(res.LowerBound),
				UpperBound:    model.ResourcesAsResourceList(res.UpperBound),
			})

		}
*/
		had := vpa.HasRecommendation()
		vpa.Recommendation = &vpa_types.RecommendedPodResources{containerResources}
		// Set RecommendationProvided if recommendation not empty.
		if len(vpa.Recommendation.ContainerRecommendations) > 0 {
			vpa.Conditions.Set(vpa_types.RecommendationProvided, true, "", "")
			if !had {
				metrics_recommender.ObserveRecommendationLatency(vpa.Created)
			}
		}
		cnt.Add(vpa)

		_, err := vpa_api_util.UpdateVpaStatusIfNeeded(
			r.vpaClient.VerticalPodAutoscalers(vpa.ID.Namespace), vpa, &observedVpa.Status)
		if err != nil {
			glog.Errorf(
				"Cannot update VPA %v object. Reason: %+v", vpa.ID.VpaName, err)
		}
	}
}

func (r *recommender) MaintainCheckpoints() {
	now := time.Now()
	if r.useCheckpoints {
		r.checkpointWriter.StoreCheckpoints(now)
		if time.Now().Sub(r.lastCheckpointGC) > r.checkpointsGCInterval {
			r.lastCheckpointGC = now
			r.clusterStateFeeder.GarbageCollectCheckpoints()
		}
	}

}

func (r *recommender) GarbageCollect() {
	gcTime := time.Now()
	if gcTime.Sub(r.lastAggregateContainerStateGC) > AggregateContainerStateGCInterval {
		r.clusterState.GarbageCollectAggregateCollectionStates(gcTime)
		r.lastAggregateContainerStateGC = gcTime
	}
}

func (r *recommender) RunOnce() {
	timer := metrics_recommender.NewExecutionTimer()
	defer timer.ObserveTotal()

	glog.V(3).Infof("Recommender Run")
	r.clusterStateFeeder.LoadVPAs()
	timer.ObserveStep("LoadVPAs")
	r.clusterStateFeeder.LoadPods()
	timer.ObserveStep("LoadPods")
	r.clusterStateFeeder.LoadRealTimeMetrics()
	timer.ObserveStep("LoadMetrics")
	glog.V(3).Infof("ClusterState is tracking %v PodStates and %v VPAs", len(r.clusterState.Pods), len(r.clusterState.Vpas))

	r.UpdateVPAs()
	timer.ObserveStep("UpdateVPAs")

	r.MaintainCheckpoints()
	timer.ObserveStep("MaintainCheckpoints")

	r.GarbageCollect()
	timer.ObserveStep("GarbageCollect")
}

// NewRecommender creates a new recommender instance,
// which can be run in order to provide continuous resource recommendations for containers.
// It requires cluster configuration object and duration between recommender intervals.
func NewRecommender(config *rest.Config, checkpointsGCInterval time.Duration, useCheckpoints bool) Recommender {
	clusterState := model.NewClusterState()
	recommender := &recommender{
		clusterState:                  clusterState,
		clusterStateFeeder:            input.NewClusterStateFeeder(config, clusterState),
		checkpointWriter:              checkpoint.NewCheckpointWriter(clusterState, vpa_clientset.NewForConfigOrDie(config).PocV1alpha1()),
		checkpointsGCInterval:         checkpointsGCInterval,
		lastCheckpointGC:              time.Now(),
		vpaClient:                     vpa_clientset.NewForConfigOrDie(config).PocV1alpha1(),
		podResourceRecommender:        logic.CreatePodResourceRecommender(),
		useCheckpoints:                useCheckpoints,
		lastAggregateContainerStateGC: time.Now(),
	}
	glog.V(3).Infof("New Recommender created %+v", recommender)
	return recommender
}
