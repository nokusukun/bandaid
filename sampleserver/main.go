package main

import (
	"caddy_bandaid"
	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()
	router.GET("/", func(context *gin.Context) {
		context.String(200, "Hello there from bandaid!")
	})

	err := caddy_bandaid.New("sample-application").
		SetDomain(caddy_bandaid.DomainConfig{
			Host: []string{"subdomain.example.com", "testx.example.com"},
		}).
		AttemptInitializeCaddy().
		ApplyAndRun(func(host string) error {
			return router.Run(host)
		})

	if err != nil {
		panic(err)
	}
}
