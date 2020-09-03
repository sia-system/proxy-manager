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

// Package server contains test utilities
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	//"net/http"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	v2grpc "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v2"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v2"
	xds "github.com/envoyproxy/go-control-plane/pkg/server"
)

/*
// Hasher returns node ID as an ID
type Hasher struct {
}

// ID function
func (h Hasher) ID(node *core.Node) string {
	if node == nil {
		return "unknown"
	}
	return node.Id
}
*/

// RunAccessLogServer starts an accesslog service.
func RunAccessLogServer(ctx context.Context, als *AccessLogService, port uint) {
	grpcServer := grpc.NewServer()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.WithError(err).Fatal("failed to listen")
	}

	accesslog.RegisterAccessLogServiceServer(grpcServer, als)
	log.WithFields(log.Fields{"port": port}).Info("access log server listening")

	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Error(err)
		}
	}()
	<-ctx.Done()

	log.Info("access log server stopping")
	grpcServer.GracefulStop()
}

const grpcMaxConcurrentStreams = 1000000

// RunManagementServer starts an xDS server at the given port.
func RunManagementServer(ctx context.Context, server xds.Server, port uint) {
	// gRPC golang library sets a very small upper bound for the number gRPC/h2
	// streams over a single TCP connection. If a proxy multiplexes requests over
	// a single connection to the management server, then it might lead to
	// availability problems.
	var grpcOptions []grpc.ServerOption
	grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	grpcServer := grpc.NewServer(grpcOptions...)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.WithError(err).Fatal("failed to listen")
	}

	// register services
	discovery.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)

	v2grpc.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
	v2grpc.RegisterClusterDiscoveryServiceServer(grpcServer, server)
	v2grpc.RegisterRouteDiscoveryServiceServer(grpcServer, server)
	v2grpc.RegisterListenerDiscoveryServiceServer(grpcServer, server)

	log.WithFields(log.Fields{"port": port}).Info("management server listening")
	go func() {
		if err = grpcServer.Serve(lis); err != nil {
			log.Error(err)
		}
	}()
	<-ctx.Done()

	log.Info("management server stopping")

	grpcServer.GracefulStop()
}

// RunManagementGateway starts an HTTP gateway to an xDS server.
func RunManagementGateway(ctx context.Context, srv xds.Server, port uint) {
	log.WithFields(log.Fields{"port": port}).Info("gateway listening HTTP/1.1")
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: &xds.HTTPGateway{Server: srv}}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Error(err)
		}
	}()
	if err := server.Shutdown(ctx); err != nil {
		log.Error(err)
	}
}

/*
type DelegateCallbacks struct {
	fetches  int
	requests int
	mu       sync.Mutex
}

func (cb *DelegateCallbacks) Report() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	log.WithFields(log.Fields{"fetches": cb.fetches, "requests": cb.requests}).Info("server callbacks")
}
func (cb *DelegateCallbacks) OnStreamOpen(context context.Context, id int64, typ string) error {
	// log.Infof("info: stream %d open for %s", id, typ)
	return nil
}
func (cb *DelegateCallbacks) OnStreamClosed(id int64) {
	// log.Infof("stream %d closed", id)
}
func (cb *DelegateCallbacks) OnStreamRequest(id int64, req *v2.DiscoveryRequest) error {
	log.Infof("stream %d open for node %s, type: %s, %v", id, req.Node.Id, req.TypeUrl, req.ResourceNames)
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.requests++
	return nil
}
func (cb *DelegateCallbacks) OnStreamResponse(id int64, req *v2.DiscoveryRequest, res *v2.DiscoveryResponse) {
	// log.Infof("stream response %d for node %s: %s", id, req.Node.Id, res.String())
}

func (cb *DelegateCallbacks) OnFetchRequest(context context.Context, req *v2.DiscoveryRequest) error {
	log.Infof("fetch request for node %s, %v", req.Node.Id, req.ResourceNames)
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.fetches++

	return nil
}
func (cb *DelegateCallbacks) OnFetchResponse(req *v2.DiscoveryRequest, res *v2.DiscoveryResponse) {
	// log.Infof("fetch response for node %s: %s", req.Node.Id, res.String())
}
*/
