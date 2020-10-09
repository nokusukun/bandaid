package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/levigross/grequests"
	"github.com/nokusukun/bandaid"
	"github.com/phayes/freeport"
	"gopkg.in/ini.v1"
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
	Config   *ini.File
	CaddyAPI string

	reserved map[int]interface{}
	configs  map[string]Configuration

	deployed map[string]*Application
}

func (api *API) BuildAPI() *gin.Engine {
	engine := gin.Default()

	a := engine.Group("/api")
	{
		a.GET("/status/:configId", api.GET_STATUS)
		a.POST("/launch/:configId", api.POST_INSTALL_CONFIG)
	}

	manager := engine.Group("/manager")
	{
		manager.GET("/app/:serviceId/stdout", api.MANAGER_GET_STDOUT)
		manager.GET("/app/:serviceId/stderr", api.MANAGER_GET_STDERR)
		manager.GET("/app/:serviceId/events", api.MANAGER_GET_EVENTS)
		manager.GET("/app/:serviceId/reload", api.MANAGER_GET_RELOAD)
		manager.GET("/app/:serviceId/config", api.MANAGER_GET_CONFIG)
		manager.DELETE("/app/:serviceId", api.MANAGER_DELETE_APPLICATION)
		manager.POST("/app", api.MANAGER_POST_DEPLOY)

	}
	return engine
}

func (api *API) MANAGER_GET_RELOAD(ctx *gin.Context) {
	service, exists := api.deployed[ctx.Param("serviceId")]
	if !exists {
		IsError(404, fmt.Errorf("service not found"), ctx)
		return
	}
	if IsError(500, service.Reload(), ctx) {
		return
	}
	ctx.String(200, "OK")
}

func (api *API) MANAGER_GET_STDOUT(ctx *gin.Context) {
	service, exists := api.deployed[ctx.Param("serviceId")]
	if !exists {
		IsError(404, fmt.Errorf("service not found"), ctx)
		return
	}
	ctx.String(200, service.log.String())
}

func (api *API) MANAGER_GET_CONFIG(ctx *gin.Context) {
	service, exists := api.deployed[ctx.Param("serviceId")]
	if !exists {
		IsError(404, fmt.Errorf("service not found"), ctx)
		return
	}

	if cfg, err := service.Config(); IsError(500, err, ctx) {
		return
	} else {
		ctx.JSON(200, cfg)
	}
}

func (api *API) MANAGER_GET_STDERR(ctx *gin.Context) {
	service, exists := api.deployed[ctx.Param("serviceId")]
	if !exists {
		IsError(404, fmt.Errorf("service not found"), ctx)
		return
	}
	ctx.String(200, service.err.String())
}

func (api *API) MANAGER_GET_EVENTS(ctx *gin.Context) {
	service, exists := api.deployed[ctx.Param("serviceId")]
	if !exists {
		IsError(404, fmt.Errorf("service not found"), ctx)
		return
	}
	ctx.JSON(200, service.Events)
}

func (api *API) MANAGER_DELETE_APPLICATION(ctx *gin.Context) {
	serviceID := ctx.Param("serviceId")
	service, exists := api.deployed[serviceID]
	if !exists {
		// Attempt to delete the folder if it exists
		applicationPath := path.Join("app_data", serviceID)
		if _, err := os.Stat(applicationPath); !os.IsNotExist(err) {
			err = os.RemoveAll(applicationPath)
			if err != nil {
				IsError(500, fmt.Errorf("failed to delete existing path '%v' please manually delete it from the app_data folder", applicationPath), ctx)
				return
			}
			ctx.String(200, "OK")
		} else {
			IsError(404, fmt.Errorf("service not found"), ctx)
		}
		return
	}

	if IsError(500, service.Kill(), ctx) {
		return
	}

	applicationPath := path.Join("app_data", serviceID)
	if _, err := os.Stat(applicationPath); !os.IsNotExist(err) {
		err = os.RemoveAll(applicationPath)
		if err != nil {
			IsError(500, fmt.Errorf("failed to delete existing path '%v' please manually delete it from the app_data folder", applicationPath), ctx)
			return
		}
	}
	ctx.String(200, "OK")
}

func (api *API) MANAGER_POST_DEPLOY(ctx *gin.Context) {
	app := &Application{}
	if IsError(400, ctx.BindJSON(app), ctx) {
		return
	}
	hash := md5.Sum([]byte(app.Repository))
	app.ID = hex.EncodeToString(hash[:])

	if _, exists := api.deployed[app.ID]; exists {
		IsError(409, fmt.Errorf("resource already exists as '%v', please reload or delete the deployed application first", app.ID), ctx)
		return
	}

	applicationPath := path.Join("app_data", app.ID)
	if _, err := os.Stat(applicationPath); !os.IsNotExist(err) {
		err = os.RemoveAll(applicationPath)
		if err != nil {
			IsError(500, fmt.Errorf("failed to delete existing path '%v' please manually delete it from the app_data folder", applicationPath), ctx)
			return
		}
	}

	if IsError(400, app.Clone(), ctx) {
		return
	}

	if _, err := app.Config(); err != nil {
		IsError(400, fmt.Errorf("Failed reading Bandaidfile, check file and recommit"), ctx)
		return
	}

	api.deployed[app.ID] = app
	go app.Launch()
	ctx.String(200, app.ID)
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
	_ = api.Config.Reload()
	config := &Configuration{}
	configId := ctx.Param("configId")
	if IsError(400, ctx.ShouldBindJSON(&config), ctx) {
		return
	}

	// Caddy
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

	// Cloudflare/DNS
	if config.DNS.Zone != "" {
		log.Println("Setting up cloudflare for", configId)
		token := api.Config.Section("cloudflare").Key(config.DNS.Zone).String()
		if token == "" {
			IsError(400, fmt.Errorf("there is no token saved for %v in the configuration file", config.DNS.Zone), ctx)
			return
		}
		auto := bandaid.AutoCloudflare(token).
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
				_, err := api.RemoveCFConfig(cfid, auto, nil, true)
				if IsError(500, err, ctx) {
					return
				}
				break
			}
		}

		// Attempt to remove existing running configuration, it's only two API calls anyways
		skipped, err := api.RemoveCFConfig(configId, auto, config, config.Force)
		if IsError(400, err, ctx) {
			return
		}

		// send CF configuration
		if !skipped {
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
	}

	api.configs[configId] = *config
	ctx.JSON(200, gin.H{
		"host": host,
	})
}

func (api *API) RemoveCFConfig(configId string, auto *bandaid.CloudflareConfig, config *Configuration, reload bool) (skipped bool, err error) {
	if b, err := ioutil.ReadFile(path.Join("configs", configId)); err == nil {
		rec := bandaid.DNSRecord{}
		err := json.Unmarshal(b, &rec)
		if err != nil {
			return false, err
		}
		if !reload {
			machine_ip, _ := bandaid.GetIP()
			if config.DNS.Zone == rec.ZoneName && config.DNS.Domain == rec.Name && machine_ip == rec.Content {
				log.Println("Skipping config removal, records are identical")
				return true, nil
			}
		}
		_ = auto.RemoveConfiguration(rec)
	}
	return false, nil
}
