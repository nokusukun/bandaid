package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/levigross/grequests"
	"gopkg.in/ini.v1"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
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

	_api := api.BuildAPI()
	go func() {
		panic(_api.Run("localhost:2020"))
	}()

	time.Sleep(time.Second)

	log.Println("[startup] Looking for deployed services...")
	matches, err := filepath.Glob(path.Join("app_data", "**"))
	for _, match := range matches {
		// check if it's a valid hash by attempting to hex decode the folder name
		id := filepath.Base(match)
		if _, err := hex.DecodeString(id); err != nil {
			continue
		}
		log.Println("[startup] Loading", match)
		cmd := exec.Command("git", "remote", "get-url", "--all", "origin")
		cmd.Dir = match
		url, err := cmd.CombinedOutput()
		if err != nil {
			log.Println("[startup] Failed to get origin url: ", err, string(url))
			continue
		}
		origin := strings.TrimSpace(string(url))
		log.Println("[startup] Deploying", origin)

		body, _ := json.Marshal(gin.H{
			"repository": origin,
		})
		response, err := (&http.Client{Timeout: time.Minute * 10}).Post("http://localhost:2020/manager/app", "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Println("[startup] Failed:", err)
			continue
		}
		data, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Println("[startup] Failed:", err)
		}
		log.Println("[startup] OK:", string(data))
	}

	log.Println("[startup] Initialization done, ctrl+c to exit")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		for _ = range c {
			log.Println("Exiting...")
			for _, application := range api.deployed {
				_ = application.Kill()
			}
			os.Exit(1)
		}
	}()
	select {}
}
