# Bandaid
---
Automate caddy and cloudflare reverse proxying within go.

### Install
`go get github.com/nokusukun/caddy_bandaid`

### Usage
```go
package main

import (
	"bandaid"
	"github.com/gin-gonic/gin"
)

func main()  {
	router := gin.Default()
	router.GET("/", func(context *gin.Context) {
		context.String(200, "Hello there from bandaid!")
	})
	
    // Cloudflare auto configuration
    err := bandaid.AutoCloudflare("cloudflare-api-token").
		SetZone("noku.pw").
		SetDomain("example.noku.pw").
		Proxied(true).
		Install()
    
    // Caddy reverse proxy configuration
	err := bandaid.AutoCaddy("sample-application").
		SetDomain(bandaid.DomainConfig{
			Host: []string{"subdomain.example.com"},
		}).
		AttemptInitializeCaddy().
		ApplyAndRun(func(host string) error {
			return router.Run(host)
		})

	if err != nil {
		panic(err)
	}
}
```
