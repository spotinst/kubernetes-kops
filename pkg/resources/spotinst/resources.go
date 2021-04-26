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
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spotinst/spotinst-sdk-go/service/ocean/providers/aws"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/cloudinstances"
	"k8s.io/kops/pkg/resources"
	"k8s.io/kops/upup/pkg/fi"
)

// ListResources returns a list of all resources.
func ListResources(cloud Cloud, clusterName string) ([]*resources.Resource, error) {
	klog.V(2).Info("Listing all resources")

	fns := []func(context.Context, Cloud, string) ([]*resources.Resource, error){
		listElastigroupResources,
		listOceanResources,
	}

	var out []*resources.Resource
	for _, fn := range fns {
		r, err := fn(context.TODO(), cloud, clusterName)
		if err != nil {
			return nil, fmt.Errorf("spotinst: error listing resources: %v", err)
		}
		out = append(out, r...)
	}

	return out, nil
}

// listElastigroupResources returns a list of all Elastigroup resources.
func listElastigroupResources(ctx context.Context, cloud Cloud,
	clusterName string) ([]*resources.Resource, error) {

	klog.V(2).Info("Listing all Elastigroup resources")
	return listInstanceGroups(ctx, cloud.Elastigroup(),
		filterClusterName(clusterName))
}

// listOceanResources returns a list of all Ocean Cluster resources.
func listOceanResources(ctx context.Context, cloud Cloud,
	clusterName string) ([]*resources.Resource, error) {

	klog.V(2).Info("Listing all Ocean resources")
	var out []*resources.Resource

	clusters, err := listInstanceGroups(ctx, cloud.OceanCluster(),
		filterClusterName(clusterName))
	if err != nil {
		return nil, err
	}
	out = append(out, clusters...)

	for _, cluster := range clusters {
		specs, err := listInstanceGroups(ctx, cloud.OceanLaunchSpec(),
			filterClusterName(clusterName),
			filterOceanId(cluster.ID))
		if err != nil {
			return nil, err
		}
		out = append(out, specs...)
	}

	return out, nil
}

// filterFunc is a function that takes an InstanceGroup and returns whether it
// should be filtered out.
type filterFunc func(InstanceGroup) bool

// filterClusterName filters instance groups based on matching the given cluster name.
func filterClusterName(clusterName string) filterFunc {
	return func(ig InstanceGroup) bool {
		internal := strings.HasPrefix(ig.Name(), "Spotinst::Ocean::")
		suffixed := strings.HasSuffix(ig.Name(), fmt.Sprintf(".%s", clusterName))
		return !internal && suffixed
	}
}

// filterOceanId filters instance groups based on matching the given Ocean ID.
func filterOceanId(oceanID string) filterFunc {
	return func(ig InstanceGroup) bool {
		spec, ok := ig.Obj().(*aws.LaunchSpec)
		return ok && spec != nil && fi.StringValue(spec.OceanID) == oceanID
	}
}

// filterAnd returns a filter function to take logical conjunction of given filters.
func filterAnd(filters ...filterFunc) filterFunc {
	return func(ig InstanceGroup) bool {
		if len(filters) > 0 {
			for _, f := range filters {
				if !f(ig) {
					return false
				}
			}
		}
		return true
	}
}

// listInstanceGroups returns a list of all instance groups.
func listInstanceGroups(ctx context.Context, svc InstanceGroupService,
	filters ...filterFunc) ([]*resources.Resource, error) {

	groups, err := svc.List(ctx)
	if err != nil {
		return nil, err
	}

	var out []*resources.Resource
	for _, group := range groups {
		if filterAnd(filters...)(group) {
			out = append(out, &resources.Resource{
				ID:      group.Id(),
				Name:    group.Name(),
				Type:    string(group.Type()),
				Obj:     group,
				Deleter: instanceGroupDeleter(ctx, svc, group),
				Dumper:  dumper,
			})
		}
	}

	return out, nil
}

// DeleteInstanceGroup deletes an existing InstanceGroup.
func DeleteInstanceGroup(cloud Cloud, group *cloudinstances.CloudInstanceGroup) error {
	klog.V(2).Infof("Deleting instance group: %q", group.HumanName)

	raw := group.Raw
	if ig, ok := raw.(InstanceGroup); ok {
		var svc InstanceGroupService
		switch ig.Type() {
		case InstanceGroupElastigroup:
			svc = cloud.Elastigroup()
		case InstanceGroupOceanCluster:
			svc = cloud.OceanCluster()
		case InstanceGroupOceanLaunchSpec:
			svc = cloud.OceanLaunchSpec()
		}
		if svc != nil {
			return svc.Delete(context.TODO(), ig.Id())
		}
	}

	return fmt.Errorf("spotinst: unexpected instance group type, got: %T", raw)
}

