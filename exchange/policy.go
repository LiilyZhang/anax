package exchange

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/open-horizon/anax/businesspolicy"
	"github.com/open-horizon/anax/exchangecommon"
	"github.com/open-horizon/anax/externalpolicy"
	"strings"
	"time"
)

// The node policy object in the exchange is identical to the node policy object
// supported by the node/policy API, so it is embedded in the ExchangeNodePolicy object.
type ExchangeNodePolicy struct {
	exchangecommon.NodePolicy
	NodePolicyVersion string `json:"nodePolicyVersion,omitempty"` // the version of the node policy
	LastUpdated       string `json:"lastUpdated,omitempty"`
}

func (e ExchangeNodePolicy) String() string {
	return fmt.Sprintf("%v, "+
		"NodePolicyVersion: %v, "+
		"LastUpdated: %v",
		e.NodePolicy, e.NodePolicyVersion, e.LastUpdated)
}

func (e ExchangeNodePolicy) ShortString() string {
	return e.String()
}

func (e ExchangeNodePolicy) DeepCopy() *ExchangeNodePolicy {
	polCopy := ExchangeNodePolicy{NodePolicy: *(e.NodePolicy.DeepCopy()), NodePolicyVersion: e.NodePolicyVersion, LastUpdated: e.LastUpdated}
	return &polCopy
}

func (e ExchangeNodePolicy) GetDeploymentPolicy() *externalpolicy.ExternalPolicy {
	return e.NodePolicy.GetDeploymentPolicy()
}

func (e ExchangeNodePolicy) GetManagementPolicy() *externalpolicy.ExternalPolicy {
	return e.NodePolicy.GetManagementPolicy()
}

func (e ExchangeNodePolicy) GetLastUpdated() string {
	return e.LastUpdated
}

// The service policy object is embedded in the ExchangeServicePolicy object.
type ExchangeServicePolicy struct {
	exchangecommon.ServicePolicy
	LastUpdated string `json:"lastUpdated,omitempty"`
}

func (e ExchangeServicePolicy) String() string {
	return fmt.Sprintf("%v, "+
		"LastUpdated: %v",
		e.ServicePolicy, e.LastUpdated)
}

func (e ExchangeServicePolicy) ShortString() string {
	return e.String()
}

func (e ExchangeServicePolicy) DeepCopy() *ExchangeServicePolicy {
	polCopy := ExchangeServicePolicy{ServicePolicy: *(e.ServicePolicy.DeepCopy()), LastUpdated: e.LastUpdated}
	return &polCopy
}

func (e ExchangeServicePolicy) GetExternalPolicy() *externalpolicy.ExternalPolicy {
	return e.ServicePolicy.GetExternalPolicy()
}

func (e ExchangeServicePolicy) GetServicePolicy() exchangecommon.ServicePolicy {
	return e.ServicePolicy
}

// the exchange business policy
type ExchangeBusinessPolicy struct {
	businesspolicy.BusinessPolicy
	Created     string `json:"created,omitempty"`
	LastUpdated string `json:"lastUpdated,omitempty"`
}

func (e ExchangeBusinessPolicy) String() string {
	return fmt.Sprintf("%v, "+
		"Created: %v, "+
		"LastUpdated: %v",
		e.BusinessPolicy, e.Created, e.LastUpdated)
}

func (e ExchangeBusinessPolicy) ShortString() string {
	return e.String()
}

func (e *ExchangeBusinessPolicy) GetBusinessPolicy() businesspolicy.BusinessPolicy {
	return e.BusinessPolicy
}

func (e *ExchangeBusinessPolicy) GetLastUpdated() string {
	return e.LastUpdated
}

func (e *ExchangeBusinessPolicy) GetCreated() string {
	return e.Created
}

type GetBusinessPolicyResponse struct {
	BusinessPolicy map[string]ExchangeBusinessPolicy `json:"businessPolicy,omitempty"` // map of all defined business policies
	LastIndex      int                               `json:"lastIndex,omitempty"`
}

