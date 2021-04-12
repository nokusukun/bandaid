package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req"
	"github.com/levigross/grequests"
	"github.com/nokusukun/bandaid"
	"gopkg.in/ini.v1"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	ACT_SHOWACTIONS = "button-actions"
	ACT_RELOAD      = "button-reload"
	ACT_KILL        = "button-kill"
	ACT_STDOUT      = "button-log-stdout"
	ACT_STDERR      = "button-log-stderr"
	ACT_EVENTS      = "button-events"
)

func SlackEngine(bind string, config *ini.File) error {
	engine := gin.Default()

	api := &Oakland{
		LogHook: config.Section("slack").Key("webhook").String(),
	}

	slack := engine.Group("/slack")
	{
		slack.POST("/apps", api.GetApplicationStatus)
		slack.POST("/deploy", api.DeployApplication)
		slack.POST("/validate", api.ValidateApplication)

		slack.POST("/interact", api.Interact)
	}

	err := bandaid.AutoCloudflare(config.Section("cloudflare").Key("kingslandtesting.com").String()).
		SetZone("kingslandtesting.com").
		SetDomain("oakland-slack.kingslandtesting.com").
		Proxied(true).
		Reinstall()
	if err != nil {
		panic(err)
	}

	return bandaid.AutoCaddy("oakland-slack").
		SetDomain(bandaid.DomainConfig{
			Host: []string{"oakland-slack.kingslandtesting.com"},
		}).
		SetHost(bind).
		AttemptInitializeCaddy().
		ApplyAndRun(func(host string) error {
			//return router.Run(host)
			return engine.Run(host)
		})

	return engine.Run(bind)
}

type SlackCommand struct {
	Token       string `form:"token"`
	Command     string `form:"command"`
	Text        string `form:"text"`
	ResponseURL string `form:"response_url"`
	TriggerID   string `form:"trigger_id"`
	UserID      string `form:"user_id"`
	UserName    string `form:"user_name"`
	APIAppID    string `form:"api_app_id"`
}

func (s SlackCommand) Reply(text string) error {
	p, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	_, err = (&http.Client{Timeout: 10 * time.Second}).Post(s.ResponseURL, "application/json", bytes.NewBuffer(p))
	return err
}

type Oakland struct {
	LogHook string
}

func (Oakland) DeployApplication(g *gin.Context) {
	command := SlackCommand{}
	if IsErrorSlack(g.Bind(&command), "", command.Command, g) {
		return
	}

	commands := strings.Split(command.Text, " ")
	repo := commands[0]
	config := ""
	if len(commands) == 2 {
		config = commands[1]
	}

	payload, err := json.Marshal(gin.H{
		"repository": repo,
		"config":     config,
	})

	if IsErrorSlack(err, "", command.Command, g) {
		return
	}
	go func() {
		r, err := (&http.Client{Timeout: 60 * time.Second}).Post("http://"+manager_address+"/manager/app", "application/json", bytes.NewBuffer(payload))
		if IsErrorSlack(err, "Oakland management server seems to be down", command.Command, g) {
			return
		}
		response, err := ioutil.ReadAll(r.Body)
		if r.StatusCode != 200 {
			command.Reply(fmt.Sprintf("Deploy failed: `%v`", string(response)))
		} else {
			//command.Reply(fmt.Sprintf("Deploy Success: `%v`", string(response)))
			LogEvent(
				BuildSuccessBlock(fmt.Sprintf("%v(%v)", command.UserName, command.UserID), "Deploy Application", strings.TrimSpace(command.Text)),
				command.ResponseURL,
			)
		}
	}()
	g.JSON(200, gin.H{"text": "Deploying application..."})
}

func (Oakland) ValidateApplication(g *gin.Context) {
	command := SlackCommand{}
	if IsErrorSlack(g.Bind(&command), "", command.Command, g) {
		return
	}

	commands := strings.Split(command.Text, " ")
	repo := commands[0]
	config := ""
	if len(commands) == 2 {
		config = commands[1]
	}

	payload, err := json.Marshal(gin.H{
		"repository": repo,
		"config":     config,
	})
	if IsErrorSlack(err, "", command.Command, g) {
		return
	}
	go func() {
		r, err := (&http.Client{Timeout: 60 * time.Second}).Post("http://"+manager_address+"/manager/validate", "application/json", bytes.NewBuffer(payload))
		if IsErrorSlack(err, "Oakland management server seems to be down", command.Command, g) {
			return
		}
		response, err := ioutil.ReadAll(r.Body)
		if r.StatusCode != 200 {
			command.Reply(fmt.Sprintf("Validate failed: `%v`", string(response)))
		} else {
			command.Reply(fmt.Sprintf("Validate Success: `%v`", string(response)))
		}
	}()
	g.JSON(200, gin.H{"text": "Validating application..."})
}

