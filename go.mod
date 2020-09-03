module demius.md/proxy-manager

go 1.14

// https://github.com/kubernetes/client-go/blob/master/INSTALL.md#go-modules

require (
	github.com/envoyproxy/go-control-plane v0.9.4
	github.com/golang/protobuf v1.3.3
	github.com/sirupsen/logrus v1.4.2
	google.golang.org/grpc v1.29.1
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.2
)
