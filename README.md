## ynoserver
Server for https://github.com/ynoproject/ynoclient

## Configuring
Change the number of rooms in main.go:
```
NUM_ROOMS = 180 //!!! change this if not hosting yume nikki
```
Set the PORT environment variable. If you don't, it defaults to 8080.

## Building
```
git clone https://github.com/ynoproject/ynoserver
cd ynoserver
go mod download github.com/gorilla/websocket
go get gopkg.in/natefinch/lumberjack.v2
go get gopkg.in/yaml.v2
go get golang.org/x/text/unicode/norm
go get github.com/go-sql-driver/mysql
go build
```

## Setting up
1) Build https://github.com/ynoproject/ynoclient
2) Put index.js and index.wasm in public/
3) Put the game files in public/play/gamesdefault
4) Run gencache in public/play/gamesdefault (can be found here https://easyrpg.org/player/guide/webplayer/)
5) Run ynoserver (or push to heroku)

## Credits
Based on https://github.com/gorilla/websocket/tree/master/examples/chat