func (Oakland) GetApplicationStatus(g *gin.Context) {
	command := SlackCommand{}
	if IsErrorSlack(g.Bind(&command), "", command.Command, g) {
		return
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Get("http://" + manager_address + "/manager/apps")
	if IsErrorSlack(err, "Oakland management server seems to be down", command.Command, g) {
		return
	}

	statuses := []*AppStatus{}
	err = json.NewDecoder(response.Body).Decode(&statuses)
	if IsErrorSlack(err, "Cannot decode body to JSON", command.Command, g) {
		return
	}

	if len(statuses) == 0 {
		g.JSON(200, gin.H{"text": "No deployed applications"})
		return
	}

	blocks := []gin.H{}
	for _, status := range statuses {
		emoji := "✔️"
		switch v := status.Status.(map[string]interface{})["error"].(type) {
		case bool:
			if v {
				emoji = "❌"
			}
		case string:
			if v != "" {
				emoji = "❌"
			}
		default:
			emoji = "❓"
		}
		blocks = append(blocks, gin.H{
			"type": "section",
			"text": gin.H{
				"type": "mrkdwn",
				"text": fmt.Sprintf("%v`%v`\n*<%v>*\nLast Event: `%v`",
					emoji,
					status.Application.ID,
					status.Application.Repository,
					status.Application.Events[len(status.Application.Events)-1].Message,
				),
			},
			"accessory": gin.H{
				"type": "button",
				"text": gin.H{
					"type":  "plain_text",
					"text":  "Actions",
					"emoji": true,
				},
				"value":     fmt.Sprintf("%v;%v", status.Application.ID, status.Application.Repository),
				"action_id": ACT_SHOWACTIONS,
			},
		}, gin.H{
			"type": "divider",
		})
	}

	commandResponse := gin.H{
		"blocks":        blocks,
		"response_type": "ephemeral",
	}
	g.JSON(200, commandResponse)
}

func (app *Oakland) Interact(g *gin.Context) {
	wrapper := InteractPayloadWrapper{}
	if IsErrorSlack(g.Bind(&wrapper), "failed to read interaction payload", "interaction", g) {
		return
	}

	payload := wrapper.Payload
	log.Println("Parsing", payload)
	action := payload.Actions[0]
	var err error
	switch action.ActionID {
	case ACT_SHOWACTIONS:
		fmt.Println("Showing action buttons")
		app.InteractShowActionButtons(action, payload.ResponseURL)
	case ACT_RELOAD:
		err = app.InteractReloadApplication(action.ApplicationID())
		if err == nil {
			_, _ = grequests.Post(payload.ResponseURL, &grequests.RequestOptions{
				JSON: gin.H{"text": "Operation Successful"}})
			LogEvent(BuildSuccessBlock(fmt.Sprintf("%v(%v)", payload.User.Username, payload.User.ID), "Reload", action.ApplicationRepo()), app.LogHook)
		}
	case ACT_KILL:
		err = app.InteractDeleteApplication(action.ApplicationID(), payload.ResponseURL)
		if err == nil {
			LogEvent(BuildSuccessBlock(fmt.Sprintf("%v(%v)", payload.User.Username, payload.User.ID), "Delete", action.ApplicationRepo()), app.LogHook)
		}
		//_, _ = grequests.Post(payload.ResponseURL, &grequests.RequestOptions{
		//	JSON: gin.H{"text": "[Notice] This might be too powerful to include as a slack command, please do it manually through the CLI"}})
	case ACT_STDOUT:
		err = app.InteractStdout(action.ApplicationID(), payload.ResponseURL)
	case ACT_STDERR:
		err = app.InteractStderr(action.ApplicationID(), payload.ResponseURL)
	case ACT_EVENTS:
		err = app.InteractEvents(action.ApplicationID(), payload.ResponseURL)
	default:
		fmt.Println("no interaction found")
		app.InteractDefaultResponse(action, payload.ResponseURL)
	}

	IsErrorSlack(err, "", action.ActionID, payload.ResponseURL)
}

func (app *Oakland) InteractDefaultResponse(action Action, responseURL string) {
	_, _ = grequests.Post(responseURL, &grequests.RequestOptions{JSON: gin.H{"text": "Unknown action selected: " + action.ActionID}})
}

func (app *Oakland) InteractReloadApplication(id string) error {
	resp, err := (&http.Client{Timeout: time.Minute * 2}).Get(fmt.Sprintf("http://"+manager_address+"/manager/app/%v/reload", id))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Command failed")
	}
	return nil
}

