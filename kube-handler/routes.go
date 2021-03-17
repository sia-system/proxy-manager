package kubehandler

import (
	"time"

	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"

	"github.com/golang/protobuf/ptypes"
	log "github.com/sirupsen/logrus"

	proxyconf "demius.md/proxy-manager/proxy-config"
)

const (
	// ShortDuration 20 seconds response timeout
	ShortDuration = 20 * time.Second
	// NormalDuraion 60 seconds response timeout
	NormalDuraion = 60 * time.Second
	// LongDuration 15 minutes response timeout
	LongDuration = 15 * time.Minute
	// ExtraLongDuration 30 minutes response timeout
	ExtraLongDuration = 30 * time.Minute
	// InfiniteDuration 1 hour response timeout
	InfiniteDuration = 1 * time.Hour
)

// MakeRoutes creates an HTTP route that routes to a given cluster.
func MakeRoutes(services []QualifiedServiceInfo) *route.RouteConfiguration {
	var routesCnt = calcRoutesCnt(services)
	routes := make([]*route.Route, routesCnt)
	curRoutes := 0

	for _, srv := range services {
		clusterName := srv.ServiceName + "." + srv.Namespace

		for _, config := range srv.ProxyConfig.Routes {
			if !checkRoute(&config) {
				continue
			}
			var route *route.Route
			if config.Route != nil {
				route = makeRoute(clusterName, &config)
			} else if config.Redirect != nil {
				route = makeRedirect(clusterName, &config)
			}
			routes[curRoutes] = route
			curRoutes++
			/*
			if srv.ProxyConfig.Default && config.Redirect != nil {
				route = makeDefaultRedirect(clusterName, &config)
				routes[idx][curRoutes[idx]] = route
				curRoutes[idx]++
			}
			*/
		}

	}
	
	return &route.RouteConfiguration{
		Name: "acc-routes",
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "backend-acc",
				Domains: []string{"*"},
				Routes:  routes,
			},
		},
	}
}

func checkRoute(config *proxyconf.Route) bool {
	if config.Route != nil {
		if config.Match.Prefix == "" {
			return false
		}
	} else if config.Redirect != nil {
		if config.Match.Prefix == "" && config.Match.Path == "" {
			return false
		}
	} else {
		return false
	}
	return true
}

func makeRoute(clusterName string, config *proxyconf.Route) *route.Route {
	duration := calcDuration(config.Route.Timeout)

	log.Println("      route: ", config.Match.Prefix, "->", config.Route.PrefixRewrite, clusterName)

	return &route.Route{
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: config.Match.Prefix,
			},
		},
		Action: &route.Route_Route{
			Route: &route.RouteAction{
				PrefixRewrite:    config.Route.PrefixRewrite,
				ClusterSpecifier: &route.RouteAction_Cluster{Cluster: clusterName},
				Timeout:          ptypes.DurationProto(duration),
			},
		},
	}
}

func makeRedirect(clusterName string, config *proxyconf.Route) *route.Route {
	var match *route.RouteMatch

	if config.Match.Prefix != "" {
		log.Println("      redirect (prefix): ", config.Match.Prefix, "->", config.Redirect.PathRedirect, clusterName)

		match = &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: config.Match.Prefix,
			},
		}
	} else {
		log.Println("      redirect (path): ", config.Match.Path, "->", config.Redirect.PathRedirect, clusterName)

		match = &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Path{
				Path: config.Match.Path,
			},
		}
	}

	return &route.Route{
		Match: match,
		Action: &route.Route_Redirect{
			Redirect: &route.RedirectAction{
				PathRewriteSpecifier: &route.RedirectAction_PathRedirect{
					PathRedirect: config.Redirect.PathRedirect,
				},
			},
		},
	}
}

// redirect from '/' (default)  to real path
func makeDefaultRedirect(clusterName string, config *proxyconf.Route) *route.Route {
	return &route.Route{
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Path{
				Path: "/",
			},
		},
		Action: &route.Route_Redirect{
			Redirect: &route.RedirectAction{
				PathRewriteSpecifier: &route.RedirectAction_PathRedirect{
					PathRedirect: config.Redirect.PathRedirect,
				},
			},
		},
	}
}

// first route is ACC, second route is IO
func calcRoutesCnt(services []QualifiedServiceInfo) int {
	var cnt = 0
	for _, srv := range services {
		cnt += len(srv.ProxyConfig.Routes)
		/*
		if srv.ProxyConfig.Default {
			cnt++
		}
		*/
	}
	return cnt
}

func calcDuration(timeout string) time.Duration {
	switch timeout {
	case "short":
		return ShortDuration
	case "normal":
		return NormalDuraion
	case "long":
		return LongDuration
	case "extra":
		return ExtraLongDuration
	case "infinite":
		return InfiniteDuration
	}
	return NormalDuraion
}
