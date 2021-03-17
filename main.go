package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	envoycache "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	envoyserver "github.com/envoyproxy/go-control-plane/pkg/server/v3"

	handler "demius.md/proxy-manager/kube-handler"
	server "demius.md/proxy-manager/server"
)

// https://github.com/envoyproxy/go-control-plane
// https://github.com/envoyproxy/go-control-plane/tree/master/sample

// https://github.com/kubernetes/client-go/blob/master/examples/workqueue/main.go
// git clone https://github.com/envoyproxy/go-control-plane/

var (
	mode         string
	port         uint
	gatewayPort  uint
	upstreamPort uint
	basePort     uint
	alsPort      uint
)

func init() {
	flag.UintVar(&port, "port", 18000, "Management server port")
	flag.UintVar(&gatewayPort, "gateway", 18001, "Management server port for HTTP gateway")
	flag.UintVar(&upstreamPort, "upstream", 18080, "Upstream HTTP/1.1 port")
	flag.UintVar(&basePort, "base", 9000, "Listener port")
}

func main() {
	flag.Parse()

	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)

	log.Println("Starting envoy-delegate-service")

	// init config for kubernetes config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// create a cache for envoy management server
	// cb := &server.DelegateCallbacks{}
	// snapshotCache := envoycache.NewSnapshotCache(false, envoycache.IDHash{}, nil)
	snapshotCache := envoycache.NewSnapshotCache(true, envoycache.IDHash{}, nil)
	srv := envoyserver.NewServer(context.Background(), snapshotCache, nil)
	// als := &server.AccessLogService{} // ???

	kubeHandler := handler.New(snapshotCache, clientset.CoreV1())

	// start the xDS server
	go server.RunManagementServer(ctx, srv, port)

	resyncPeriod := 1 * time.Minute

	// watch services
	servicesListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "services", v1.NamespaceAll, fields.Everything())

	_, watchController := cache.NewInformer(servicesListWatcher, &v1.Service{}, resyncPeriod, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				srv := obj.(*v1.Service)
				kubeHandler.HandleServiceStatusChange(srv, key, handler.ResourceCreated)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				srv := obj.(*v1.Service)
				kubeHandler.HandleServiceStatusChange(srv, key, handler.ResourceDeleted)
			}
		},
	})

	// watch endpoints
	endpointsListWatcher := cache.NewListWatchFromClient(clientset.CoreV1().RESTClient(), "endpoints", v1.NamespaceAll, fields.Everything())

	_, endpointsWatchController := cache.NewInformer(endpointsListWatcher, &v1.Endpoints{}, resyncPeriod, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				srv := obj.(*v1.Endpoints)
				kubeHandler.HandleEndpointStatusChange(srv, key, handler.ResourceCreated)
			}
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(new)
			if err == nil {
				newEndpoints := new.(*v1.Endpoints)
				oldEndpoints := old.(*v1.Endpoints)
				kubeHandler.HandleEndpointStatusChange(oldEndpoints, key, handler.ResourceDeleted)
				kubeHandler.HandleEndpointStatusChange(newEndpoints, key, handler.ResourceCreated)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				srv := obj.(*v1.Endpoints)
				kubeHandler.HandleEndpointStatusChange(srv, key, handler.ResourceDeleted)
			}
		},
	})

	// start kubernetes listener
	go watchController.Run(ctx.Done())
	time.Sleep(100 * time.Millisecond)
	go endpointsWatchController.Run(ctx.Done())

	waitForSignal()
	log.Println("Stopping envoy-delegate-service")
	cancel()
}

func waitForSignal() {
	// graceful shutdown
	// trap Ctrl+C and call cancel on the context
	gracefulStop := make(chan os.Signal)
	signal.Notify(gracefulStop, syscall.SIGINT, syscall.SIGTERM)
	s := <-gracefulStop
	log.Printf("Got signal: %v, exiting.", s)
}

type logger struct{}

func (logger logger) Infof(format string, args ...interface{}) {
	log.Debugf(format, args...)
}
func (logger logger) Errorf(format string, args ...interface{}) {
	log.Errorf(format, args...)
}
