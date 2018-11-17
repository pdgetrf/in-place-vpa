/*
Copyright 2018 The Kubernetes Authors.

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

package nodegroups

import (
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/autoscaler/cluster-autoscaler/context"
	"k8s.io/autoscaler/cluster-autoscaler/utils/errors"
)

// NodeGroupManager is responsible for creating/deleting node groups.
type NodeGroupManager interface {
	CreateNodeGroup(context *context.AutoscalingContext, nodeGroup cloudprovider.NodeGroup) (cloudprovider.NodeGroup, errors.AutoscalerError)
	RemoveUnneededNodeGroups(context *context.AutoscalingContext) error
	CleanUp()
}

// NoOpNodeGroupManager is a no-op implementation of NodeGroupManager.
// It does not remove any node groups and its CreateNodeGroup method always returns an error.
// To be used together with NoOpNodeGroupListProcessor.
type NoOpNodeGroupManager struct {
}

// CreateNodeGroup always returns internal error. It must not be called on NoOpNodeGroupManager.
func (*NoOpNodeGroupManager) CreateNodeGroup(context *context.AutoscalingContext, nodeGroup cloudprovider.NodeGroup) (cloudprovider.NodeGroup, errors.AutoscalerError) {
	return nil, errors.NewAutoscalerError(errors.InternalError, "not implemented")
}

// RemoveUnneededNodeGroups does nothing in NoOpNodeGroupManager
func (*NoOpNodeGroupManager) RemoveUnneededNodeGroups(context *context.AutoscalingContext) error {
	return nil
}

// CleanUp does nothing in NoOpNodeGroupManager
func (*NoOpNodeGroupManager) CleanUp() {}

// NewDefaultNodeGroupManager creates an instance of NodeGroupManager.
func NewDefaultNodeGroupManager() NodeGroupManager {
	return &NoOpNodeGroupManager{}
}
