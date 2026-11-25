package bootcvm

import (
	"fmt"

	"cloud.google.com/go/compute/apiv1/computepb"
	"google.golang.org/protobuf/proto"
)

const (
	// Tag marks the VMs the firewall rules below apply to.
	Tag = "bootc-dev-vm"

	allowRule = Tag + "-allow-iap-ssh"
	denyRule  = Tag + "-deny-ssh"

	// Google's IAP TCP forwarding range:
	// https://cloud.google.com/iap/docs/using-tcp-forwarding
	iapRange = "35.235.240.0/20"

	imageFamily = "projects/ubuntu-os-cloud/global/images/family/ubuntu-2604-lts-amd64"
)

func (c Config) networkURL() string {
	return fmt.Sprintf("projects/%s/global/networks/%s", c.Project, c.Network)
}

// Instance is the VM to create.
func (c Config) Instance() *computepb.Instance {
	inst := &computepb.Instance{
		Name:        proto.String(c.Name),
		MachineType: proto.String(fmt.Sprintf("zones/%s/machineTypes/%s", c.Zone, c.MachineType)),
		Disks: []*computepb.AttachedDisk{{
			Boot:       proto.Bool(true),
			AutoDelete: proto.Bool(true),
			InitializeParams: &computepb.AttachedDiskInitializeParams{
				SourceImage: proto.String(imageFamily),
				DiskSizeGb:  proto.Int64(c.DiskGB),
				DiskType:    proto.String(fmt.Sprintf("zones/%s/diskTypes/pd-balanced", c.Zone)),
			},
		}},
		// The external IP carries outbound traffic only; the firewall rules
		// keep SSH to IAP.
		NetworkInterfaces: []*computepb.NetworkInterface{{
			Network: proto.String(c.networkURL()),
			AccessConfigs: []*computepb.AccessConfig{{
				Name: proto.String("External NAT"),
				Type: proto.String(computepb.AccessConfig_ONE_TO_ONE_NAT.String()),
			}},
		}},
		AdvancedMachineFeatures: &computepb.AdvancedMachineFeatures{
			EnableNestedVirtualization: proto.Bool(true),
		},
		Tags: &computepb.Tags{Items: []string{Tag}},
		Metadata: &computepb.Metadata{Items: []*computepb.Items{
			{Key: proto.String("enable-oslogin"), Value: proto.String("TRUE")},
		}},
		// No ServiceAccounts: the VM never calls Google APIs, so it holds no
		// credentials.
	}
	if c.Spot {
		// A preempted Spot VM stops instead of being deleted, so the disk
		// and its caches stay.
		inst.Scheduling = &computepb.Scheduling{
			ProvisioningModel:         proto.String(computepb.Scheduling_SPOT.String()),
			InstanceTerminationAction: proto.String(computepb.Scheduling_STOP.String()),
			AutomaticRestart:          proto.Bool(false),
			OnHostMaintenance:         proto.String(computepb.Scheduling_TERMINATE.String()),
		}
	}
	return inst
}

// FirewallRules apply only to VMs with Tag. default-allow-ssh on the default
// network admits 0.0.0.0/0, so one rule denies SSH from anywhere and a
// higher-priority one (lower number) allows it from IAP.
func (c Config) FirewallRules() []*computepb.Firewall {
	ssh := func() []string { return []string{"22"} }
	return []*computepb.Firewall{
		{
			Name:         proto.String(allowRule),
			Description:  proto.String("SSH to " + Tag + " VMs through IAP only"),
			Network:      proto.String(c.networkURL()),
			Direction:    proto.String(computepb.Firewall_INGRESS.String()),
			Priority:     proto.Int32(900),
			SourceRanges: []string{iapRange},
			TargetTags:   []string{Tag},
			Allowed:      []*computepb.Allowed{{IPProtocol: proto.String("tcp"), Ports: ssh()}},
		},
		{
			Name:         proto.String(denyRule),
			Description:  proto.String("Block direct SSH to " + Tag + " VMs; see " + allowRule),
			Network:      proto.String(c.networkURL()),
			Direction:    proto.String(computepb.Firewall_INGRESS.String()),
			Priority:     proto.Int32(1000),
			SourceRanges: []string{"0.0.0.0/0"},
			TargetTags:   []string{Tag},
			Denied:       []*computepb.Denied{{IPProtocol: proto.String("tcp"), Ports: ssh()}},
		},
	}
}
