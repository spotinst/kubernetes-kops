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
	"time"

	"github.com/spotinst/spotinst-sdk-go/service/elastigroup"
	awseg "github.com/spotinst/spotinst-sdk-go/service/elastigroup/providers/aws"
	"github.com/spotinst/spotinst-sdk-go/service/ocean"
	awsoc "github.com/spotinst/spotinst-sdk-go/service/ocean/providers/aws"
	"github.com/spotinst/spotinst-sdk-go/spotinst"
	"github.com/spotinst/spotinst-sdk-go/spotinst/client"
	"github.com/spotinst/spotinst-sdk-go/spotinst/session"
	"k8s.io/kops/upup/pkg/fi"
)

// region Cloud

type awsCloud struct {
	eg InstanceGroupService
	oc InstanceGroupService
	ls InstanceGroupService
}

func newAWSCloud(sess *session.Session) *awsCloud {
	return &awsCloud{
		eg: &awsElastigroupService{
			svc: elastigroup.New(sess).CloudProviderAWS(),
		},
		oc: &awsOceanClusterService{
			svc: ocean.New(sess).CloudProviderAWS(),
		},
		ls: &awsOceanLaunchSpecService{
			svc: ocean.New(sess).CloudProviderAWS(),
		},
	}
}

func (x *awsCloud) Elastigroup() InstanceGroupService     { return x.eg }
func (x *awsCloud) OceanCluster() InstanceGroupService    { return x.oc }
func (x *awsCloud) OceanLaunchSpec() InstanceGroupService { return x.ls }

// endregion

// region Services

type awsElastigroupService struct {
	svc awseg.Service
}

func (x *awsElastigroupService) List(ctx context.Context) ([]InstanceGroup, error) {
	output, err := x.svc.List(ctx, &awseg.ListGroupsInput{})
	if err != nil {
		return nil, err
	}

	groups := make([]InstanceGroup, len(output.Groups))
	for i, group := range output.Groups {
		groups[i] = &awsElastigroupInstanceGroup{group}
	}

	return groups, nil
}

func (x *awsElastigroupService) Create(ctx context.Context, group InstanceGroup) (string, error) {
	input := &awseg.CreateGroupInput{
		Group: group.Obj().(*awseg.Group),
	}

	output, err := x.svc.Create(ctx, input)
	if err != nil {
		return "", err
	}

	return fi.StringValue(output.Group.ID), nil
}

func (x *awsElastigroupService) Read(ctx context.Context, groupID string) (InstanceGroup, error) {
	input := &awseg.ReadGroupInput{
		GroupID: fi.String(groupID),
	}

	output, err := x.svc.Read(ctx, input)
	if err != nil {
		return nil, err
	}

	return &awsElastigroupInstanceGroup{output.Group}, nil
}

func (x *awsElastigroupService) Update(ctx context.Context, group InstanceGroup) error {
	input := &awseg.UpdateGroupInput{
		Group: group.Obj().(*awseg.Group),
	}

	_, err := x.svc.Update(ctx, input)
	return err
}

func (x *awsElastigroupService) Delete(ctx context.Context, groupID string) error {
	input := &awseg.DeleteGroupInput{
		GroupID: fi.String(groupID),
	}

	_, err := x.svc.Delete(ctx, input)
	return err
}

func (x *awsElastigroupService) Detach(ctx context.Context, groupID, instanceID string) error {
	input := &awseg.DetachGroupInput{
		GroupID:                       fi.String(groupID),
		InstanceIDs:                   []string{instanceID},
		ShouldDecrementTargetCapacity: fi.Bool(false),
		ShouldTerminateInstances:      fi.Bool(true),
	}

	_, err := x.svc.Detach(ctx, input)
	return err
}

func (x *awsElastigroupService) Instances(ctx context.Context, groupID string) ([]Instance, error) {
	input := &awseg.StatusGroupInput{
		GroupID: fi.String(groupID),
	}

	output, err := x.svc.Status(ctx, input)
	if err != nil {
		return nil, err
	}

	instances := make([]Instance, len(output.Instances))
	for i, instance := range output.Instances {
		instances[i] = &awsElastigroupInstance{instance}
	}

	return instances, err
}

type awsOceanClusterService struct {
	svc awsoc.Service
}

func (x *awsOceanClusterService) List(ctx context.Context) ([]InstanceGroup, error) {
	output, err := x.svc.ListClusters(ctx, &awsoc.ListClustersInput{})
	if err != nil {
		return nil, err
	}

	groups := make([]InstanceGroup, len(output.Clusters))
	for i, group := range output.Clusters {
		groups[i] = &awsOceanClusterInstanceGroup{group}
	}

	return groups, nil
}

func (x *awsOceanClusterService) Create(ctx context.Context, group InstanceGroup) (string, error) {
	input := &awsoc.CreateClusterInput{
		Cluster: group.Obj().(*awsoc.Cluster),
	}

	output, err := x.svc.CreateCluster(ctx, input)
	if err != nil {
		return "", err
	}

	return fi.StringValue(output.Cluster.ID), nil
}

