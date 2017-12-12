# caddyshack
Website Cloner and Credential Harvester written in Go

Written by: Brennan Turner (@BLTSEC)
Website: https://bltsec.ninja

<img width="40%" src=https://raw.githubusercontent.com/BLTSEC/go-webscraper/master/govatar.png></img>


## Requirements
* git
* go
* wget

## Installation on macOS
	brew install git
	brew install go
	
## Installation on Windows
	choco install git.install
	choco install golang
	
## Usage
### Build Binary 
	GOOS=linux go build -ldflags="-s -w" caddyshack.go
### Run Binary
	./caddyshack --url https://site.com --port 8006

