package config

import (
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	pb "github.com/linkerd/linkerd2/controller/gen/config"
	l5dcharts "github.com/linkerd/linkerd2/pkg/charts/linkerd2"
	log "github.com/sirupsen/logrus"
)

// Global returns the Global protobuf config from the linkerd-config ConfigMap
func Global(filepath string) (*pb.Global, error) {
	config := &pb.Global{}
	err := unmarshalFile(filepath, config)
	return config, err
}

// Proxy returns the Proxy protobuf config from the linkerd-config ConfigMap
func Proxy(filepath string) (*pb.Proxy, error) {
	config := &pb.Proxy{}
	err := unmarshalFile(filepath, config)
	return config, err
}

// Install returns the Install protobuf config from the linkerd-config ConfigMap
func Install(filepath string) (*pb.Install, error) {
	config := &pb.Install{}
	err := unmarshalFile(filepath, config)
	return config, err
}

func unmarshalFile(filepath string, msg proto.Message) error {
	configJSON, err := ioutil.ReadFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %s", err)
	}

	log.Debugf("%s config JSON: %s", filepath, configJSON)
	if err = unmarshal(string(configJSON), msg); err != nil {
		return fmt.Errorf("failed to unmarshal JSON from: %s: %s", filepath, err)
	}

	return nil
}

func unmarshal(json string, msg proto.Message) error {
	// If a config is missing, then just leave the message as nil and return
	// without an error.
	if json == "" {
		return nil
	}

	// If we're using older code to read a newer config, blowing up during decoding
	// is not helpful. We should detect that through other means.
	u := jsonpb.Unmarshaler{AllowUnknownFields: true}
	return u.Unmarshal(strings.NewReader(json), msg)
}

// FromConfigMap builds a configuration by reading a map with the keys "global"
// and "proxy", each containing JSON values.
func FromConfigMap(configMap map[string]string) (*pb.All, error) {
	c := &pb.All{Global: &pb.Global{}, Proxy: &pb.Proxy{}, Install: &pb.Install{}}

	if err := unmarshal(configMap["global"], c.Global); err != nil {
		return nil, fmt.Errorf("invalid global config: %s", err)
	}

	if err := unmarshal(configMap["proxy"], c.Proxy); err != nil {
		return nil, fmt.Errorf("invalid proxy config: %s", err)
	}

	if err := unmarshal(configMap["install"], c.Install); err != nil {
		return nil, fmt.Errorf("invalid install config: %s", err)
	}

	return c, nil
}

// ToJSON encode the configuration to JSON, i.e. to be stored in a ConfigMap.
func ToJSON(configs *pb.All) (global, proxy, install string, err error) {
	m := jsonpb.Marshaler{EmitDefaults: true}

	global, err = m.MarshalToString(configs.GetGlobal())
	if err != nil {
		return
	}

	proxy, err = m.MarshalToString(configs.GetProxy())
	if err != nil {
		return
	}

	install, err = m.MarshalToString(configs.GetInstall())
	return
}

