package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/imroc/req"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

type AppEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Error     string    `json:"error,omitempty"`
	Message   string    `json:"message,omitempty"`
}

type Application struct {
	Repository     string      `json:"repository"`
	ID             string      `json:"id"`
	Events         []*AppEvent `json:"events"`
	SpecificConfig string      `json:"config"`
	Branch         string      `json:"branch"`

	directory  string
	cmd        *exec.Cmd
	env        []string
	log        bytes.Buffer
	err        bytes.Buffer
	event_urls []string
}

type BandaidFile struct {
	ConfigPath  string
	Application struct {
		ID            string     `toml:"id"`
		Name          string     `toml:"name"`
		Run           [][]string `toml:"run"`
		EventURL      string     `toml:"event_urls"`
		Health        string     `toml:"health_endpoint"`
		Envs          []string   `toml:"envs"`
		BaseDirectory string     `toml:"base_dir"`
	} `toml:"application"`

	Repository struct {
		Branch       string `toml:"branch"`
		ReloadOnPush bool   `toml:"reload_on_push"`
	} `toml:"repository"`

	DNS struct {
		Zone    string `toml:"zone"`
		Domain  string `toml:"domain"`
		Proxied bool   `toml:"proxied"`
	} `toml:"dns"`

	Caddy struct {
		Domains []string `toml:"domains"`
		Host    string   `toml:"host"`
	} `toml:"caddy"`
}

func (app *Application) Log_Eventf(format string, msgs ...interface{}) {
	app.Log_Event(fmt.Sprintf(format, msgs...))
}

func (app *Application) Log_Event(message string) {
	app.add_event(&AppEvent{
		Timestamp: time.Now(),
		Message:   message,
	})
}

func (app *Application) Log_Errorf(format string, msgs ...interface{}) {
	app.Log_Error(fmt.Errorf(format, msgs...))
}

func (app *Application) Log_Error(err error) {
	app.add_event(&AppEvent{
		Timestamp: time.Now(),
		Error:     fmt.Sprintf("%v", err.Error()),
	})
}

func (app *Application) add_event(event *AppEvent) {
	// TODO: Do some magical logging stuff here
	var b bytes.Buffer
	if json.NewEncoder(&b).Encode(event) != nil {
		for _, event_url := range app.event_urls {
			go (&http.Client{Timeout: time.Second * 5}).Post(event_url, "application/json", &b)
		}
	}

	app.Events = append(app.Events, event)
}

func (app *Application) Clone() error {
	app.Log_Eventf("Cloning from repository %v", app.Repository)
	app.directory = path.Join("app_data", app.ID)
	if app.SpecificConfig != "" {
		app.directory += "." + strings.Replace(app.SpecificConfig, ".", "-", -1)
	}
	app.env = os.Environ()

	args := []string{"clone"}
	if app.Branch != "" {
		args = append(args, "--branch", app.Branch)
	}
	args = append(args, app.Repository, app.directory)

	b, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %v", string(b), err)
	}
	return nil
}

func (app *Application) Destroy() error {
	if _, err := os.Stat(app.directory); !os.IsNotExist(err) {
		err = os.RemoveAll(app.directory)
		if err != nil {
			return err
		}
	}
	return nil
}

func (app *Application) Kill() error {
	log.Println("Killing process", app.ID)
	app.Log_Eventf("Killing process %v", app.ID)
	// ProcessState contains information about an exited process available after a call to Wait or Run.
	if app.cmd != nil {
		if app.cmd.ProcessState == nil {
			return app.cmd.Process.Kill()
		}
		// Just do it anyways for safe measure
		_ = app.cmd.Process.Kill()
	}
	return nil
}

func (app *Application) Reload() error {
	app.Log_Eventf("Reloading application %v", app.ID)
	err := app.Kill()
	if err != nil {
		return err
	}

	app.Log_Eventf("Pulling from repository %v", app.Repository)
	cmd := exec.Command("git", "pull")
	cmd.Dir = app.directory
	log.Println("Pulling new files for", app.directory)
	err = cmd.Run()
	if err != nil {
		return err
	}

	go app.Launch()
	return nil
}

func (app *Application) Config() (*BandaidFile, error) {
	config := &BandaidFile{}

	config_path := "Bandaid"
	if app.SpecificConfig != "" {
		config_path = app.SpecificConfig
	}

	_, err := toml.DecodeFile(path.Join(app.directory, config_path), config)
	if err != nil {
		return nil, err
	}

	config.ConfigPath = config_path
	return config, err
}

func (app *Application) Launch() {
	config, err := app.Config()
	app.Log_Event("Launching application")
	log.Println("Reading configuration from Bandaidfile")
	if err != nil {
		log.Println("Error", err)
		app.Log_Errorf("failed to read configuration: %v", err)
		return
	}

	app.event_urls = append(app.event_urls, config.Application.EventURL)

	log.Println("setting up autoconfig")
	resp, err := req.Post("http://localhost:2020/api/launch/"+app.ID, req.BodyJSON(Configuration{
		DNS: struct {
			Zone    string `json:"zone"`
			Domain  string `json:"domain"`
			Proxied bool   `json:"proxied"`
		}{
			Zone:    config.DNS.Zone,
			Domain:  config.DNS.Domain,
			Proxied: config.DNS.Proxied,
		},
		Caddy: struct {
			Domains   []string `json:"domains"`
			Host      string   `json:"host"`
			AutoHTTPS bool     `json:"auto_https"`
		}{
			Domains: config.Caddy.Domains,
			Host:    config.Caddy.Host,
		},
		Health: struct {
			CheckURL string `json:"check_url"`
		}{},
		Force: false,
	}))

	if err != nil {
		log.Println("Error", err)
		app.Log_Errorf("failed to send autoconfig: %v", err)
		return
	}

	type Response struct {
		Host  string `json:"host"`
		Error string `json:"error,omitempty"`
	}
	host := &Response{}
	err = resp.ToJSON(host)
	if err != nil {
		log.Println("Error", err)
		app.Log_Errorf("failed to read host from service, got: %v", resp.String())
		return
	}

	if host.Error != "" {
		log.Println("Error", host.Error)
		app.Log_Errorf("failed to setup host from service: %v", host.Error)
		return
	}

	if config.Application.BaseDirectory != "" {
		app.directory = path.Join(app.directory, config.Application.BaseDirectory)
	}

	app.env = append(app.env, fmt.Sprintf("APP_HOST=%v", host.Host))
	app.env = append(app.env, config.Application.Envs...)

	log.Println("Executing service at:", host.Host)
	app.Log_Eventf("Executing service at '%v'", host.Host)
	for i, commands := range config.Application.Run {
		app.Log_Eventf("Launching CMD (%v/%v) '%v'", i+1, len(config.Application.Run), commands)
		cmd := exec.Command(commands[0], commands[1:]...)
		cmd.Dir = app.directory
		cmd.Env = app.env
		app.cmd = cmd

		cmd.Stdout = &app.log
		cmd.Stderr = &app.err

		err := cmd.Run()
		if err != nil {
			log.Println("Error", err)
			app.Log_Error(err)
			return
		}
		app.Log_Eventf("Finished CMD '%v'", commands)
	}
}
