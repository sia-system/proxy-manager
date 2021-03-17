// Copyright 2018 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.

// Package resource creates test xDS resources
package kubehandler

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	log "github.com/sirupsen/logrus"

	cache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	types "github.com/envoyproxy/go-control-plane/pkg/cache/types"

	v1 "k8s.io/api/core/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	proxyconf "demius.md/proxy-manager/proxy-config"
)

const (
	anyhost   = "0.0.0.0"
	localhost = "127.0.0.1"
	// XdsCluster is the cluster name for the control server (used by non-ADS set-up)
	XdsCluster       = "xds_cluster"
)

// KubeHandler connect envoy-proxy management with k8s info
type KubeHandler struct {
	snapshotVersion uint64
	mu              *sync.Mutex

	coreInterface corev1.CoreV1Interface
	snapshotCache cache.SnapshotCache

	namespaces map[string]Namespace
	listener  *listener.Listener
}

// Namespace map of k8s service-names -> config
type Namespace = map[string]ServiceInfo

// ServiceInfo k8s service routing envoy-proxy config
type ServiceInfo struct {
	ServiceName string
	ProxyConfig *proxyconf.Config
	// node-name -> IO
	Endpoints map[string]EndpointInfo
}

// EndpointInfo logical socket address of endpoint
type EndpointInfo struct {
	IP   string
	Port int32
}

// QualifiedServiceInfo contains service info with namespace
type QualifiedServiceInfo struct {
	Namespace   string
	ServiceName string
	Alpn        bool
	ProxyConfig *proxyconf.Config
	Endpoints   []EndpointInfo
}

// New create new KubeHandler
func New(snapshotCache cache.SnapshotCache, coreInterface corev1.CoreV1Interface) *KubeHandler {
	var snapshotVersion uint64
	namespaces := map[string]Namespace{}

	mu := &sync.Mutex{}
	listener := MakeHTTPListener()

	info := KubeHandler{snapshotVersion, mu, coreInterface, snapshotCache, namespaces, listener}

	snapshotCache.SetSnapshot("envoy-proxy", info.Generate())

	return &info
}

// ResourceChangeType kinds of resource changing
type ResourceChangeType = int

const (
	// ResourceCreated when service or endpoint created
	ResourceCreated ResourceChangeType = iota + 1
	// ResourceDeleted when service or endpoint deleted
	ResourceDeleted
)

// ChangeTypeString convert changeType enum to string representation
func ChangeTypeString(changeType ResourceChangeType) string {
	if changeType == ResourceCreated {
		return "created"
	}
	return "deleted"
}

// HandleServiceStatusChange handle creation or deletion of service
func (handler *KubeHandler) HandleServiceStatusChange(srv *v1.Service, key string, changeType ResourceChangeType) {
	ct := ChangeTypeString(changeType)

	namespace := srv.Namespace
	name := srv.ObjectMeta.Name
	annotations := srv.Annotations

	log.Printf("Srv: %s.%s -> %s", name, namespace, ct)

	if ariaProxyConfig, ok := annotations["aria.io/proxy-config"]; ok {
		log.Printf("   Config: %s", ariaProxyConfig)
		proxyconfig, err := proxyconf.Parse([]byte(ariaProxyConfig))
		if err == nil {
			ns, ok := handler.namespaces[srv.Namespace]
			if changeType == ResourceCreated {
				// service created
				var endpoints map[string]EndpointInfo
				if ok {
					// namespace exists
					srvInfo, ok := ns[srv.ObjectMeta.Name]
					if ok {
						// service exists
						endpoints = srvInfo.Endpoints
					} else {
						endpoints = map[string]EndpointInfo{}
					}
				} else {
					// new namespace
					ns = Namespace{}
					handler.namespaces[srv.Namespace] = ns
					endpoints = map[string]EndpointInfo{}
				}
				ns[srv.ObjectMeta.Name] =
					ServiceInfo{
						ServiceName: srv.ObjectMeta.Name,
						ProxyConfig: proxyconfig,
						Endpoints:   endpoints,
					}
			} else {
				// service deleted
				ns, ok := handler.namespaces[srv.Namespace]
				if ok {
					delete(ns, srv.ObjectMeta.Name)
					if len(ns) == 0 {
						delete(handler.namespaces, srv.Namespace)
					}
				}
			}
			handler.snapshotCache.SetSnapshot("envoy-proxy", handler.Generate())
		}
	}
}

// HandleEndpointStatusChange handle creation or deletion of endpoint
func (handler *KubeHandler) HandleEndpointStatusChange(endpoints *v1.Endpoints, key string, changeType ResourceChangeType) {
	ns, ok := handler.namespaces[endpoints.Namespace]
	if ok {
		// namespace exists
		srvInfo, ok := ns[endpoints.Name]
		if ok {
			// service for endpoints exists
			// change internal database
			handler.mu.Lock()
			defer handler.mu.Unlock()

			if changeType == ResourceDeleted {
				srvInfo.Endpoints = map[string]EndpointInfo{}
			} else {
				for _, ss := range endpoints.Subsets {
					for _, pp := range ss.Ports {
						for _, a := range ss.Addresses {
							srvInfo.Endpoints[*a.NodeName] = EndpointInfo{
								IP:   a.IP,
								Port: pp.Port,
							}
						}
					}
				}
			}

			ct := ChangeTypeString(changeType)
			fmt.Printf("Endpoints %s: %s, key: %s, ns: %s\n", ct, endpoints.Name, key, endpoints.Namespace)
			handler.snapshotCache.SetSnapshot("envoy-proxy", handler.Generate())
		}
	}
}

// MakeQualifiedSrvInfos expand namespaces with services
func (handler *KubeHandler) MakeQualifiedSrvInfos() []QualifiedServiceInfo {
	var numClusters = 0
	for _, ns := range handler.namespaces {
		numClusters += len(ns)
	}

	clusters := make([]QualifiedServiceInfo, numClusters)

	i := 0
	for ns, nsMap := range handler.namespaces {
		for _, srv := range nsMap {
			endpoints := make([]EndpointInfo, 0, len(srv.Endpoints))
			for _, e := range srv.Endpoints {
				endpoints = append(endpoints, e)
			}

			clusters[i] =
				QualifiedServiceInfo{
					Namespace:   ns,
					ServiceName: srv.ServiceName,
					Alpn:        srv.ProxyConfig.HTTP2,
					ProxyConfig: srv.ProxyConfig,
					Endpoints:   endpoints,
				}
			i++
		}
	}

	return clusters
}

// Generate produces a snapshot from the parameters.
func (handler *KubeHandler) Generate() cache.Snapshot {
	qualifiedServices := handler.MakeQualifiedSrvInfos()
	numClusters := len(qualifiedServices)

	clusters := make([]types.Resource, numClusters)
	endpoints := make([]types.Resource, numClusters)

	for i, srv := range qualifiedServices {
		clusters[i] = MakeCluster(srv.ServiceName, srv.Namespace, srv.Alpn)
		endpoints[i] = MakeEndpoint(srv.ServiceName, srv.Namespace, srv.Endpoints)
	}

	routes := MakeRoutes(qualifiedServices)

	atomic.AddUint64(&handler.snapshotVersion, 1)
	version := atomic.LoadUint64(&handler.snapshotVersion)

	return cache.NewSnapshot(
		strconv.FormatUint(version, 10), 
		endpoints, 
		clusters, 
		[]types.Resource{routes}, 
		[]types.Resource{handler.listener},
		[]types.Resource{}, // runtimes
		[]types.Resource{}, // secrets
	)
}
