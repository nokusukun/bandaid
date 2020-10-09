package main

import (
	"fmt"
	"github.com/levigross/grequests"
	"gopkg.in/ini.v1"
	"log"
	"os/exec"
)

var api *API

func init() {
	config, err := ini.Load("config.ini")
	if err != nil {
		panic(err)
	}
	api = &API{
		//CFToken:  config.Section("cloudflare").Key("token").String(),
		Config:   config,
		CaddyAPI: "http://localhost:2019",
		reserved: map[int]interface{}{},
		configs:  map[string]Configuration{},
		deployed: map[string]*Application{},
	}

	err = exec.Command("git", "--version").Run()
	if err != nil {
		log.Println("running 'git --version' failed, make sure that git is installed in the machine")
		panic(err)
	}
}

func main() {
	// Making sure that caddy is actually running
	_, err := grequests.Get("http://localhost:2019/", nil)
	if err != nil {
		panic(fmt.Errorf("Initial request failed, make sure caddy-admin is running on localhost:2019 (%v)", err))
	}

	panic(api.BuildAPI().Run("localhost:2020"))
}