// DeleteInstance removes an instance from its instance group.
func DeleteInstance(cloud Cloud, instance *cloudinstances.CloudInstance) error {
	klog.V(2).Infof("Detaching instance %q from instance group: %q",
		instance.ID, instance.CloudInstanceGroup.HumanName)

	raw := instance.CloudInstanceGroup.Raw
	if ig, ok := raw.(InstanceGroup); ok {
		var svc InstanceGroupService
		switch ig.Type() {
		case InstanceGroupElastigroup:
			svc = cloud.Elastigroup()
		case InstanceGroupOceanCluster:
			svc = cloud.OceanCluster()
		case InstanceGroupOceanLaunchSpec:
			svc = cloud.OceanLaunchSpec()
		}
		if svc != nil {
			return svc.Detach(context.TODO(), ig.Id(), instance.ID)
		}
	}

	return fmt.Errorf("spotinst: unexpected instance group type, got: %T", raw)
}

// DetachInstance is not implemented yet. It needs to cause a cloud instance to
// no longer be counted against the group's size limits.
func DetachInstance(cloud Cloud, instance *cloudinstances.CloudInstance) error {
	return fmt.Errorf("spotinst: does not support surging")
}

// GetCloudGroups returns a list of InstanceGroups as CloudInstanceGroup objects.
func GetCloudGroups(cloud Cloud, cluster *kops.Cluster, instanceGroups []*kops.InstanceGroup,
	warnUnmatched bool, nodes []v1.Node) (map[string]*cloudinstances.CloudInstanceGroup, error) {

	cloudInstanceGroups := make(map[string]*cloudinstances.CloudInstanceGroup)
	nodeMap := cloudinstances.GetNodeMap(nodes, cluster)

	// List all resources.
	resources, err := ListResources(cloud, cluster.Name)
	if err != nil {
		return nil, err
	}

	// Build all cloud instance groups.
	for _, resource := range resources {
		// Ocean Cluster resources should be ignored since we already have the
		// instances from the Ocean Launch Spec resources.
		if InstanceGroupType(resource.Type) == InstanceGroupOceanCluster {
			continue
		}

		// Build cloud instance group.
		ig, err := buildCloudInstanceGroupFromResource(
			cloud, cluster, instanceGroups, resource, nodeMap)
		if err != nil {
			return nil, fmt.Errorf("spotinst: error building cloud "+
				"instance group: %v", err)
		}
		if ig == nil {
			if warnUnmatched {
				klog.V(2).Infof("Found group with no corresponding "+
					"instance group: %q", resource.Name)
			}
			continue
		}

		klog.V(2).Infof("Discovered resource: %q (%s)",
			resource.Name, resource.Type)

		cloudInstanceGroups[resource.Name] = ig
	}

	return cloudInstanceGroups, nil
}

func buildCloudInstanceGroupFromResource(cloud Cloud, cluster *kops.Cluster,
	instanceGroups []*kops.InstanceGroup, resource *resources.Resource,
	nodeMap map[string]*v1.Node) (*cloudinstances.CloudInstanceGroup, error) {
	klog.V(2).Infof("Building instance group for resource: %q", resource.Name)

	ig, err := findInstanceGroupFromResource(cluster, instanceGroups, resource)
	if err != nil {
		return nil, fmt.Errorf("spotinst: failed to find instance group "+
			"of resource %q: %v", resource.Name, err)
	}
	if ig == nil {
		return nil, nil
	}

	switch g := resource.Obj.(InstanceGroup); g.Type() {
	case InstanceGroupElastigroup:
		return buildCloudInstanceGroupFromElastigroup(cloud, ig, g, nodeMap)
	case InstanceGroupOceanLaunchSpec:
		return buildCloudInstanceGroupFromOceanLaunchSpec(cloud, ig, g, nodeMap)
	default:
		return nil, fmt.Errorf("spotinst: unexpected resource type: %s", resource.Type)
	}
}

func buildCloudInstanceGroupFromElastigroup(cloud Cloud, ig *kops.InstanceGroup,
	group InstanceGroup, nodeMap map[string]*v1.Node) (*cloudinstances.CloudInstanceGroup, error) {

	instanceGroup := &cloudinstances.CloudInstanceGroup{
		HumanName:     group.Name(),
		MinSize:       group.MinSize(),
		TargetSize:    group.MinSize(),
		MaxSize:       group.MaxSize(),
		Raw:           group,
		InstanceGroup: ig,
	}

	klog.V(2).Infof("Attempting to fetch all instances of "+
		"instance group %q (id: %q)", group.Name(), group.Id())
	instances, err := cloud.Elastigroup().Instances(context.TODO(), group.Id())
	if err != nil {
		return nil, err
	}

	if err := registerCloudInstances(instanceGroup, nodeMap,
		instances, group.Name(), group.UpdatedAt()); err != nil {
		return nil, err
	}

	return instanceGroup, nil
}

// TODO(liran): We should fetch Ocean's instances using a query param of `?launchSpecId=foo`,
// but, since we do not support it at the moment, we should fetch all instances only once.
var fetchOceanInstances sync.Once

