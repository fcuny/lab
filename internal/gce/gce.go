// Package gce wraps the Compute Engine calls the lab recipes need. It
// authenticates with the operator's gcloud login and uses gcloud for SSH,
// which handles OS Login keys and IAP tunnels.
package gce

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Client acts on one project and zone.
type Client struct {
	Project string
	Zone    string

	instances *compute.InstancesClient
	firewalls *compute.FirewallsClient
}

// New returns a client that authenticates with the gcloud login.
func New(ctx context.Context, project, zone string) (*Client, error) {
	auth := option.WithTokenSource(gcloudTokenSource())
	instances, err := compute.NewInstancesRESTClient(ctx, auth)
	if err != nil {
		return nil, err
	}
	firewalls, err := compute.NewFirewallsRESTClient(ctx, auth)
	if err != nil {
		instances.Close()
		return nil, err
	}
	return &Client{Project: project, Zone: zone, instances: instances, firewalls: firewalls}, nil
}

// Close releases the API connections.
func (c *Client) Close() error {
	return errors.Join(c.instances.Close(), c.firewalls.Close())
}

func isNotFound(err error) bool {
	var apiErr *apierror.APIError
	return errors.As(err, &apiErr) && apiErr.HTTPCode() == http.StatusNotFound
}

// InstanceStatus returns the instance's status (RUNNING, TERMINATED, ...),
// or "" if it does not exist.
func (c *Client) InstanceStatus(ctx context.Context, name string) (string, error) {
	inst, err := c.instances.Get(ctx, &computepb.GetInstanceRequest{
		Project: c.Project, Zone: c.Zone, Instance: name,
	})
	if isNotFound(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return inst.GetStatus(), nil
}

// CreateInstance creates the instance and waits for the operation.
func (c *Client) CreateInstance(ctx context.Context, inst *computepb.Instance) error {
	op, err := c.instances.Insert(ctx, &computepb.InsertInstanceRequest{
		Project: c.Project, Zone: c.Zone, InstanceResource: inst,
	})
	if err != nil {
		return fmt.Errorf("create instance %s: %w", inst.GetName(), err)
	}
	return op.Wait(ctx)
}

// StartInstance starts the instance and waits for the operation.
func (c *Client) StartInstance(ctx context.Context, name string) error {
	op, err := c.instances.Start(ctx, &computepb.StartInstanceRequest{
		Project: c.Project, Zone: c.Zone, Instance: name,
	})
	if err != nil {
		return fmt.Errorf("start instance %s: %w", name, err)
	}
	return op.Wait(ctx)
}

// StopInstance stops the instance and waits for the operation.
func (c *Client) StopInstance(ctx context.Context, name string) error {
	op, err := c.instances.Stop(ctx, &computepb.StopInstanceRequest{
		Project: c.Project, Zone: c.Zone, Instance: name,
	})
	if err != nil {
		return fmt.Errorf("stop instance %s: %w", name, err)
	}
	return op.Wait(ctx)
}

// DeleteInstance deletes the instance, and its boot disk if the disk is set
// to auto-delete, and waits for the operation.
func (c *Client) DeleteInstance(ctx context.Context, name string) error {
	op, err := c.instances.Delete(ctx, &computepb.DeleteInstanceRequest{
		Project: c.Project, Zone: c.Zone, Instance: name,
	})
	if err != nil {
		return fmt.Errorf("delete instance %s: %w", name, err)
	}
	return op.Wait(ctx)
}

// InstancesWithTag returns the names of the project's instances, in any
// zone, that carry the network tag.
func (c *Client) InstancesWithTag(ctx context.Context, tag string) ([]string, error) {
	var names []string
	it := c.instances.AggregatedList(ctx, &computepb.AggregatedListInstancesRequest{Project: c.Project})
	for {
		pair, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return names, nil
		}
		if err != nil {
			return nil, err
		}
		for _, inst := range pair.Value.GetInstances() {
			for _, t := range inst.GetTags().GetItems() {
				if t == tag {
					names = append(names, inst.GetName())
					break
				}
			}
		}
	}
}

// FirewallExists reports whether the project has a firewall rule by that name.
func (c *Client) FirewallExists(ctx context.Context, name string) (bool, error) {
	_, err := c.firewalls.Get(ctx, &computepb.GetFirewallRequest{Project: c.Project, Firewall: name})
	if isNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

// CreateFirewall creates the rule and waits for the operation.
func (c *Client) CreateFirewall(ctx context.Context, rule *computepb.Firewall) error {
	op, err := c.firewalls.Insert(ctx, &computepb.InsertFirewallRequest{
		Project: c.Project, FirewallResource: rule,
	})
	if err != nil {
		return fmt.Errorf("create firewall rule %s: %w", rule.GetName(), err)
	}
	return op.Wait(ctx)
}

// DeleteFirewall deletes the rule and waits for the operation.
func (c *Client) DeleteFirewall(ctx context.Context, name string) error {
	op, err := c.firewalls.Delete(ctx, &computepb.DeleteFirewallRequest{Project: c.Project, Firewall: name})
	if err != nil {
		return fmt.Errorf("delete firewall rule %s: %w", name, err)
	}
	return op.Wait(ctx)
}
