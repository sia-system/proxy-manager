package kubehandler

import (
	"time"

	"github.com/golang/protobuf/ptypes"
	log "github.com/sirupsen/logrus"

	env_api "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	env_auth "github.com/envoyproxy/go-control-plane/envoy/api/v2/auth"
	env_core "github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	env_endpnt "github.com/envoyproxy/go-control-plane/envoy/api/v2/endpoint"
	env_lsnr "github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	env_als "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v2"
	env_alf "github.com/envoyproxy/go-control-plane/envoy/config/filter/accesslog/v2"
	env_hcm "github.com/envoyproxy/go-control-plane/envoy/config/filter/network/http_connection_manager/v2"
	env_cache "github.com/envoyproxy/go-control-plane/pkg/cache"

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

var ioKeys = certKeysPath{
	CertificateChain: "/certs/internal-sia.crt",
	PrivateKey:       "/certs/internal-sia.key",
}

// MakeEndpoint creates a localhost endpoint on a given port.
func MakeEndpoint(clusterName, ns string, endpoints []EndpointInfo) *env_api.ClusterLoadAssignment {
	lbEndpoints := make([]*env_endpnt.LbEndpoint, len(endpoints))

	for i, ep := range endpoints {

		log.Println("      endpont: ", clusterName, ns, ep.IP, ep.Port)

		lbEndpoints[i] = &env_endpnt.LbEndpoint{
			HostIdentifier: &env_endpnt.LbEndpoint_Endpoint{
				Endpoint: &env_endpnt.Endpoint{
					Address: &env_core.Address{
						Address: &env_core.Address_SocketAddress{
							SocketAddress: &env_core.SocketAddress{
								Protocol: env_core.SocketAddress_TCP,
								Address:  ep.IP,
								PortSpecifier: &env_core.SocketAddress_PortValue{
									PortValue: uint32(ep.Port),
								},
							},
						},
					},
				},
			},
		}
	}

	return &env_api.ClusterLoadAssignment{
		ClusterName: clusterName + "." + ns,
		Endpoints: []*env_endpnt.LocalityLbEndpoints{{
			LbEndpoints: lbEndpoints,
		}},
	}
}

// MakeCluster creates a cluster using either ADS or EDS.
func MakeCluster(clusterName, ns string, alpn bool, listenerIdx int) *env_api.Cluster {
	edsSource := &env_core.ConfigSource{
		ConfigSourceSpecifier: &env_core.ConfigSource_ApiConfigSource{
			ApiConfigSource: &env_core.ApiConfigSource{
				ApiType: env_core.ApiConfigSource_GRPC,
				GrpcServices: []*env_core.GrpcService{{
					TargetSpecifier: &env_core.GrpcService_EnvoyGrpc_{
						EnvoyGrpc: &env_core.GrpcService_EnvoyGrpc{ClusterName: XdsCluster},
					},
				}},
			},
		},
	}

	connectTimeout := 30 * time.Second

	tlsConfig := upstreamTLSContext(listenerIdx, alpn) // may be downstream tls context

	tlsConfigPbst, err := ptypes.MarshalAny(tlsConfig)
	if err != nil {
		panic(err)
	}

	return &env_api.Cluster{
		Name:           clusterName + "." + ns,
		ConnectTimeout: ptypes.DurationProto(connectTimeout),

		TransportSocket: &env_core.TransportSocket{
			Name: "envoy.transport_sockets.tls",
			ConfigType: &env_core.TransportSocket_TypedConfig{
				TypedConfig: tlsConfigPbst,
			},
		},

		ClusterDiscoveryType: &env_api.Cluster_Type{Type: env_api.Cluster_EDS},
		EdsClusterConfig: &env_api.Cluster_EdsClusterConfig{
			EdsConfig: edsSource,
		},
	}
}

// MakeHTTPListeners creates a HTTP listeners for a cluster (redirect for HTTP and HTTPS)
func MakeHTTPListeners() []env_cache.Resource {
	// access log service configuration

	alsConfig := &env_als.FileAccessLog{
		// Path: "dev/stdout",
		Path: "/var/log/envoy/https_access.log",
	}
	alsConfigPbst, err := ptypes.MarshalAny(alsConfig)
	if err != nil {
		panic(err)
	}

	rdsSource := env_core.ConfigSource{}
	rdsSource.ConfigSourceSpecifier = &env_core.ConfigSource_ApiConfigSource{
		ApiConfigSource: &env_core.ApiConfigSource{
			ApiType: env_core.ApiConfigSource_GRPC,
			GrpcServices: []*env_core.GrpcService{{
				TargetSpecifier: &env_core.GrpcService_EnvoyGrpc_{
					EnvoyGrpc: &env_core.GrpcService_EnvoyGrpc{ClusterName: XdsCluster},
				},
			}},
		},
	}

	listeners := make([]env_cache.Resource, 1) // for enable io listener: 2
	for i := 0; i <= 0; i++ {                  // for enable io listener: i <= 1
		routeConfigHame := calcRouteConfigName(i)
		listenerName := calcListenerName(i)
		listenerPort := calcListenerPort(i)

		// HTTP filter configuration
		httpsManager := &env_hcm.HttpConnectionManager{
			CodecType:  env_hcm.HttpConnectionManager_AUTO,
			StatPrefix: "ingress_https",
			RouteSpecifier: &env_hcm.HttpConnectionManager_Rds{
				Rds: &env_hcm.Rds{
					ConfigSource:    &rdsSource,
					RouteConfigName: routeConfigHame,
				},
			},
			HttpFilters: []*env_hcm.HttpFilter{{
				Name: wellknown.Router,
			}},
			AccessLog: []*env_alf.AccessLog{{
				// Name:   util.HTTPGRPCAccessLog,
				Name:       wellknown.FileAccessLog,
				ConfigType: &env_alf.AccessLog_TypedConfig{TypedConfig: alsConfigPbst},
			}},
		}

		httpsPbst, err := ptypes.MarshalAny(httpsManager)
		if err != nil {
			panic(err)
		}

		// HTTP listener configuration
		listeners[i] = &env_api.Listener{
			Name: listenerName,
			Address: &env_core.Address{
				Address: &env_core.Address_SocketAddress{
					SocketAddress: &env_core.SocketAddress{
						Protocol: env_core.SocketAddress_TCP,
						Address:  anyhost,
						PortSpecifier: &env_core.SocketAddress_PortValue{
							PortValue: listenerPort,
						},
					},
				},
			},
			FilterChains: []*env_lsnr.FilterChain{{
				Filters: []*env_lsnr.Filter{{
					Name:       wellknown.HTTPConnectionManager,
					ConfigType: &env_lsnr.Filter_TypedConfig{TypedConfig: httpsPbst},
				}},

				TlsContext: downstreamTLSContext(i, true),
			}},
		}
	}

	return listeners
}

// first route is ACC, second route is IO
func calcListenerName(idx int) string {
	switch idx {
	case 0:
		return "acc-listener"
	case 1:
		return "io-listener"
	}
	return ""
}

// first route is ACC, second route is IO
func calcListenerPort(idx int) uint32 {
	switch idx {
	case 0:
		return 8443
	case 1:
		return 8081
	}
	return 8443
}

func downstreamTLSContext(listenerIdx int, alpn bool) *env_auth.DownstreamTlsContext {
	return &env_auth.DownstreamTlsContext{
		CommonTlsContext: commonTLSContext(listenerIdx, alpn),
	}
}

func upstreamTLSContext(listenerIdx int, alpn bool) *env_auth.UpstreamTlsContext {
	return &env_auth.UpstreamTlsContext{
		CommonTlsContext: commonTLSContext(listenerIdx, alpn),
	}
}

func commonTLSContext(listenerIdx int, alpn bool) *env_auth.CommonTlsContext {
	var keys *certKeysPath
	switch listenerIdx {
	case 0:
		keys = &accKeys
	case 1:
		keys = &ioKeys
	}
	var alpnProtocols []string
	if alpn {
		alpnProtocols = []string{"h2", "http/1.1"}
	}
	return &env_auth.CommonTlsContext{
		AlpnProtocols: alpnProtocols,
		TlsCertificates: []*env_auth.TlsCertificate{
			&env_auth.TlsCertificate{
				CertificateChain: &env_core.DataSource{
					Specifier: &env_core.DataSource_Filename{Filename: keys.CertificateChain},
				},
				PrivateKey: &env_core.DataSource{
					Specifier: &env_core.DataSource_Filename{Filename: keys.PrivateKey},
				},
			},
		},
	}
}
