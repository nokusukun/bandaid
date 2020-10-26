package main

import (
	"fmt"
	"time"
)

type AppStatusResponse struct {
	Application Application `json:"application"`
	Status      Status      `json:"status"`
	Error       string      `json:"error"`
}

type Application struct {
	Repository string  `json:"repository"`
	ID         string  `json:"id"`
	Events     []Event `json:"events"`
}

type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

func (e Event) String() string {
	return fmt.Sprintf("<%v ago> %v", time.Now().Sub(e.Timestamp).Truncate(time.Second).String(), e.Message)
}

type Status struct {
	Configuration Configuration `json:"configuration"`
	DialError     string        `json:"dial_error,omitempty"`
	ServiceError  *ServiceError `json:"service_error,omitempty"`
	Error         bool          `json:"error"`
}

type ServiceError struct {
	Code    int    `json:"code,omitempty"`
	Content string `json:"content,omitempty"`
}

type Configuration struct {
	Caddy  Caddy  `json:"caddy"`
	DNS    DNS    `json:"dns"`
	Force  bool   `json:"force"`
	Health Health `json:"health"`
}

type Caddy struct {
	Domains []string `json:"domains"`
	Host    string   `json:"host"`
}

type DNS struct {
	Domain  string `json:"domain"`
	Proxied bool   `json:"proxied"`
	Zone    string `json:"zone"`
}

type Health struct {
	CheckURL string `json:"check_url"`
}
