package kubehandler

import (
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	transport_sockets "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/golang/protobuf/ptypes"
	log "github.com/sirupsen/logrus"

	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
)

type certKeysPath struct {
	CertificateChain string
	PrivateKey       string
}

// Certificates for internal IO
var accKeys = certKeysPath{
	CertificateChain: "/certs/internal.crt",
	PrivateKey:       "/certs/internal.key",
}

// MakeEndpoint creates a localhost endpoint on a given port.
func MakeEndpoint(clusterName, ns string, endpoints []EndpointInfo) *endpoint.ClusterLoadAssignment {
	lbEndpoints := make([]*endpoint.LbEndpoint, len(endpoints))

	for i, ep := range endpoints {

		log.Println("      endpont: ", clusterName, ns, ep.IP, ep.Port)

		lbEndpoints[i] = &endpoint.LbEndpoint{
			HostIdentifier: &endpoint.LbEndpoint_Endpoint{
				Endpoint: &endpoint.Endpoint{
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Protocol: core.SocketAddress_TCP,
								Address:  ep.IP,
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: uint32(ep.Port),
								},
							},
						},
					},
				},
			},
		}
	}

	return &endpoint.ClusterLoadAssignment{
		ClusterName: clusterName + "." + ns,
		Endpoints: []*endpoint.LocalityLbEndpoints{{
			LbEndpoints: lbEndpoints,
		}},
	}
}

// MakeCluster creates a cluster using either ADS or EDS.
func MakeCluster(clusterName, ns string, alpn bool) *cluster.Cluster {
	edsSource := &core.ConfigSource{
		ConfigSourceSpecifier: &core.ConfigSource_ApiConfigSource{
			ApiConfigSource: &core.ApiConfigSource{
				ApiType: core.ApiConfigSource_GRPC,
				TransportApiVersion: core.ApiVersion_V3,
				GrpcServices: []*core.GrpcService{{
					TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &core.GrpcService_EnvoyGrpc{ClusterName: XdsCluster},
					},
				}},
			},
		},
		ResourceApiVersion: core.ApiVersion_V3,
	}

	connectTimeout := 30 * time.Second

	tlsConfig := upstreamTLSContext(alpn) // may be downstream tls context

	tlsConfigPbst, err := ptypes.MarshalAny(tlsConfig)
	if err != nil {
		panic(err)
	}

	return &cluster.Cluster{
		Name:           clusterName + "." + ns,
		ConnectTimeout: ptypes.DurationProto(connectTimeout),

		TransportSocket: &core.TransportSocket{
			Name: "envoy.transport_sockets.tls",
			ConfigType: &core.TransportSocket_TypedConfig{
				TypedConfig: tlsConfigPbst,
			},
		},

		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_EDS},
		EdsClusterConfig: &cluster.Cluster_EdsClusterConfig{
			EdsConfig: edsSource,
		},
	}
}

// MakeHTTPListeners creates a HTTP listeners for a cluster (redirect for HTTP and HTTPS)
func MakeHTTPListener() *listener.Listener {
	// make config source
	source := &core.ConfigSource{}
	source.ResourceApiVersion = resource.DefaultAPIVersion
	source.ConfigSourceSpecifier = &core.ConfigSource_ApiConfigSource{
		ApiConfigSource: &core.ApiConfigSource{
			TransportApiVersion:       resource.DefaultAPIVersion,
			ApiType:                   core.ApiConfigSource_GRPC,
			SetNodeOnFirstMessageOnly: true,
			GrpcServices: []*core.GrpcService{{
				TargetSpecifier: &core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &core.GrpcService_EnvoyGrpc{ClusterName: "xds_cluster"},
				},
			}},
		},
	}

	// HTTP filter configuration
	httpsManager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "ingress_https",
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				ConfigSource:    source,
				RouteConfigName: "acc-routes",
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
		}},
	}

	httpsPbst, err := ptypes.MarshalAny(httpsManager)
	if err != nil {
		panic(err)
	}

	downstreamPbst, err := ptypes.MarshalAny(downstreamTLSContext(true))
	if err != nil {
		panic(err)
	}

	return &listener.Listener{
		Name: "acc-listener",
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  anyhost,
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: 8443,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name:       wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{TypedConfig: httpsPbst},
			}},

			TransportSocket: &core.TransportSocket{
				Name: "https-transport",
				ConfigType: &core.TransportSocket_TypedConfig{
					TypedConfig: downstreamPbst,
				},
			},
		}},
	}
}

func downstreamTLSContext(alpn bool) *transport_sockets.DownstreamTlsContext {
	return &transport_sockets.DownstreamTlsContext{
		CommonTlsContext: commonTLSContext(alpn),
	}
}

func upstreamTLSContext(alpn bool) *transport_sockets.UpstreamTlsContext {
	return &transport_sockets.UpstreamTlsContext{
		CommonTlsContext: commonTLSContext(alpn),
	}
}

func commonTLSContext(alpn bool) *transport_sockets.CommonTlsContext {
	var alpnProtocols []string
	if alpn {
		alpnProtocols = []string{"h2", "http/1.1"}
	}
	return &transport_sockets.CommonTlsContext{
		AlpnProtocols: alpnProtocols,
		TlsCertificates: []*transport_sockets.TlsCertificate{
			{
				CertificateChain: &core.DataSource{
					Specifier: &core.DataSource_Filename{Filename: accKeys.CertificateChain},
				},
				PrivateKey: &core.DataSource{
					Specifier: &core.DataSource_Filename{Filename: accKeys.PrivateKey},
				},
			},
		},
	}
}
