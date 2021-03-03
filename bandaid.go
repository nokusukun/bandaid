package bandaid

import (
	"errors"
	"fmt"
	"github.com/levigross/grequests"
	"github.com/phayes/freeport"
	"log"
	"strings"
)

type CaddyConfig struct {
	ID       string         `json:"@id,omitempty"`
	Match    []DomainConfig `json:"match,omitempty"`
	Handle   []ConfigHandle `json:"handle,omitempty"`
	Terminal *bool          `json:"terminal,omitempty"`
}

type ConfigHandle struct {
	Handler string  `json:"handler,omitempty"`
	Routes  []Route `json:"routes,omitempty"`
}

type Route struct {
	Handle []RouteHandle `json:"handle,omitempty"`
}

type RouteHandle struct {
	Handler   string     `json:"handler,omitempty"`
	Upstreams []Upstream `json:"upstreams,omitempty"`
}

type Upstream struct {
	Dial string `json:"dial,omitempty"`
}

type DomainConfig struct {
	Host []string `json:"host,omitempty"`
	Path []string `json:"path,omitempty"`
}

type AutoCaddyConfig struct {
	host      string
	Config    *CaddyConfig
	CaddyAPI  string
	RoutePath string

	initial_should_enable_autohttps bool
}

func AutoCaddy(id string) *AutoCaddyConfig {
	return &AutoCaddyConfig{
		Config:    &CaddyConfig{ID: fmt.Sprintf("bandaid-%v", id)},
		CaddyAPI:  "http://localhost:2019",
		RoutePath: "config/apps/http/servers/srv0/routes",
	}
}

func (b *AutoCaddyConfig) SetDomain(config DomainConfig) *AutoCaddyConfig {
	if len(b.Config.Match) == 0 {
		b.Config.Match = []DomainConfig{}
	}

	b.Config.Match = append(b.Config.Match, config)
	return b
}

func (b *AutoCaddyConfig) SetHost(host string) *AutoCaddyConfig {
	b.host = host
	return b
}

func (b *AutoCaddyConfig) Initial_SetAutoHTTPS(auto bool) *AutoCaddyConfig {
	b.initial_should_enable_autohttps = auto
	return b
}

func (b *AutoCaddyConfig) AttemptInitializeCaddy() *AutoCaddyConfig {
	def := map[string]interface{}{
		"apps": map[string]interface{}{
			"http": map[string]interface{}{
				"servers": map[string]interface{}{
					"srv0": map[string]interface{}{
						"automatic_https": map[string]interface{}{
							"disable": !b.initial_should_enable_autohttps,
						},
						"listen": []interface{}{
							":80",
						},
						"routes": []interface{}{},
					},
				},
			},
		},
	}

	resp, _ := grequests.Get(fmt.Sprintf("%v/config", b.CaddyAPI), nil)
	if strings.TrimSpace(resp.String()) == "null" {
		log.Println("[bandaid] Initializing configuration")
		resp, _ := grequests.Post(fmt.Sprintf("%v/load", b.CaddyAPI), &grequests.RequestOptions{
			JSON: def,
		})
		if !resp.Ok {
			log.Panicln("failed to initialize configuration:", resp.String())
		}
	}

	return b
}

func (b *AutoCaddyConfig) ApplyAndRun(launch func(host string) error) error {
	host, err := b.Apply()
	if err != nil {
		return err
	}

	log.Println("[bandaid] Launching")
	return launch(host)
}

func (b *AutoCaddyConfig) Apply() (string, error) {
	log.Println("[bandaid] Configuring caddy reverse proxy")

	host := b.host
	// If no host has been selected, then try to launch the application of a random unused port
	if host == "" {
		port, err := freeport.GetFreePort()
		log.Printf("[bandaid] No host specified, using 'localhost:%v'\n", port)
		if err != nil {
			return "", err
		}
		host = fmt.Sprintf("localhost:%v", port)
	}
	b.Config.Handle = []ConfigHandle{
		{
			Handler: "subroute",
			Routes: []Route{
				{Handle: []RouteHandle{
					{
						Handler: "reverse_proxy",
						Upstreams: []Upstream{
							{Dial: host},
						},
					},
				}},
			},
		},
	}

	resp, err := grequests.Delete(fmt.Sprintf("%v/id/%v", b.CaddyAPI, b.Config.ID), nil)
	if err != nil {
		return "", err
	}
	if !resp.Ok && !strings.Contains(resp.String(), "unknown object ID") {
		return "", errors.New(resp.String())
	}

	resp, err = grequests.Post(fmt.Sprintf("%v/%v", b.CaddyAPI, b.RoutePath), &grequests.RequestOptions{
		JSON: b.Config,
	})
	if err != nil {
		return "", err
	}
	if !resp.Ok {
		return "", errors.New(resp.String())
	}
	return host, nil
}
