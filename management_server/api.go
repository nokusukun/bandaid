package main

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/levigross/grequests"
	"github.com/nokusukun/bandaid"
	"github.com/phayes/freeport"
	"io/ioutil"
	"log"
	"os"
	"path"
	"time"
)

type Configuration struct {
	DNS struct {
		Zone    string `json:"zone"`
		Domain  string `json:"domain"`
		Proxied bool   `json:"proxied"`
	} `json:"dns"`

	Caddy struct {
		Domains []string `json:"domains"`
		Host    string   `json:"host"`
	} `json:"caddy"`

	Health struct {
		CheckURL string `json:"check_url"`
	} `json:"health"`

	Force bool `json:"force"`
}

func IsError(code int, err interface{}, g *gin.Context) bool {
	if err == nil {
		return false
	}
	g.AbortWithStatusJSON(code, gin.H{
		"error": fmt.Sprintf("%v", err),
	})
	return true
}

type API struct {
	CFToken  string
	CaddyAPI string

	reserved map[int]interface{}
	configs  map[string]Configuration
}

func (api *API) BuildAPI() *gin.Engine {
	engine := gin.Default()

	a := engine.Group("/api")
	{
		a.GET("/status/:configId", api.GET_STATUS)
		a.POST("/launch/:configId", api.POST_INSTALL_CONFIG)
	}

	return engine
}

func (api *API) GET_STATUS(ctx *gin.Context) {
	service := ctx.Param("configId")
	config, exists := api.configs[service]
	if !exists {
		IsError(404, fmt.Errorf("config '%v' not found", service), ctx)
		return
	}

	type ServiceError struct {
		Code    int    `json:"code,omitempty"`
		Content string `json:"content,omitempty"`
	}
	type Health struct {
		Configuration Configuration `json:"configuration"`
		DialError     string        `json:"dial_error,omitempty"`
		ServiceError  *ServiceError `json:"service_error,omitempty"`
		Error         bool          `json:"error"`
	}
	health := Health{Configuration: config}

	// Attempt to ping
	url := fmt.Sprintf("http://%v/%v", config.Caddy.Host, config.Health.CheckURL)
	resp, err := grequests.Get(url, &grequests.RequestOptions{
		RequestTimeout: time.Second * 10,
	})
	if err != nil {
		health.DialError = fmt.Sprintf("failed to contact health_url(%v): %v", url, err)
		health.Error = true
	} else {
		if !resp.Ok {
			health.Error = true
			health.ServiceError = &ServiceError{
				Code:    resp.StatusCode,
				Content: resp.String(),
			}
		}
	}

	ctx.JSON(200, health)
}

func (api *API) POST_INSTALL_CONFIG(ctx *gin.Context) {
	config := &Configuration{}
	configId := ctx.Param("configId")
	if IsError(400, ctx.ShouldBindJSON(&config), ctx) {
		return
	}

	if config.DNS.Zone != "" {
		log.Println("Setting up cloudflare for", configId)

		auto := bandaid.AutoCloudflare(api.CFToken).
			SetZone(config.DNS.Zone).
			SetDomain(config.DNS.Domain).
			Proxied(config.DNS.Proxied)

		// make sure we're not overwriting someone's currently running service
		for cfid, configuration := range api.configs {
			fmt.Println(configuration.DNS.Domain, config.DNS.Domain, cfid, configId)
			if configuration.DNS.Domain == config.DNS.Domain && cfid != configId {
				if !config.Force {
					IsError(400, fmt.Errorf(
						"DNS domain is being used by a running service(%v),"+
							" change it or pass {'force': true} to the launch configuration. The service will attempt"+
							" to remove the existing configuration", cfid),
						ctx)
					return
				}
				if IsError(500, api.RemoveCFConfig(cfid, auto), ctx) {
					return
				}
				break
			}
		}

		// Attempt to remove existing running configuration, it's only two API calls anyways
		if IsError(400, api.RemoveCFConfig(configId, auto), ctx) {
			return
		}

		// send CF configuration
		record, err := auto.SendConfiguration()
		if IsError(400, err, ctx) {
			return
		}

		rb, err := json.Marshal(record)
		if IsError(500, err, ctx) {
			return
		}
		if IsError(500, ioutil.WriteFile(path.Join("configs", configId), rb, os.ModePerm), ctx) {
			return
		}
	}

	log.Println("Setting up caddy configuration for", configId)
	host := config.Caddy.Host
	if host == "" {
		ports, err := freeport.GetFreePorts(100)
		if IsError(500, err, ctx) {
			return
		}
		for _, port := range ports {
			if _, used := api.reserved[port]; !used {
				host = fmt.Sprintf("localhost:%v", port)
				api.reserved[port] = true
				break
			}
		}
	}

	host, err := bandaid.AutoCaddy(configId).
		SetDomain(bandaid.DomainConfig{
			Host: config.Caddy.Domains,
		}).
		SetHost(host).
		AttemptInitializeCaddy().
		Apply()
	if IsError(400, err, ctx) {
		return
	}

	config.Caddy.Host = host
	api.configs[configId] = *config
	ctx.JSON(200, gin.H{
		"host": host,
	})
}

func (api *API) RemoveCFConfig(configId string, auto *bandaid.CloudflareConfig) error {
	if b, err := ioutil.ReadFile(path.Join("configs", configId)); err == nil {
		rec := bandaid.DNSRecord{}
		err := json.Unmarshal(b, &rec)
		if err != nil {
			return err
		}
		_ = auto.RemoveConfiguration(rec)
	}
	return nil
}
