// This file was generated from JSON Schema using quicktype, do not modify it directly.
// To parse and unparse this JSON data, add this code to your project and do:
//
//    gitlabHookPayload, err := UnmarshalGitlabHookPayload(bytes)
//    bytes, err = gitlabHookPayload.Marshal()

package main

import "encoding/json"

func UnmarshalGitlabHookPayload(data []byte) (GitlabHookPayload, error) {
	var r GitlabHookPayload
	err := json.Unmarshal(data, &r)
	return r, err
}

func (r *GitlabHookPayload) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type GitlabHookPayload struct {
	ObjectKind        string        `json:"object_kind"`
	Before            string        `json:"before"`
	After             string        `json:"after"`
	Ref               string        `json:"ref"`
	CheckoutSHA       string        `json:"checkout_sha"`
	UserID            int64         `json:"user_id"`
	UserName          string        `json:"user_name"`
	UserAvatar        string        `json:"user_avatar"`
	ProjectID         int64         `json:"project_id"`
	Project           Project       `json:"project"`
	Repository        Repository    `json:"repository"`
	Commits           []interface{} `json:"commits"`
	TotalCommitsCount int64         `json:"total_commits_count"`
}

type Project struct {
	ID                int64       `json:"id"`
	Name              string      `json:"name"`
	Description       string      `json:"description"`
	WebURL            string      `json:"web_url"`
	AvatarURL         interface{} `json:"avatar_url"`
	GitSSHURL         string      `json:"git_ssh_url"`
	GitHTTPURL        string      `json:"git_http_url"`
	Namespace         string      `json:"namespace"`
	VisibilityLevel   int64       `json:"visibility_level"`
	PathWithNamespace string      `json:"path_with_namespace"`
	DefaultBranch     string      `json:"default_branch"`
	Homepage          string      `json:"homepage"`
	URL               string      `json:"url"`
	SSHURL            string      `json:"ssh_url"`
	HTTPURL           string      `json:"http_url"`
}

type Repository struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	Description     string `json:"description"`
	Homepage        string `json:"homepage"`
	GitHTTPURL      string `json:"git_http_url"`
	GitSSHURL       string `json:"git_ssh_url"`
	VisibilityLevel int64  `json:"visibility_level"`
}
