/*
Copyright 2016 The Kubernetes Authors.

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

package azure

import (
	"fmt"
	"strings"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/cloudprovider"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
)

const (
	loadBalancerMinimumPriority = 500
	loadBalancerMaximumPriority = 4096

	machineResourceIDTemplate   = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s"
	frontendIPConfigIDTemplate  = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s"
	backendPoolIDTemplate       = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s"
	loadBalancerRuleIDTemplate  = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/loadBalancingRules/%s"
	loadBalancerProbeIDTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/probes/%s"
	securityRuleIDTemplate      = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s/securityRules/%s"
)

// returns the full identifier of a machine
func (az *Cloud) getMachineID(machineName string) string {
	return fmt.Sprintf(
		machineResourceIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		machineName)
}

// returns the full identifier of a loadbalancer frontendipconfiguration.
func (az *Cloud) getFrontendIPConfigID(lbName, backendPoolName string) string {
	return fmt.Sprintf(
		frontendIPConfigIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		lbName,
		backendPoolName)
}

// returns the full identifier of a loadbalancer backendpool.
func (az *Cloud) getBackendPoolID(lbName, backendPoolName string) string {
	return fmt.Sprintf(
		backendPoolIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		lbName,
		backendPoolName)
}

// returns the full identifier of a loadbalancer rule.
func (az *Cloud) getLoadBalancerRuleID(lbName, lbRuleName string) string {
	return fmt.Sprintf(
		loadBalancerRuleIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		lbName,
		lbRuleName)
}

// returns the full identifier of a loadbalancer probe.
func (az *Cloud) getLoadBalancerProbeID(lbName, lbRuleName string) string {
	return fmt.Sprintf(
		loadBalancerProbeIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		lbName,
		lbRuleName)
}

// returns the full identifier of a network security group security rule.
func (az *Cloud) getSecurityRuleID(securityRuleName string) string {
	return fmt.Sprintf(
		securityRuleIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		az.SecurityGroupName,
		securityRuleName)
}

// returns the deepest child's identifier from a full identifier string.
func getLastSegment(ID string) (string, error) {
	parts := strings.Split(ID, "/")
	name := parts[len(parts)-1]
	if len(name) == 0 {
		return "", fmt.Errorf("resource name was missing from identifier")
	}

	return name, nil
}

// returns the equivalent LoadBalancerRule, SecurityRule and LoadBalancerProbe
// protocol types for the given Kubernetes protocol type.
func getProtocolsFromKubernetesProtocol(protocol api.Protocol) (network.TransportProtocol, network.SecurityRuleProtocol, network.ProbeProtocol, error) {
	switch protocol {
	case api.ProtocolTCP:
		return network.TransportProtocolTCP, network.TCP, network.ProbeProtocolTCP, nil
	default:
		return "", "", "", fmt.Errorf("Only TCP is supported for Azure LoadBalancers")
	}
}

// This returns the full identifier of the primary NIC for the given VM.
func getPrimaryInterfaceID(machine compute.VirtualMachine) (string, error) {
	if len(*machine.Properties.NetworkProfile.NetworkInterfaces) == 1 {
		return *(*machine.Properties.NetworkProfile.NetworkInterfaces)[0].ID, nil
	}

	for _, ref := range *machine.Properties.NetworkProfile.NetworkInterfaces {
		if *ref.Properties.Primary {
			return *ref.ID, nil
		}
	}

	return "", fmt.Errorf("failed to find a primary nic for the vm. vmname=%q", *machine.Name)
}

func getPrimaryIPConfig(nic network.Interface) (*network.InterfaceIPConfiguration, error) {
	if len(*nic.Properties.IPConfigurations) == 1 {
		return &((*nic.Properties.IPConfigurations)[0]), nil
	}

	// we're here because we either have multiple ipconfigs and can't determine the primary:
	//   https://github.com/Azure/azure-rest-api-specs/issues/305
	// or somehow we had zero ipconfigs
	return nil, fmt.Errorf("failed to determine the determine primary ipconfig. nicname=%q", *nic.Name)
}

func getLoadBalancerName(clusterName string) string {
	return clusterName
}

func getBackendPoolName(clusterName string) string {
	return clusterName
}

func getRuleName(service *api.Service, port api.ServicePort) string {
	return fmt.Sprintf("%s-%s-%d-%d", getRulePrefix(service), port.Protocol, port.Port, port.NodePort)
}

// This returns a human-readable version of the Service used to tag some resources.
// This is only used for human-readable convenience, and not to filter.
func getServiceName(service *api.Service) string {
	return fmt.Sprintf("%s/%s", service.Namespace, service.Name)
}

// This returns a prefix for loadbalancer/security rules.
func getRulePrefix(service *api.Service) string {
	return cloudprovider.GetLoadBalancerName(service)
}

func serviceOwnsRule(service *api.Service, rule string) bool {
	prefix := getRulePrefix(service)
	return strings.HasPrefix(strings.ToUpper(rule), strings.ToUpper(prefix))
}

func getFrontendIPConfigName(service *api.Service) string {
	return cloudprovider.GetLoadBalancerName(service)
}

func getPublicIPName(clusterName string, service *api.Service) string {
	return fmt.Sprintf("%s-%s", clusterName, cloudprovider.GetLoadBalancerName(service))
}

// This returns the next available rule priority level for a given set of security rules.
func getNextAvailablePriority(rules []network.SecurityRule) (int32, error) {
	var smallest int32 = loadBalancerMinimumPriority
	var spread int32 = 1

outer:
	for smallest < loadBalancerMaximumPriority {
		for _, rule := range rules {
			if *rule.Properties.Priority == smallest {
				smallest += spread
				continue outer
			}
		}
		// no one else had it
		return smallest, nil
	}

	return -1, fmt.Errorf("SecurityGroup priorities are exhausted")
}

func (az *Cloud) getIPForMachine(machineName string) (string, error) {
	machine, exists, err := az.getVirtualMachine(machineName)
	if !exists {
		return "", cloudprovider.InstanceNotFound
	}
	if err != nil {
		return "", err
	}

	nicID, err := getPrimaryInterfaceID(machine)
	if err != nil {
		return "", err
	}

	nicName, err := getLastSegment(nicID)
	if err != nil {
		return "", err
	}

	nic, err := az.InterfacesClient.Get(az.ResourceGroup, nicName, "")
	if err != nil {
		return "", err
	}

	ipConfig, err := getPrimaryIPConfig(nic)
	if err != nil {
		return "", err
	}

	targetIP := *ipConfig.Properties.PrivateIPAddress
	return targetIP, nil
}
