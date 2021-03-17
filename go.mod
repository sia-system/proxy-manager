module demius.md/proxy-manager

go 1.16

// https://github.com/kubernetes/client-go/blob/master/INSTALL.md#go-modules

require (
	github.com/envoyproxy/go-control-plane v0.9.9-0.20201210154907-fd9021fe5dad
	github.com/golang/protobuf v1.4.3
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/grpc v1.36.0
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.20.4
	k8s.io/apimachinery v0.20.4
	k8s.io/client-go v0.20.4
)