func (x *awsOceanClusterService) Read(ctx context.Context, clusterID string) (InstanceGroup, error) {
	input := &awsoc.ReadClusterInput{
		ClusterID: fi.String(clusterID),
	}

	output, err := x.svc.ReadCluster(ctx, input)
	if err != nil {
		return nil, err
	}

	return &awsOceanClusterInstanceGroup{output.Cluster}, nil
}

func (x *awsOceanClusterService) Update(ctx context.Context, group InstanceGroup) error {
	input := &awsoc.UpdateClusterInput{
		Cluster: group.Obj().(*awsoc.Cluster),
	}

	_, err := x.svc.UpdateCluster(ctx, input)
	return err
}

func (x *awsOceanClusterService) Delete(ctx context.Context, clusterID string) error {
	input := &awsoc.DeleteClusterInput{
		ClusterID: fi.String(clusterID),
	}

	_, err := x.svc.DeleteCluster(ctx, input)
	return err
}

func (x *awsOceanClusterService) Detach(ctx context.Context, clusterID, instanceID string) error {
	input := &awsoc.DetachClusterInstancesInput{
		ClusterID:                     fi.String(clusterID),
		InstanceIDs:                   []string{instanceID},
		ShouldDecrementTargetCapacity: fi.Bool(false),
		ShouldTerminateInstances:      fi.Bool(true),
	}

	_, err := x.svc.DetachClusterInstances(ctx, input)
	return err
}

func (x *awsOceanClusterService) Instances(ctx context.Context, clusterID string) ([]Instance, error) {
	input := &awsoc.ListClusterInstancesInput{
		ClusterID: fi.String(clusterID),
	}

	output, err := x.svc.ListClusterInstances(ctx, input)
	if err != nil {
		return nil, err
	}

	instances := make([]Instance, len(output.Instances))
	for i, instance := range output.Instances {
		instances[i] = &awsOceanInstance{instance}
	}

	return instances, err
}

type awsOceanLaunchSpecService struct {
	svc awsoc.Service
}

func (x *awsOceanLaunchSpecService) List(ctx context.Context) ([]InstanceGroup, error) {
	output, err := x.svc.ListLaunchSpecs(ctx, &awsoc.ListLaunchSpecsInput{})
	if err != nil {
		return nil, err
	}

	groups := make([]InstanceGroup, len(output.LaunchSpecs))
	for i, group := range output.LaunchSpecs {
		groups[i] = &awsOceanLaunchSpecInstanceGroup{group}
	}

	return groups, nil
}

func (x *awsOceanLaunchSpecService) Create(ctx context.Context, group InstanceGroup) (string, error) {
	input := &awsoc.CreateLaunchSpecInput{
		LaunchSpec: group.Obj().(*awsoc.LaunchSpec),
	}

	output, err := x.svc.CreateLaunchSpec(ctx, input)
	if err != nil {
		return "", err
	}

	return fi.StringValue(output.LaunchSpec.ID), nil
}

func (x *awsOceanLaunchSpecService) Read(ctx context.Context, specID string) (InstanceGroup, error) {
	input := &awsoc.ReadLaunchSpecInput{
		LaunchSpecID: fi.String(specID),
	}

	output, err := x.svc.ReadLaunchSpec(ctx, input)
	if err != nil {
		return nil, err
	}

	return &awsOceanLaunchSpecInstanceGroup{output.LaunchSpec}, nil
}

func (x *awsOceanLaunchSpecService) Update(ctx context.Context, group InstanceGroup) error {
	input := &awsoc.UpdateLaunchSpecInput{
		LaunchSpec: group.Obj().(*awsoc.LaunchSpec),
	}

	_, err := x.svc.UpdateLaunchSpec(ctx, input)
	return err
}

func (x *awsOceanLaunchSpecService) Delete(ctx context.Context, specID string) error {
	input := &awsoc.DeleteLaunchSpecInput{
		LaunchSpecID: fi.String(specID),
	}

	_, err := x.svc.DeleteLaunchSpec(ctx, input)
	if err != nil {
		if errs, ok := err.(client.Errors); ok {
			for _, e := range errs {
				if strings.Contains(strings.ToLower(e.Message), "ocean does not exist") {
					return nil
				}
			}
		}
		return err
	}

	return nil
}

func (x *awsOceanLaunchSpecService) Detach(ctx context.Context, specID, instanceID string) error {
	input := &awsoc.DetachClusterInstancesInput{
		ClusterID:                     fi.String(specID),
		InstanceIDs:                   []string{instanceID},
		ShouldDecrementTargetCapacity: fi.Bool(false),
		ShouldTerminateInstances:      fi.Bool(true),
	}

	_, err := x.svc.DetachClusterInstances(ctx, input)
	return err
}

func (x *awsOceanLaunchSpecService) Instances(ctx context.Context, specID string) ([]Instance, error) {
	input := &awsoc.ListClusterInstancesInput{
		ClusterID: fi.String(specID),
	}

	output, err := x.svc.ListClusterInstances(ctx, input)
	if err != nil {
		return nil, err
	}

	instances := make([]Instance, len(output.Instances))
	for i, instance := range output.Instances {
		instances[i] = &awsOceanInstance{instance}
	}

	return instances, err
}