func buildCloudInstanceGroupFromOceanLaunchSpec(cloud Cloud, ig *kops.InstanceGroup,
	spec InstanceGroup, nodeMap map[string]*v1.Node) (*cloudinstances.CloudInstanceGroup, error) {

	instanceGroup := &cloudinstances.CloudInstanceGroup{
		HumanName:     spec.Name(),
		Raw:           spec,
		InstanceGroup: ig,
	}

	var instances []Instance
	var err error

	s, ok := spec.Obj().(*aws.LaunchSpec)
	if !ok {
		return nil, fmt.Errorf("spotinst: unexpected object type: %T", spec.Obj())
	}

	fetchOceanInstances.Do(func() {
		klog.V(2).Infof("Fetching instances of instance group %q (id: %q)", spec.Name(), spec.Id())
		instances, err = cloud.OceanCluster().Instances(context.TODO(), fi.StringValue(s.OceanID))
	})
	if err != nil {
		return nil, err
	}

	if err := registerCloudInstances(instanceGroup, nodeMap,
		instances, spec.Name(), spec.UpdatedAt()); err != nil {
		return nil, err
	}

	return instanceGroup, nil
}

func registerCloudInstances(instanceGroup *cloudinstances.CloudInstanceGroup, nodeMap map[string]*v1.Node,
	instances []Instance, currentInstanceGroupName string, instanceGroupUpdatedAt time.Time) error {
	// The instance registration below registers all active instances with
	// their instance group. It also checks for outdated instances by
	// comparing each instance creation timestamp against the modification
	// timestamp of its instance group.
	//
	// A rolling-update operation involves one or more detach operations, which
	// are performed to replace existing instances. This is done by updating the
	// instance group and results in the modification timestamp being updated to
	// the current time.
	//
	// Once the detach operation is complete, the modification timestamp is
	// updated, meaning that new instances have already been created, so our
	// comparison may be inaccurate.
	//
	// In order to work around this, we assume that the detach operation will
	// take up to two minutes, and therefore we subtract this duration from the
	// modification timestamp of the instance group before the comparison.
	instanceGroupUpdatedAt = instanceGroupUpdatedAt.Add(-2 * time.Minute)

	for _, instance := range instances {
		if instance.Id() == "" {
			klog.Warningf("Ignoring instance with no ID: %v", instance)
			continue
		}

		// If the instance was created before the last update, mark it as `NeedUpdate`.
		newInstanceGroupName := currentInstanceGroupName
		if instance.CreatedAt().Before(instanceGroupUpdatedAt) {
			newInstanceGroupName = fmt.Sprintf("%s:%d",
				currentInstanceGroupName, time.Now().Nanosecond())
		}

		klog.V(2).Infof("Adding instance %q (created at: %s) to "+
			"instance group: %q (updated at: %s)",
			instance.Id(), instance.CreatedAt().Format(time.RFC3339),
			currentInstanceGroupName, instanceGroupUpdatedAt.Format(time.RFC3339))

		status := cloudinstances.CloudInstanceStatusUpToDate
		if newInstanceGroupName != currentInstanceGroupName {
			status = cloudinstances.CloudInstanceStatusNeedsUpdate
		}
		if _, err := instanceGroup.NewCloudInstance(
			instance.Id(), status, nodeMap[instance.Id()]); err != nil {
			return fmt.Errorf("spotinst: error creating cloud "+
				"instance group member: %v", err)
		}
	}

	return nil
}

func findInstanceGroupFromResource(cluster *kops.Cluster, instanceGroups []*kops.InstanceGroup,
	resource *resources.Resource) (*kops.InstanceGroup, error) {

	var instanceGroup *kops.InstanceGroup
	for _, ig := range instanceGroups {
		name := getGroupNameByRole(cluster, ig)
		if name == "" {
			continue
		}
		if name == resource.Name {
			if instanceGroup != nil {
				return nil, fmt.Errorf("spotinst: found multiple "+
					"instance groups matching group: %q", name)
			}

			klog.V(2).Infof("Found group with corresponding instance group: %q", name)
			instanceGroup = ig
		}
	}

	return instanceGroup, nil
}

func getGroupNameByRole(cluster *kops.Cluster, ig *kops.InstanceGroup) string {
	var groupName string

	switch ig.Spec.Role {
	case kops.InstanceGroupRoleMaster:
		groupName = ig.ObjectMeta.Name + ".masters." + cluster.ObjectMeta.Name
	case kops.InstanceGroupRoleNode:
		groupName = ig.ObjectMeta.Name + "." + cluster.ObjectMeta.Name
	case kops.InstanceGroupRoleBastion:
		groupName = ig.ObjectMeta.Name + "." + cluster.ObjectMeta.Name
	default:
		klog.Warningf("Ignoring InstanceGroup of unknown role %q", ig.Spec.Role)
	}

	return groupName
}

func instanceGroupDeleter(ctx context.Context, svc InstanceGroupService,
	group InstanceGroup) func(fi.Cloud, *resources.Resource) error {

	return func(cloud fi.Cloud, resource *resources.Resource) error {
		klog.V(2).Infof("Deleting instance group: %q", group.Id())
		return svc.Delete(ctx, group.Id())
	}
}

func dumper(op *resources.DumpOperation, resource *resources.Resource) error {
	data := make(map[string]interface{})
	data["id"] = resource.ID
	data["type"] = resource.Type
	data["raw"] = resource.Obj
	op.Dump.Resources = append(op.Dump.Resources, data)
	return nil
}
