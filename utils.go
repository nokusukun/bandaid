package bandaid

import "github.com/levigross/grequests"

func GetIP() (string, error) {
	resp, err := grequests.Get("https://v4.ident.me/", nil)
	if err != nil {
		return "", err
	}
	return resp.String(), nil
}