func (app *Oakland) InteractDeleteApplication(id string, responseURL string) error {
	resp, _ := req.Delete(fmt.Sprintf("http://"+manager_address+"/manager/app/%v", id))
	//if err != nil {
	//	return err
	//}
	if resp.Response().StatusCode != 200 {
		return fmt.Errorf("Command failed: %v", resp.String())
	} else {
		_, _ = req.Post(responseURL, gin.H{
			"text": fmt.Sprintf("Command Success: %v", resp.String()),
		})
	}
	return nil
}

func (app *Oakland) InteractStdout(id string, responseURL string) error {
	resp, err := (&http.Client{Timeout: time.Minute * 2}).Get(fmt.Sprintf("http://%v/manager/app/%v/stdout", manager_address, id))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Command failed")
	}
	msg, err := ioutil.ReadAll(resp.Body)
	_, _ = grequests.Post(responseURL, &grequests.RequestOptions{JSON: gin.H{"text": fmt.Sprintf("```%v```", string(msg))}})
	return err
}

func (app *Oakland) InteractStderr(id string, responseURL string) error {
	resp, err := (&http.Client{Timeout: time.Minute * 2}).Get(fmt.Sprintf("http://%v/manager/app/%v/stderr", manager_address, id))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Command failed")
	}
	msg, err := ioutil.ReadAll(resp.Body)
	_, _ = grequests.Post(responseURL, &grequests.RequestOptions{JSON: gin.H{"text": fmt.Sprintf("```%v```", string(msg))}})
	return err
}

func (app *Oakland) InteractEvents(id string, responseURL string) error {
	resp, err := (&http.Client{Timeout: time.Minute * 2}).Get(fmt.Sprintf("http://%v/manager/app/%v/events", manager_address, id))
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("Command failed")
	}
	events := []AppEvent{}
	err = json.NewDecoder(resp.Body).Decode(&events)
	if err != nil {
		return err
	}

	blocks := []gin.H{}

	for _, event := range events {
		ts := event.Timestamp.Format(time.Stamp)
		msgtype := "⚠️ Error"
		msg := fmt.Sprintf("%v", event.Error)
		if event.Message != "" {
			msgtype = "ℹ️ Message"
			msg = event.Message
		}
		blocks = append(blocks, gin.H{
			"type": "section",
			"text": gin.H{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*%v* %v\n`%v`", ts, msgtype, msg),
			},
		}, gin.H{
			"type": "divider",
		})
	}

	commandResponse := gin.H{
		"blocks":        blocks,
		"response_type": "ephemeral",
	}
	_, err = grequests.Post(responseURL, &grequests.RequestOptions{
		JSON: commandResponse,
	})
	if err != nil {
		log.Println("Post response failed", err)
	}
	return err
}

func (app *Oakland) InteractShowActionButtons(action Action, responseURL string) {
	value := strings.Split(action.Value, ";")
	repo := value[1]
	blocks := []gin.H{
		{
			"type": "section",
			"text": gin.H{
				"type": "mrkdwn",
				"text": fmt.Sprintf("Actions for: `%v`", repo),
			},
		},
		{
			"type": "actions",
			"elements": []gin.H{
				{
					"type": "button",
					"text": gin.H{
						"type": "plain_text",
						"text": "Reload",
					},
					"value":     action.Value,
					"action_id": ACT_RELOAD,
					"style":     "primary",
				},
				{
					"type": "button",
					"text": gin.H{
						"type": "plain_text",
						"text": "Kill",
					},
					"style":     "danger",
					"value":     action.Value,
					"action_id": ACT_KILL,
				},
				{
					"type": "button",
					"text": gin.H{
						"type": "plain_text",
						"text": "Log:stdout",
					},
					"value":     action.Value,
					"action_id": ACT_STDOUT,
				},
				{
					"type": "button",
					"text": gin.H{
						"type": "plain_text",
						"text": "Log:stderr",
					},
					"value":     action.Value,
					"action_id": ACT_STDERR,
				},
				{
					"type": "button",
					"text": gin.H{
						"type": "plain_text",
						"text": "Events",
					},
					"value":     action.Value,
					"action_id": ACT_EVENTS,
				},
			},
		},
	}
	commandResponse := gin.H{
		"blocks":        blocks,
		"response_type": "ephemeral",
	}
	//g.JSON(200, commandResponse)
	resp, err := grequests.Post(responseURL, &grequests.RequestOptions{
		JSON: commandResponse,
	})
	if err != nil {
		log.Println("Post response failed", err)
	}
	log.Println(resp.String())
}

