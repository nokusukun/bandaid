package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req"
	"github.com/nokusukun/stemp"
	"io/ioutil"
	"net/http"
	"time"
)

type CommandFunction func(fl Flags) (int, error)
type FlagFunction func() *flag.FlagSet

type Command struct {
	Name        string
	Usage       string
	Description string
	Function    CommandFunction
	Flags       FlagFunction
}

func InitializeCommands() {
	AddCommand(Command{
		Name:        "status",
		Usage:       "status [--app <application id>]",
		Description: "Get application status",
		Function:    cmdStatus,
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("status", flag.ExitOnError)
			fs.String("app", "", "Application ID")
			return fs
		},
	})

	AddCommand(Command{
		Name:        "apps",
		Usage:       "apps",
		Description: "Display a concise list of the currently running applications",
		Function:    cmdApps,
		Flags: func() *flag.FlagSet {
			return flag.NewFlagSet("apps", flag.ContinueOnError)
		},
	})

	AddCommand(Command{
		Name:        "events",
		Usage:       "events [--app <application id>]",
		Description: "Display the events for an application",
		Function:    cmdEvents,
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("events", flag.ExitOnError)
			fs.String("app", "", "Application ID")
			return fs
		},
	})

	AddCommand(Command{
		Name:        "stdout",
		Usage:       "stdout [--app <application id>]",
		Description: "Display the application's stdout",
		Function:    cmdStdout,
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("events", flag.ExitOnError)
			fs.String("app", "", "Application ID")
			return fs
		},
	})

	AddCommand(Command{
		Name:        "stderr",
		Usage:       "stderr [--app <application id>]",
		Description: "Display the application's stderr",
		Function:    cmdStderr,
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("events", flag.ExitOnError)
			fs.String("app", "", "Application ID")
			return fs
		},
	})

	AddCommand(Command{
		Name:        "validate",
		Usage:       "validate [--repo <git repository url>]",
		Description: "Validate the app's Bandaid file, does not deploy the application yet",
		Function:    cmdValidate,
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("events", flag.ExitOnError)
			fs.String("repo", "", "Clone URL")
			return fs
		},
	})

	AddCommand(Command{
		Name:        "deploy",
		Usage:       "deploy [--repo <git repository url>]",
		Description: "Deploy an app",
		Function:    cmdDeploy,
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("events", flag.ExitOnError)
			fs.String("repo", "", "Clone URL")
			return fs
		},
	})

	AddCommand(Command{
		Name:        "reload",
		Usage:       "reload [--app <application id>]",
		Description: "Reload an application",
		Function:    cmdReload,
		Flags: func() *flag.FlagSet {
			fs := flag.NewFlagSet("events", flag.ExitOnError)
			fs.String("app", "", "Application ID")
			return fs
		},
	})

}

func cmdReload(fl Flags) (int, error) {
	if err := printServerVersion(); err != nil {
		return 1, err
	}

	resp, err := (&http.Client{Timeout: time.Second * 10}).Get("http://localhost:2020/manager/app/" + fl.String("app") + "/reload")
	if err != nil {
		return 1, err
	}

	if resp.StatusCode != 200 {
		d, _ := ioutil.ReadAll(resp.Body)
		return 1, fmt.Errorf("Command failed: %v", string(d))
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 1, err
	}
	fmt.Println(string(b))
	return 0, nil
}

func cmdValidate(fl Flags) (int, error) {
	if err := printServerVersion(); err != nil {
		return 1, err
	}
	//resp, err := (&http.Client{Timeout: time.Second * 10}).Head("http://localhost:2020/manager/validate")
	resp, err := req.Post("http://localhost:2020/manager/validate", req.BodyJSON(gin.H{
		"repository": fl.String("repo"),
	}))
	if err != nil {
		return 1, err
	}
	if resp.Response().StatusCode != 200 {
		errCode, _ := resp.ToString()
		fmt.Println("Validation Failed:", errCode)
		return 1, err
	}
	fmt.Println("Validation: OK")
	return 0, nil
}

func cmdDeploy(fl Flags) (int, error) {
	if err := printServerVersion(); err != nil {
		return 1, err
	}
	//resp, err := (&http.Client{Timeout: time.Second * 10}).Head("http://localhost:2020/manager/validate")
	resp, err := req.Post("http://localhost:2020/manager/app", req.BodyJSON(gin.H{
		"repository": fl.String("repo"),
	}))
	if err != nil {
		return 1, err
	}
	data, _ := resp.ToString()
	if resp.Response().StatusCode != 200 {
		fmt.Println("Deploy Failed:", data)
		return 1, err
	}
	fmt.Println("Deploy:", data)
	return 0, nil
}

