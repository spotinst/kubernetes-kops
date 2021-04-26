/*
Copyright 2019 The Kubernetes Authors.

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

package spotinst

import (
	"context"
	"time"
)

type (
	// InstanceGroup wraps a cloud-specific instance group object.
	InstanceGroup interface {
		// Id returns the ID of the InstanceGroup.
		Id() string

		// Type returns the type of the InstanceGroup.
		Type() InstanceGroupType

		// Name returns the name of the InstanceGroup.
		Name() string

		// MinSize returns the minimum size of the InstanceGroup.
		MinSize() int

		// MaxSize returns the maximum size of the InstanceGroup.
		MaxSize() int

		// CreatedAt returns the timestamp when the InstanceGroup has been created.
		CreatedAt() time.Time

		// UpdatedAt returns the timestamp when the InstanceGroup has been updated.
		UpdatedAt() time.Time

		// Obj returns the raw object which is a cloud-specific implementation.
		Obj() interface{}
	}

	// Instance wraps a cloud-specific instance object.
	Instance interface {
		// Id returns the ID of the instance.
		Id() string

		// CreatedAt returns the timestamp when the Instance has been created.
		CreatedAt() time.Time

		// Obj returns the raw object which is a cloud-specific implementation.
		Obj() interface{}
	}

	// Cloud wraps all services provided by Spotinst.
	Cloud interface {
		// Elastigroup returns a new Elastigroup service.
		Elastigroup() InstanceGroupService

		// OceanCluster returns a new OceanCluster service.
		OceanCluster() InstanceGroupService

		// OceanLaunchSpec returns a new OceanLaunchSpec service.
		OceanLaunchSpec() InstanceGroupService
	}

	// InstanceGroupService wraps all common functionality for InstanceGroups.
	InstanceGroupService interface {
		// List returns a list of InstanceGroups.
		List(ctx context.Context) ([]InstanceGroup, error)

		// Create creates a new InstanceGroup and returns its ID.
		Create(ctx context.Context, group InstanceGroup) (string, error)

		// Read returns a specific InstanceGroup by ID.
		Read(ctx context.Context, groupID string) (InstanceGroup, error)

		// Update updates an existing InstanceGroup.
		Update(ctx context.Context, group InstanceGroup) error

		// Delete deletes a specific InstanceGroup by ID.
		Delete(ctx context.Context, groupID string) error

		// Detach removes an instances from a specific InstanceGroup.
		Detach(ctx context.Context, groupID, instanceID string) error

		// Instances returns a list of all instances belonging to a specific InstanceGroup.
		Instances(ctx context.Context, groupID string) ([]Instance, error)
	}
)

// InstanceGroupType represents the type of InstanceGroup.
type InstanceGroupType string

// These are valid InstanceGroup types.
const (
	InstanceGroupElastigroup     InstanceGroupType = "elastigroup"
	InstanceGroupOceanCluster    InstanceGroupType = "ocean/cluster"
	InstanceGroupOceanLaunchSpec InstanceGroupType = "ocean/launchspec"
)