// Retrieve the node policy object from the exchange. The input device Id is assumed to be prefixed with its org.
func GetNodePolicy(ec ExchangeContext, deviceId string) (*ExchangeNodePolicy, error) {
	glog.V(3).Infof(rpclogString(fmt.Sprintf("getting node policy for %v.", deviceId)))

	if cachedNodePol := GetNodePolicyFromCache(GetOrg(deviceId), GetId(deviceId)); cachedNodePol != nil {
		return cachedNodePol, nil
	}

	// Get the node policy object. There should only be 1.
	var resp interface{}
	resp = new(ExchangeNodePolicy)

	targetURL := fmt.Sprintf("%vorgs/%v/nodes/%v/policy", ec.GetExchangeURL(), GetOrg(deviceId), GetId(deviceId))

	retryCount := ec.GetHTTPFactory().RetryCount
	retryInterval := ec.GetHTTPFactory().GetRetryInterval()
	for {
		if err, tpErr := InvokeExchange(ec.GetHTTPFactory().NewHTTPClient(nil), "GET", targetURL, ec.GetExchangeId(), ec.GetExchangeToken(), nil, &resp); err != nil {
			glog.Errorf(rpclogString(fmt.Sprintf("%s", err.Error())))
			return nil, err
		} else if tpErr != nil {
			glog.Warningf(rpclogString(fmt.Sprintf("%s", tpErr.Error())))
			if ec.GetHTTPFactory().RetryCount == 0 {
				time.Sleep(time.Duration(retryInterval) * time.Second)
				continue
			} else if retryCount == 0 {
				return nil, fmt.Errorf("Exceeded %v retries for error: %v", ec.GetHTTPFactory().RetryCount, tpErr)
			} else {
				retryCount--
				time.Sleep(time.Duration(retryInterval) * time.Second)
				continue
			}
		} else {
			glog.V(5).Infof(rpclogString(fmt.Sprintf("returning node policy %v for %v.", resp, deviceId)))
			nodePolicy := resp.(*ExchangeNodePolicy)
			if nodePolicy.GetLastUpdated() == "" {
				return nil, nil
			} else {
				if nodePolicy.NodePolicyVersion == "" {
					convertedNodePol := exchangecommon.ConvertNodePolicy_v1Tov2(nodePolicy.NodePolicy.ExternalPolicy)
					convertedExchNodePol := &ExchangeNodePolicy{NodePolicy: *convertedNodePol, NodePolicyVersion: exchangecommon.NODEPOLICY_VERSION_VERSION_2, LastUpdated: nodePolicy.LastUpdated}
					UpdateCache(NodeCacheMapKey(GetOrg(deviceId), GetId(deviceId)), NODE_POL_TYPE_CACHE, *convertedExchNodePol)
					return convertedExchNodePol, nil
				} else if nodePolicy.NodePolicyVersion == exchangecommon.NODEPOLICY_VERSION_VERSION_2 {
					UpdateCache(NodeCacheMapKey(GetOrg(deviceId), GetId(deviceId)), NODE_POL_TYPE_CACHE, *nodePolicy)
					return nodePolicy, nil
				} else {
					return nil, fmt.Errorf("Unsupported node policy version %v", nodePolicy.NodePolicyVersion)
				}
			}
		}
	}
}

// Write an updated node policy to the exchange.
func PutNodePolicy(ec ExchangeContext, deviceId string, np *exchangecommon.NodePolicy) (*PutDeviceResponse, error) {
	// create PUT body
	var resp interface{}
	resp = new(PutDeviceResponse)
	targetURL := fmt.Sprintf("%vorgs/%v/nodes/%v/policy", ec.GetExchangeURL(), GetOrg(deviceId), GetId(deviceId))

	ep := &ExchangeNodePolicy{NodePolicy: *np, NodePolicyVersion: exchangecommon.NODEPOLICY_VERSION_VERSION_2}
	retryCount := ec.GetHTTPFactory().RetryCount
	retryInterval := ec.GetHTTPFactory().GetRetryInterval()
	for {
		if err, tpErr := InvokeExchange(ec.GetHTTPFactory().NewHTTPClient(nil), "PUT", targetURL, ec.GetExchangeId(), ec.GetExchangeToken(), ep, &resp); err != nil {
			return nil, err
		} else if tpErr != nil {
			glog.Warningf(rpclogString(fmt.Sprintf("%s", tpErr.Error())))
			if ec.GetHTTPFactory().RetryCount == 0 {
				time.Sleep(time.Duration(retryInterval) * time.Second)
				continue
			} else if retryCount == 0 {
				return nil, fmt.Errorf("Exceeded %v retries for error: %v", ec.GetHTTPFactory().RetryCount, tpErr)
			} else {
				retryCount--
				time.Sleep(time.Duration(retryInterval) * time.Second)
				continue
			}
		} else {
			glog.V(3).Infof(rpclogString(fmt.Sprintf("put device policy for %v to exchange %v", deviceId, ep)))
			UpdateCache(NodeCacheMapKey(GetOrg(deviceId), GetId(deviceId)), NODE_POL_TYPE_CACHE, ep)
			return resp.(*PutDeviceResponse), nil
		}
	}
}

