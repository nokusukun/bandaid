package main

import (
	"github.com/gin-gonic/gin"
	"github.com/imroc/req"
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

func main() {
	router := gin.Default()
	router.GET("/", func(context *gin.Context) {
		context.String(200, "Hello there from bandaid!")
	})

	config := Configuration{}
	config.DNS.Domain = "test-bandaid.app.com"
	config.DNS.Zone = "app.com"
	config.DNS.Proxied = true
	config.Caddy.Domains = []string{"test-bandaid.app.com"}

	resp, err := req.Post("http://localhost:2020/launch/test-app", req.BodyJSON(config))
	if err != nil {
		panic(err)
	}

	host := map[string]string{}
	_ = resp.ToJSON(&host)

	err = router.Run(host["host"])
	if err != nil {
		panic(err)
	}
}
