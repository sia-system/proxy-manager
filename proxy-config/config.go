package proxyconfig

import (
	"fmt"

	yaml "gopkg.in/yaml.v2"
)

// Config contains config for envoy-proxy
type Config struct {
	Listener string  `yaml:"listener"`
	HTTP2    bool    `yaml:"h2"`
	Routes   []Route `yaml:"routes"`
}

// Route contains single route entry
type Route struct {
	Match    RouteMatch      `yaml:"match"`
	Route    *RouteAction    `yaml:"route"`
	Redirect *RedirectAction `yaml:"redirect"`
}

// RouteMatch corresponds to envoy-proxy match clause
type RouteMatch struct {
	Path   string `yaml:"path"`
	Prefix string `yaml:"prefix"`
}

// RouteAction corresponds to envoy-proxy route clause
// Valid timeouts: normal, long, infinite
type RouteAction struct {
	PrefixRewrite string `yaml:"prefix_rewrite"`
	Timeout       string `yaml:"timeout"`
}

// RedirectAction corresponds to envoy-proxy redirect clause
type RedirectAction struct {
	PathRedirect string `yaml:"path_redirect"`
}

// Parse service annotation aria.io/proxy-config
func Parse(config []byte) (*Config, error) {
	c := Config{}
	if err := yaml.Unmarshal(config, &c); err != nil {
		return nil, fmt.Errorf("Can not unmarshar ProxyConfig: %v", err)
	}
	return &c, nil
}