func BuildSuccessBlock(who, what, where string) gin.H {
	return gin.H{
		"response_type": "in_channel",
		"blocks": []gin.H{
			{
				"type": "header",
				"text": gin.H{
					"type":  "plain_text",
					"text":  "Operation Successful 🥳",
					"emoji": true,
				},
			},
			{
				"type": "section",
				"fields": []gin.H{
					{
						"type": "mrkdwn",
						"text": "*What:*\n" + what,
					},
					{
						"type": "mrkdwn",
						"text": "*Where:*\n" + where,
					},
				},
			},
			{
				"type": "context",
				"elements": []gin.H{
					{
						"type":  "plain_text",
						"text":  "Command executed by: " + who,
						"emoji": true,
					},
				},
			},
		},
	}
}

func LogEvent(blocks gin.H, hook string) {
	_, _ = grequests.Post(hook, &grequests.RequestOptions{
		JSON: blocks,
	})
}

type InteractPayloadWrapper struct {
	Payload InteractPayload `form:"payload"`
}

type InteractPayload struct {
	Type        string    `json:"type"`
	User        User      `json:"user"`
	APIAppID    string    `json:"api_app_id"`
	Token       string    `json:"token"`
	Container   Container `json:"container"`
	TriggerID   string    `json:"trigger_id"`
	Team        Team      `json:"team"`
	Channel     Channel   `json:"channel"`
	ResponseURL string    `json:"response_url"`
	Actions     []Action  `json:"actions"`
}

type Action struct {
	ActionID string `json:"action_id"`
	BlockID  string `json:"block_id"`
	Text     Text   `json:"text"`
	Value    string `json:"value"`
	Type     string `json:"type"`
	ActionTs string `json:"action_ts"`
}

func (a Action) ApplicationRepo() string {
	return strings.Split(a.Value, ";")[1]
}
func (a Action) ApplicationID() string {
	return strings.Split(a.Value, ";")[0]
}

type Text struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Emoji bool   `json:"emoji"`
}

type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Container struct {
	Type        string `json:"type"`
	MessageTs   string `json:"message_ts"`
	ChannelID   string `json:"channel_id"`
	IsEphemeral bool   `json:"is_ephemeral"`
}

type Team struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	TeamID   string `json:"team_id"`
}

func IsErrorSlack(err error, message string, source string, responder interface{}) bool {
	hasError := err != nil
	if hasError {
		log.Println("Oakland error", err, message)
		if message == "" {
			message = fmt.Sprintf("`%v`", err)
		}
		//payload := []byte(stemp.Compile(`{
		//"blocks": [
		//	{
		//		"type": "header",
		//		"text": {
		//			"type": "plain_text",
		//			"text": "💥 Command Failed",
		//			"emoji": true
		//		}
		//	},
		//	{
		//		"type": "section",
		//		"text": {
		//			"type": "mrkdwn",
		//			"text": "{message}"
		//		}
		//	},
		//	{
		//		"type": "context",
		//		"elements": [
		//			{
		//				"type": "mrkdwn",
		//				"text": "Source: {source}"
		//			}
		//		]
		//	}
		//]
		//}`, map[string]interface{}{
		//	"source":  source,
		//	"message": message,
		//}))

		blocks := []gin.H{
			{
				"type": "header",
				"text": gin.H{
					"type":  "plain_text",
					"text":  "💥 Command Failed",
					"emoji": true,
				},
			},
			{
				"type": "section",
				"text": gin.H{
					"type": "mrkdwn",
					"text": message,
				},
			},
			{
				"type": "context",
				"elements": []gin.H{
					{
						"type": "mrkdwn",
						"text": "Source: " + source,
					},
				},
			},
		}
		payload := gin.H{"blocks": blocks}

		switch responder.(type) {
		case string:
			_, _ = req.Post(responder.(string), req.BodyJSON(payload))
		case *gin.Context:
			responder.(*gin.Context).JSON(200, payload)
		}
		return true
	}
	return false
}
