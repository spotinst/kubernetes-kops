package spotinsttasks

import (
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go/service/ec2"
	awseg "github.com/spotinst/spotinst-sdk-go/service/elastigroup/providers/aws"
	awsoc "github.com/spotinst/spotinst-sdk-go/service/ocean/providers/aws"
	"k8s.io/klog"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/awstasks"
	"k8s.io/kops/upup/pkg/fi/cloudup/awsup"
	"k8s.io/kops/upup/pkg/fi/utils"
)

func buildElastigroupTags(tags map[string]string) []*awseg.Tag {
	out := make([]*awseg.Tag, 0, len(tags))

	for key, value := range tags {
		out = append(out, &awseg.Tag{
			Key:   fi.String(key),
			Value: fi.String(value),
		})
	}

	return out
}

func buildOceanTags(tags map[string]string) []*awsoc.Tag {
	out := make([]*awsoc.Tag, 0, len(tags))

	for key, value := range tags {
		out = append(out, &awsoc.Tag{
			Key:   fi.String(key),
			Value: fi.String(value),
		})
	}

	return out
}

func buildAutoScaleLabels(labels map[string]string) []*awseg.AutoScaleLabel {
	out := make([]*awseg.AutoScaleLabel, 0, len(labels))

	for key, value := range labels {
		out = append(out, &awseg.AutoScaleLabel{
			Key:   fi.String(key),
			Value: fi.String(value),
		})
	}

	return out
}

func buildEphemeralDevices(instanceTypeName *string) (map[string]*awstasks.BlockDeviceMapping, error) {
	if instanceTypeName == nil {
		return nil, fi.RequiredField("InstanceType")
	}

	instanceType, err := awsup.GetMachineTypeInfo(*instanceTypeName)
	if err != nil {
		return nil, err
	}

	blockDeviceMappings := make(map[string]*awstasks.BlockDeviceMapping)
	for _, ed := range instanceType.EphemeralDevices() {
		m := &awstasks.BlockDeviceMapping{
			VirtualName: fi.String(ed.VirtualName),
		}
		blockDeviceMappings[ed.DeviceName] = m
	}

	return blockDeviceMappings, nil
}

func buildRootDevice(cloud awsup.AWSCloud, imageID *string, opts *RootVolumeOpts) (map[string]*awstasks.BlockDeviceMapping, error) {
	image, err := resolveImage(cloud, fi.StringValue(imageID))
	if err != nil {
		return nil, err
	}

	rootDeviceName := fi.StringValue(image.RootDeviceName)
	blockDeviceMappings := make(map[string]*awstasks.BlockDeviceMapping)

	rootDeviceMapping := &awstasks.BlockDeviceMapping{
		EbsDeleteOnTermination: fi.Bool(true),
		EbsVolumeSize:          fi.Int64(int64(fi.Int32Value(opts.Size))),
		EbsVolumeType:          opts.Type,
	}

	// The parameter IOPS is not supported for gp2 volumes.
	if opts.IOPS != nil && fi.StringValue(opts.Type) != "gp2" {
		rootDeviceMapping.EbsVolumeIops = fi.Int64(int64(fi.Int32Value(opts.IOPS)))
	}

	blockDeviceMappings[rootDeviceName] = rootDeviceMapping

	return blockDeviceMappings, nil
}

func buildBlockDeviceMapping(deviceName string, i *awstasks.BlockDeviceMapping) *awseg.BlockDeviceMapping {
	o := &awseg.BlockDeviceMapping{}
	o.DeviceName = fi.String(deviceName)
	o.VirtualName = i.VirtualName

	if i.EbsDeleteOnTermination != nil || i.EbsVolumeSize != nil || i.EbsVolumeType != nil {
		o.EBS = &awseg.EBS{}
		o.EBS.DeleteOnTermination = i.EbsDeleteOnTermination
		o.EBS.VolumeSize = fi.Int(int(fi.Int64Value(i.EbsVolumeSize)))
		o.EBS.VolumeType = i.EbsVolumeType

		// The parameter IOPS is not supported for gp2 volumes.
		if i.EbsVolumeIops != nil && fi.StringValue(i.EbsVolumeType) != "gp2" {
			o.EBS.IOPS = fi.Int(int(fi.Int64Value(i.EbsVolumeIops)))
		}
	}

	return o
}

func buildOceanLabels(labels map[string]string) []*awsoc.Label {
	out := make([]*awsoc.Label, 0, len(labels))

	for key, value := range labels {
		out = append(out, &awsoc.Label{
			Key:   fi.String(key),
			Value: fi.String(value),
		})
	}

	return out
}

func buildOceanTaints(taints []string) ([]*awsoc.Taint, error) {
	re, err := regexp.Compile(`(?P<Key>.+)\=(?P<Value>.+)\:(?P<Effect>.+)`)
	if err != nil {
		return nil, err
	}

	var out []*awsoc.Taint
	for _, t := range taints {
		taint := new(awsoc.Taint)
		match := re.FindStringSubmatch(t)

		for i, name := range re.SubexpNames() {
			if i > 0 && i <= len(match) {
				switch name {
				case "Key":
					taint.Key = fi.String(match[i])
				case "Value":
					taint.Value = fi.String(match[i])
				case "Effect":
					taint.Effect = fi.String(match[i])
				}
			}
		}

		if taint.Key != nil && taint.Value != nil && taint.Effect != nil {
			out = append(out, taint)
		}
	}

	return out, nil
}

func resolveImage(cloud awsup.AWSCloud, name string) (*ec2.Image, error) {
	image, err := cloud.ResolveImage(name)
	if err != nil {
		return nil, fmt.Errorf("spotinst: unable to resolve image %q: %v", name, err)
	} else if image == nil {
		return nil, fmt.Errorf("spotinst: unable to resolve image %q: not found", name)
	}

	return image, nil
}

func subnetSlicesEqualIgnoreOrder(l, r []*awstasks.Subnet) bool {
	var lIDs []string
	for _, s := range l {
		lIDs = append(lIDs, *s.ID)
	}

	var rIDs []string
	for _, s := range r {
		if s.ID == nil {
			klog.V(4).Infof("Subnet ID not set; returning not-equal: %v", s)
			return false
		}
		rIDs = append(rIDs, *s.ID)
	}

	return utils.StringSlicesEqualIgnoreOrder(lIDs, rIDs)
}

type Orientation string

const (
	OrientationBalanced              Orientation = "balanced"
	OrientationCost                  Orientation = "costOriented"
	OrientationAvailability          Orientation = "availabilityOriented"
	OrientationEqualZoneDistribution Orientation = "equalAzDistribution"
)

func normalizeOrientation(orientation *string) Orientation {
	out := OrientationBalanced

	// Fast path.
	if orientation == nil {
		return out
	}

	switch *orientation {
	case "cost":
		out = OrientationCost
	case "availability":
		out = OrientationAvailability
	case "equal-distribution":
		out = OrientationEqualZoneDistribution
	}

	return out
}