func cmdApps(fl Flags) (int, error) {
	if err := printServerVersion(); err != nil {
		return 1, err
	}

	resp, err := (&http.Client{Timeout: time.Second * 10}).Get("http://localhost:2020/manager/apps")
	if err != nil {
		return 1, err
	}

	if resp.StatusCode != 200 {
		d, _ := ioutil.ReadAll(resp.Body)
		return 1, fmt.Errorf("Command failed: %v", string(d))
	}

	var apps []AppStatusResponse
	err = json.NewDecoder(resp.Body).Decode(&apps)
	if err != nil {
		return 1, err
	}

	fmt.Println(stemp.Compile(
		"{id:w=32}  {repo:w=60} {status:w=8} {event}",
		gin.H{
			"id":     "ID",
			"repo":   "Repository",
			"status": "Healthy",
			"event":  "Last Event"}),
	)
	fmt.Println("--")

	for _, app := range apps {
		fmt.Println(stemp.Compile(
			"{id:w=32}  {repo:w=60} {status:w=8} {event}",
			gin.H{
				"id":     app.Application.ID,
				"repo":   app.Application.Repository,
				"status": !app.Status.Error,
				"event":  app.Application.Events[len(app.Application.Events)-1].Message}),
		)

	}

	return 0, nil
}

func cmdStdout(fl Flags) (int, error) {
	if err := printServerVersion(); err != nil {
		return 1, err
	}

	resp, err := (&http.Client{Timeout: time.Second * 10}).Get("http://localhost:2020/manager/app/" + fl.String("app") + "/stdout")
	if err != nil {
		return 1, err
	}

	if resp.StatusCode != 200 {
		d, _ := ioutil.ReadAll(resp.Body)
		return 1, fmt.Errorf("Command failed: %v", string(d))
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 1, err
	}
	fmt.Println(string(b))
	return 0, nil
}

func cmdStderr(fl Flags) (int, error) {
	if err := printServerVersion(); err != nil {
		return 1, err
	}

	resp, err := (&http.Client{Timeout: time.Second * 10}).Get("http://localhost:2020/manager/app/" + fl.String("app") + "/stderr")
	if err != nil {
		return 1, err
	}

	if resp.StatusCode != 200 {
		d, _ := ioutil.ReadAll(resp.Body)
		return 1, fmt.Errorf("Command failed: %v", string(d))
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 1, err
	}
	fmt.Println(string(b))
	return 0, nil
}

func cmdEvents(fl Flags) (int, error) {
	if err := printServerVersion(); err != nil {
		return 1, err
	}

	resp, err := (&http.Client{Timeout: time.Second * 10}).Get("http://localhost:2020/manager/app/" + fl.String("app"))
	if err != nil {
		return 1, err
	}

	if resp.StatusCode != 200 {
		d, _ := ioutil.ReadAll(resp.Body)
		return 1, fmt.Errorf("Command failed: %v", string(d))
	}

	var app AppStatusResponse
	err = json.NewDecoder(resp.Body).Decode(&app)
	if err != nil {
		return 1, err
	}
	for _, event := range app.Application.Events {
		fmt.Println(event)
	}
	return 0, nil
}

func cmdStatus(fl Flags) (int, error) {
	if err := printServerVersion(); err != nil {
		return 1, err
	}

	resp, err := (&http.Client{Timeout: time.Second * 10}).Get("http://localhost:2020/manager/app/" + fl.String("app"))
	if err != nil {
		return 1, err
	}

	if resp.StatusCode != 200 {
		d, _ := ioutil.ReadAll(resp.Body)
		return 1, fmt.Errorf("Command failed: %v", string(d))
	}

	var app AppStatusResponse
	err = json.NewDecoder(resp.Body).Decode(&app)
	if err != nil {
		return 1, err
	}

	fmt.Println(app.Application.ID)
	fmt.Println("  ", app.Application.Repository, "\n")

	if app.Status.Error {
		fmt.Println("STATUS: ERROR")
		if app.Status.ServiceError != nil {
			fmt.Println("[!] Service Error", app.Status.ServiceError.Code, app.Status.ServiceError.Content)
		}
		if app.Status.DialError != "" {
			fmt.Println("[!] Dial Error", app.Status.DialError)
		}
	} else {
		fmt.Println("STATUS: OK")
		fmt.Println("Last Event:", app.Application.Events[len(app.Application.Events)-1])
	}
	return 0, nil
}

func AddCommand(cmd Command) {
	if cmd.Function == nil {
		panic(fmt.Errorf("Command '%v' does not have a function associated with it", cmd.Name))
	}
	commands[cmd.Name] = &cmd
}

func getVersion() (string, error) {
	resp, err := (&http.Client{Timeout: time.Second * 10}).Get("http://localhost:2020/")
	if err != nil {
		return "", err
	}
	version, err := ioutil.ReadAll(resp.Body)
	return string(version), err
}

func printServerVersion() error {
	version, err := getVersion()
	if err != nil {
		fmt.Println("Failed to get server version, perhaps it's not running?")
		return err
	}
	fmt.Printf("Running Server Version: %v\n\n", version)
	return nil
}
