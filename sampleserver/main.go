package main

import (
	"github.com/gin-gonic/gin"
	"github.com/nokusukun/bandaid"
	"io/ioutil"
)

func main() {
	router := gin.Default()
	router.GET("/", func(context *gin.Context) {
		context.String(200, "Hello there from bandaid!")
	})

	token, err := ioutil.ReadFile("token.ini")
	if err != nil {
		panic(err)
	}
	err = bandaid.AutoCloudflare(string(token)).
		SetZone("noku.pw").
		SetDomain("example.noku.pw").
		Proxied(true).
		Reinstall()
	if err != nil {
		panic(err)
	}

	err = bandaid.AutoCaddy("sample-application").
		SetDomain(bandaid.DomainConfig{
			Host: []string{"example.noku.pw", "test.example.com"},
		}).
		SetHost("localhost:3451").
		AttemptInitializeCaddy().
		ApplyAndRun(func(host string) error {
			return router.Run(host)
		})

	if err != nil {
		panic(err)
	}
}
