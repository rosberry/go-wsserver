# wsserver
Library for work with websockets.

Wrapper for [gobwas/ws](https://github.com/gobwas/ws) 

## Usage

```go
package main

import (
	"ws"
	"github.com/rosberry/go-wsserver"
	"github.com/rosberry/go-wsserver/debugger"
)

var DebugMode bool

func main() {
	handlers := ws.GetHandlers()
	//start debug
	if _, err := debugger.New(&debugger.Config{
		Addr:     ":6007",
		Handlers: &handlers,
	}); err != nil {

		log.Println("Failed start debugger")
	}

	//start
	if _, err := wsserver.Start(&wsserver.Config{
			Addr:     ":6006",
			Handlers: handlers,
		}); err != nil {

		panic(err)
	}
}
```

```go
package ws

type MyHandler struct{
	cc	wsserver.ConnController
}

func GetGetHandlers() *MyHandler {
	return &MyHandler{}
}

func (h *MyHandler) OnAuth(token string) (id uint, ok bool) {
	//do anything
	//get id by token
	id = a.GetIdByToken(token)
	return id, true
}

func (h *MyHandler) OnOnline(id uint) {
	//do anything
	log.Println("%b ONLINE", id)
}

func (h *MyHandler) OnOffline(id uint) {
	//do anything
	log.Println("%b OFFLINE", id)
}

func (h *MyHandler) OnText(id uint, msg []byte) {
	//do anything
	log.Printf("[Message IN](%v) %s\n", id, string(msg))
}

func (h *MyHandler) OnSend(id uint, msg []byte) (ok bool){
	fmt.Printf("[Message OUT](%v) %s\n", id, string(msg))
	return true
}

func (h *MyHandler) SetConnCtrlr(cc wsserver.ConnController) {
	h.cc = cc
}
```

## About

<img src="https://github.com/rosberry/Foundation/blob/master/Assets/full_logo.png?raw=true" height="100" />

This project is owned and maintained by [Rosberry](http://rosberry.com). We build mobile apps for users worldwide 🌏.

Check out our [open source projects](https://github.com/rosberry), read [our blog](https://medium.com/@Rosberry) or give us a high-five on 🐦 [@rosberryapps](http://twitter.com/RosberryApps).

## License

This project is available under the MIT license. See the LICENSE file for more info.