// Delete node policy from the exchange.
// Return nil if the policy is deleted or does not exist.
func DeleteNodePolicy(ec ExchangeContext, deviceId string) error {
	// create PUT body
	var resp interface{}
	resp = new(PostDeviceResponse)
	targetURL := fmt.Sprintf("%vorgs/%v/nodes/%v/policy", ec.GetExchangeURL(), GetOrg(deviceId), GetId(deviceId))

	retryCount := ec.GetHTTPFactory().RetryCount
	retryInterval := ec.GetHTTPFactory().GetRetryInterval()
	for {
		if err, tpErr := InvokeExchange(ec.GetHTTPFactory().NewHTTPClient(nil), "DELETE", targetURL, ec.GetExchangeId(), ec.GetExchangeToken(), nil, &resp); err != nil && !strings.Contains(err.Error(), "status: 404") {
			return err
		} else if tpErr != nil {
			glog.Warningf(rpclogString(fmt.Sprintf("%s", tpErr.Error())))
			if ec.GetHTTPFactory().RetryCount == 0 {
				time.Sleep(time.Duration(retryInterval) * time.Second)
				continue
			} else if retryCount == 0 {
				return fmt.Errorf("Exceeded %v retries for error: %v", ec.GetHTTPFactory().RetryCount, tpErr)
			} else {
				retryCount--
				time.Sleep(time.Duration(retryInterval) * time.Second)
				continue
			}
		} else {
			glog.V(3).Infof(rpclogString(fmt.Sprintf("deleted device policy for %v from the exchange.", deviceId)))
			DeleteCacheResource(NODE_POL_TYPE_CACHE, NodeCacheMapKey(GetOrg(deviceId), GetId(deviceId)))
			return nil
		}
	}
}

// Get all the business policy metadata for a specific organization, and policy if specified.
func GetBusinessPolicies(ec ExchangeContext, org string, policy_id string) (map[string]ExchangeBusinessPolicy, error) {

	if policy_id == "" {
		glog.V(3).Infof(rpclogString(fmt.Sprintf("getting business policy for %v", org)))
	} else {
		glog.V(3).Infof(rpclogString(fmt.Sprintf("getting business policy for %v/%v", org, policy_id)))
	}

	var resp interface{}
	resp = new(GetBusinessPolicyResponse)

	// Search the exchange for the business policy definitions
	targetURL := ""
	if policy_id == "" {
		targetURL = fmt.Sprintf("%vorgs/%v/business/policies", ec.GetExchangeURL(), org)
	} else {
		targetURL = fmt.Sprintf("%vorgs/%v/business/policies/%v", ec.GetExchangeURL(), org, policy_id)
	}

	retryCount := ec.GetHTTPFactory().RetryCount
	retryInterval := ec.GetHTTPFactory().GetRetryInterval()
	for {
		if err, tpErr := InvokeExchange(ec.GetHTTPFactory().NewHTTPClient(nil), "GET", targetURL, ec.GetExchangeId(), ec.GetExchangeToken(), nil, &resp); err != nil {
			glog.Errorf(rpclogString(fmt.Sprintf("%s", err.Error())))
			return nil, err
		} else if tpErr != nil {
			glog.Warningf(rpclogString(fmt.Sprintf("%s", tpErr.Error())))
			if ec.GetHTTPFactory().RetryCount == 0 {
				time.Sleep(time.Duration(retryInterval) * time.Second)
				continue
			} else if retryCount == 0 {
				return nil, fmt.Errorf("Exceeded %v retries for error: %v", ec.GetHTTPFactory().RetryCount, tpErr)
			} else {
				retryCount--
				time.Sleep(time.Duration(retryInterval) * time.Second)
				continue
			}
		} else {
			var pols map[string]ExchangeBusinessPolicy
			if resp != nil {
				pols = resp.(*GetBusinessPolicyResponse).BusinessPolicy
			}

			if policy_id != "" {
				glog.V(3).Infof(rpclogString(fmt.Sprintf("found business policy for %v, %v", org, pols)))
			} else {
				glog.V(3).Infof(rpclogString(fmt.Sprintf("found %v business policies for %v", len(pols), org)))
			}
			return pols, nil
		}
	}
}