// ToValues converts configuration into a Values struct, i.e to be consumed by check
// TODO: Remove this once the newer configuration becomes the default i.e 2.10
func ToValues(configs *pb.All) *l5dcharts.Values {

	// convert install flags into values
	values := &l5dcharts.Values{
		Global: &l5dcharts.Global{
			CNIEnabled:              configs.GetGlobal().GetCniEnabled(),
			Namespace:               configs.GetGlobal().GetLinkerdNamespace(),
			IdentityTrustAnchorsPEM: configs.GetGlobal().GetIdentityContext().GetTrustAnchorsPem(),
			IdentityTrustDomain:     configs.GetGlobal().GetIdentityContext().GetTrustDomain(),
			ClusterDomain:           configs.GetGlobal().GetClusterDomain(),
			LinkerdVersion:          configs.GetGlobal().GetVersion(),
			Proxy: &l5dcharts.Proxy{
				Image: &l5dcharts.Image{
					Name:       configs.GetProxy().GetProxyImage().GetImageName(),
					PullPolicy: configs.GetProxy().GetProxyImage().GetPullPolicy(),
					Version:    configs.GetProxy().GetProxyVersion(),
				},
				Ports: &l5dcharts.Ports{
					Control:  int32(configs.GetProxy().GetControlPort().GetPort()),
					Inbound:  int32(configs.GetProxy().GetInboundPort().GetPort()),
					Admin:    int32(configs.GetProxy().GetAdminPort().GetPort()),
					Outbound: int32(configs.GetProxy().GetOutboundPort().GetPort()),
				},
				Resources: &l5dcharts.Resources{
					CPU: l5dcharts.Constraints{
						Limit:   configs.GetProxy().GetResource().GetLimitCpu(),
						Request: configs.GetProxy().GetResource().GetRequestCpu(),
					},
					Memory: l5dcharts.Constraints{
						Limit:   configs.GetProxy().GetResource().GetLimitMemory(),
						Request: configs.GetProxy().GetResource().GetRequestMemory(),
					},
				},
				EnableExternalProfiles: !configs.Proxy.GetDisableExternalProfiles(),
				DestinationGetNetworks: configs.GetProxy().GetDestinationGetNetworks(),
				LogFormat:              configs.GetProxy().GetLogFormat(),
				OutboundConnectTimeout: configs.GetProxy().GetOutboundConnectTimeout(),
				InboundConnectTimeout:  configs.GetProxy().GetInboundConnectTimeout(),
			},
			ProxyInit: &l5dcharts.ProxyInit{
				IgnoreInboundPorts:  toString(configs.GetProxy().GetIgnoreInboundPorts()),
				IgnoreOutboundPorts: toString(configs.GetProxy().GetIgnoreOutboundPorts()),
				Image: &l5dcharts.Image{
					Name:       configs.GetProxy().GetProxyInitImage().GetImageName(),
					PullPolicy: configs.GetProxy().GetProxyInitImage().GetPullPolicy(),
					Version:    configs.GetProxy().GetProxyInitImageVersion(),
				},
			},
		},
		Identity: &l5dcharts.Identity{
			Issuer: &l5dcharts.Issuer{
				Scheme: configs.GetGlobal().GetIdentityContext().GetScheme(),
			},
		},
		OmitWebhookSideEffects: configs.GetGlobal().GetOmitWebhookSideEffects(),
		DebugContainer: &l5dcharts.DebugContainer{
			Image: &l5dcharts.Image{
				Name:       configs.GetProxy().GetDebugImage().GetImageName(),
				PullPolicy: configs.GetProxy().GetDebugImage().GetPullPolicy(),
				Version:    configs.GetProxy().GetDebugImageVersion(),
			},
		},
	}

	// for non-primitive types set only if they are not nil
	if configs.GetGlobal().GetIdentityContext().GetIssuanceLifetime() != nil {
		values.Identity.Issuer.IssuanceLifetime = configs.GetGlobal().GetIdentityContext().GetIssuanceLifetime().String()
	}
	if configs.GetGlobal().GetIdentityContext().GetClockSkewAllowance() != nil {
		values.Identity.Issuer.ClockSkewAllowance = configs.GetGlobal().GetIdentityContext().GetClockSkewAllowance().String()
	}

	if configs.GetProxy().GetLogLevel() != nil {
		values.Global.Proxy.LogLevel = configs.GetProxy().GetLogLevel().String()

	}

	// set HA, and Heartbeat flags as health-check needs them for old config installs
	for _, flag := range configs.GetInstall().GetFlags() {
		if flag.GetName() == "ha" && flag.GetValue() == "true" {
			values.Global.HighAvailability = true
		}

		if flag.GetName() == "disable-heartbeat" && flag.GetValue() == "true" {
			values.DisableHeartBeat = true
		}
	}

	return values
}

func toString(portRanges []*pb.PortRange) string {

	var portRangeString string
	if len(portRanges) > 0 {
		for i := 0; i < len(portRanges)-1; i++ {
			portRangeString += portRanges[i].GetPortRange() + ","
		}

		portRangeString += portRanges[len(portRanges)-1].GetPortRange()
	}

	return portRangeString
}