// endregion

// region Instance Groups

func newAWSInstanceGroup(typ InstanceGroupType, obj interface{}) (InstanceGroup, error) {
	switch typ {
	case InstanceGroupElastigroup:
		return &awsElastigroupInstanceGroup{obj: obj.(*awseg.Group)}, nil
	case InstanceGroupOceanCluster:
		return &awsOceanClusterInstanceGroup{obj: obj.(*awsoc.Cluster)}, nil
	case InstanceGroupOceanLaunchSpec:
		return &awsOceanLaunchSpecInstanceGroup{obj: obj.(*awsoc.LaunchSpec)}, nil
	default:
		return nil, fmt.Errorf("spotinst: unsupported instance group type: %s", typ)
	}
}

type awsElastigroupInstanceGroup struct {
	obj *awseg.Group
}

func (x *awsElastigroupInstanceGroup) Id() string {
	return fi.StringValue(x.obj.ID)
}

func (x *awsElastigroupInstanceGroup) Type() InstanceGroupType {
	return InstanceGroupElastigroup
}

func (x *awsElastigroupInstanceGroup) Name() string {
	return fi.StringValue(x.obj.Name)
}

func (x *awsElastigroupInstanceGroup) MinSize() int {
	return fi.IntValue(x.obj.Capacity.Minimum)
}

func (x *awsElastigroupInstanceGroup) MaxSize() int {
	return fi.IntValue(x.obj.Capacity.Maximum)
}

func (x *awsElastigroupInstanceGroup) CreatedAt() time.Time {
	return spotinst.TimeValue(x.obj.CreatedAt)
}

func (x *awsElastigroupInstanceGroup) UpdatedAt() time.Time {
	return spotinst.TimeValue(x.obj.UpdatedAt)
}

func (x *awsElastigroupInstanceGroup) Obj() interface{} {
	return x.obj
}

type awsOceanClusterInstanceGroup struct {
	obj *awsoc.Cluster
}

func (x *awsOceanClusterInstanceGroup) Id() string {
	return fi.StringValue(x.obj.ID)
}

func (x *awsOceanClusterInstanceGroup) Type() InstanceGroupType {
	return InstanceGroupOceanCluster
}

func (x *awsOceanClusterInstanceGroup) Name() string {
	return fi.StringValue(x.obj.Name)
}

func (x *awsOceanClusterInstanceGroup) MinSize() int {
	return fi.IntValue(x.obj.Capacity.Minimum)
}

func (x *awsOceanClusterInstanceGroup) MaxSize() int {
	return fi.IntValue(x.obj.Capacity.Maximum)
}

func (x *awsOceanClusterInstanceGroup) CreatedAt() time.Time {
	return spotinst.TimeValue(x.obj.CreatedAt)
}

func (x *awsOceanClusterInstanceGroup) UpdatedAt() time.Time {
	return spotinst.TimeValue(x.obj.UpdatedAt)
}

func (x *awsOceanClusterInstanceGroup) Obj() interface{} {
	return x.obj
}

type awsOceanLaunchSpecInstanceGroup struct {
	obj *awsoc.LaunchSpec
}

func (x *awsOceanLaunchSpecInstanceGroup) Id() string {
	return fi.StringValue(x.obj.ID)
}

func (x *awsOceanLaunchSpecInstanceGroup) Type() InstanceGroupType {
	return InstanceGroupOceanLaunchSpec
}

func (x *awsOceanLaunchSpecInstanceGroup) Name() string {
	return fi.StringValue(x.obj.Name)
}

func (x *awsOceanLaunchSpecInstanceGroup) MinSize() int {
	return -1
}

func (x *awsOceanLaunchSpecInstanceGroup) MaxSize() int {
	return -1
}

func (x *awsOceanLaunchSpecInstanceGroup) CreatedAt() time.Time {
	return spotinst.TimeValue(x.obj.CreatedAt)
}

func (x *awsOceanLaunchSpecInstanceGroup) UpdatedAt() time.Time {
	return spotinst.TimeValue(x.obj.UpdatedAt)
}

func (x *awsOceanLaunchSpecInstanceGroup) Obj() interface{} {
	return x.obj
}

// endregion

// region Instances

type awsOceanInstance struct {
	obj *awsoc.Instance
}

func (x *awsOceanInstance) Id() string {
	return fi.StringValue(x.obj.ID)
}

func (x *awsOceanInstance) CreatedAt() time.Time {
	return spotinst.TimeValue(x.obj.CreatedAt)
}

func (x *awsOceanInstance) Obj() interface{} {
	return x.obj
}

type awsElastigroupInstance struct {
	obj *awseg.Instance
}

func (x *awsElastigroupInstance) Id() string {
	return fi.StringValue(x.obj.ID)
}

func (x *awsElastigroupInstance) CreatedAt() time.Time {
	return spotinst.TimeValue(x.obj.CreatedAt)
}

func (x *awsElastigroupInstance) Obj() interface{} {
	return x.obj
}

// endregion
