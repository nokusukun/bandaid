# Bandaid
---
Automate caddy and cloudflare reverse proxying within go.

### Install
`go get github.com/nokusukun/bandaid`

### Usage
```go
package main

import (
	"bandaid"
	"github.com/gin-gonic/gin"
)

func main()  {
	router := gin.Default()
	router.GET("/", func(context *gin.Context) {
		context.String(200, "Hello there from bandaid!")
	})
	
    // Cloudflare auto configuration
    err := bandaid.AutoCloudflare("cloudflare-api-token").
		SetZone("noku.pw").
		SetDomain("example.noku.pw").
		Proxied(true).
		Install()
    
    // Caddy reverse proxy configuration
	err := bandaid.AutoCaddy("sample-application").
		SetDomain(bandaid.DomainConfig{
			Host: []string{"subdomain.example.com"},
		}).
		AttemptInitializeCaddy().
		ApplyAndRun(func(host string) error {
			return router.Run(host)
		})

	if err != nil {
		panic(err)
	}
}
```


## Management Server
Bandaid also offers a management server if your application isn't on go. It provides the same 
provisions as the Bandaid library but in a self contained managed way.

### Requirements
* Go 1.1x
* Caddy running

### Installing and Running
```bash
$ git clone https://github.com/nokusukun/bandaid
$ cd ./bandaid/management_server
$ mv config.default.ini config.ini
$ nano config.ini /// ... add tokens here
$ go run .
```
The management server runs on `http://localhost:2020`

### Usage
Sample python flask app
```python3
import flask, requests

app = Flask(__name__)

@app.route('/')
def hello():
    return "Hello World!"

# not required, it just needs anything that returns a 200 code
@app.route('/ping')
def ping():
    return "OK"

if __name__ == '__main__':
    bandaid_config = {
        "dns": {
            "zone": "noku.pw",
            "domain": "sampley.noku.pw",
            "proxied": true
        },
        "caddy": {
            "domains": ["sampley.noku.pw", "dev.internal"]
        },
        "health": {
            "check_url": "ping"
        }
    }
    response = requests.post("http://localhost:2020/launch/my-sample-app", data = bandaid_config)
    app.run(response.json()['host'])
```