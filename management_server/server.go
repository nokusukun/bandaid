package main

import (
	"bytes"
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
var (
	manager_address string
	slack_address   string
	caddy_address   string
)

func init() {
	config, err := ini.Load("config.ini")
	manager_address = config.Section("locations").Key("manager").String()
	slack_address = config.Section("locations").Key("slack_engine").String()
	caddy_address = config.Section("locations").Key("caddy").String()

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
	_, err := grequests.Get(fmt.Sprintf("http://%v", caddy_address), nil)
	if err != nil {
		panic(fmt.Errorf("Initial request failed, make sure caddy-admin is running on localhost:2019 (%v)", err))
	}

	go func() {
		panic(api.BuildAPI().Run(manager_address))
	}()
	go func() {
		panic(SlackEngine(slack_address, api.Config))
	}()

	time.Sleep(time.Second)

	LoadApplications()

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

func LoadApplications() {
	log.Println("[startup] Looking for deployed services...")
	matches, err := filepath.Glob(path.Join("app_data", "**"))
	if err != nil {
		panic(err)
	}
	for _, match := range matches {
		// check if it's a valid hash by attempting to hex decode the folder name
		//id := filepath.Base(match)
		// Check if it's a valid application folder by attempting to decode it as a hex string
		//if _, err := hex.DecodeString(id); err != nil {
		//	continue
		//}

		stat_dir, err := os.Stat(match)

		if err != nil || !stat_dir.IsDir() {
			continue
		}

		_, err = os.Stat(path.Join(match, ".git"))
		if os.IsNotExist(err) {
			log.Println("[startup]", match, "does not contain a git repo. Deleting...")
			err = os.Remove(match)
			if err != nil {
				log.Println("[startup] Failed to remove", match, ",", err)
			}
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
		specificConfig := path.Ext(match)
		// removes the separator if it exists
		if specificConfig != "" {
			log.Printf("[startup] Using configuration specified: %v\n", specificConfig[1:])
			specificConfig = strings.Replace(specificConfig[1:], "-", ".", -1)
		}

		body, _ := json.Marshal(gin.H{
			"repository": origin,
			"config":     specificConfig,
		})
		manager_address := api.Config.Section("locations").Key("manager").String()
		response, err := (&http.Client{Timeout: time.Minute * 10}).Post(fmt.Sprintf("http://%v/manager/app", manager_address), "application/json", bytes.NewBuffer(body))
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
}
