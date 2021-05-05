package wsserver

import (
	"log"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	. "github.com/smartystreets/goconvey/convey"
)

var wsServer *WS
var runned []string

func TestMain(m *testing.M) {
	var err error

	wsServer, err = Start(&Config{
		Addr:     ":6008",
		Handlers: THandlers{},
	})
	if err != nil {
		log.Print(err)
	}
	os.Exit(m.Run())
}

type THandlers struct{}
func (h THandlers) SetConnCtrlr(ctrlr ConnController) {}
func (h THandlers) OnAuth(token string) (id uint, ok bool) {
	runned = append(runned, "OnAuth")
	return 1, true
}
func (h THandlers) OnOnline(id uint){
	runned = append(runned, "OnOnline")
}
func (h THandlers) OnText(id uint, msg []byte){
	runned = append(runned, "OnText")
}
func (h THandlers) OnSend(id uint, msg []byte) (ok bool){
	runned = append(runned, "OnSend")
	return true
}
func (h THandlers) OnOffline(id uint){
	runned = append(runned, "OnOffline")
}

func TestConnectHandlers(t *testing.T) {
	Convey("Given WS server", t, func() {
		Convey("When we connect by websocket to server", func() {
			runned = make([]string, 0)
			c := setWSConnection()
			Convey("Then 'OnAuth' handler should be runned", func() {
				So(runned, ShouldContain, "OnAuth")
				Convey("And 'OnAuth' should be first", func() {
					So(runned[0], ShouldEqual, "OnAuth")
				})
			})
			Convey("Then 'OnOnline' handler should be runned", func() {
				So(runned, ShouldContain, "OnOnline")
			})
			Reset(func() {
				c.Close()
			})
		})
	})
}

func TestConnectWithoutAuth(t *testing.T) {
	Convey("Given WS server", t, func() {
		Convey("When we connect by websocket to server without token", func() {
			runned = make([]string, 0)
			u := url.URL{
				Scheme: "ws",
				Host: "localhost:6008",
				Path: "/",
			}
			_, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				log.Print(err)
			}

			Convey("Then error should not be nil ", func() {
				So(err, ShouldNotBeNil)
				Convey("And err should be 'bad handshake'", func() {
					So(err, ShouldEqual, websocket.ErrBadHandshake)
				})
			})
		})
	})
}

func TestConnectWithAuthorizationHeader(t *testing.T) {
	Convey("Given WS server", t, func() {
		Convey("When we connect by websocket to server with 'Authorization' header", func() {
			runned = make([]string, 0)
			u := url.URL{
				Scheme: "ws",
				Host: "localhost:6008",
				Path: "/",
			}
			c, _, err := websocket.DefaultDialer.Dial(u.String(), http.Header{
				"Authorization": []string{"Bearer 123456"},
			})
			if err != nil {
				log.Print(err)
			}

			Convey("Then 'OnAuth' handler should be runned", func() {
				So(runned, ShouldContain, "OnAuth")
				Convey("And 'OnAuth' should be first", func() {
					So(runned[0], ShouldEqual, "OnAuth")
				})
			})
			Convey("Then 'OnOnline' handler should be runned", func() {
				So(runned, ShouldContain, "OnOnline")
			})
			Reset(func() {
				c.Close()
			})
		})
	})
}

func TestReceiveMessageHandlers(t *testing.T) {
	Convey("Given client with connection to server", t, func() {
		runned = make([]string, 0)
		c := setWSConnection()
		Convey("When we receive message", func() {
			c.WriteMessage(websocket.TextMessage, []byte("Hello websocket! I'm client"))
			Convey("Then 'OnText' handler should be runned", func() {
				time.Sleep(time.Second*1) //TODO: How test without sleep??
				So(runned, ShouldContain, "OnText")
			})
		})
		Reset(func() {
			c.Close()
		})
	})
}

func TestSendMessageHandlers(t *testing.T) {
	Convey("Given server with client connections", t, func() {
		runned = make([]string, 0)
		var connID uint
		c := setWSConnection()
		Convey("When server send message to client", func() {
			Convey("And client exist", func() {
				connID = 1
				err := wsServer.WriteMessage(connID, []byte("Hello, i'm ws server"))
				Convey("Then 'OnSend' handler should be runned", func() {
					So(runned, ShouldContain, "OnSend")
				})
				Convey("Then err should be nil", func() {
					So(err, ShouldBeNil)
				})
			})
			Convey("And client not exist", func() {
				connID = 2
				err := wsServer.WriteMessage(connID, []byte("Hello, i'm ws server"))
				Convey("Then err should not be nil", func() {
					So(err, ShouldNotBeNil)
					Convey("And error should be 'Connection not found'", func() {
						So(err, ShouldEqual, ErrConnNotFound)
					})
				})
			})
		})
		Reset(func() {
			c.Close()
		})
	})
}

func TestDisconnectHandlers(t *testing.T) {
	Convey("Given server with client connections", t, func() {
		runned = make([]string, 0)
		c := setWSConnection()
		Convey("When client close connection with server", func() {
			c.Close()
			Convey("Then 'OnOffline' handler should be runned", func() {
				time.Sleep(time.Second*1) //TODO: How test without sleep??
				So(runned, ShouldContain, "OnOffline")
			})
		})
		Reset(func() {
			c.Close()
		})
	})
}

func TestCloseConnectionHandlers(t *testing.T) {
	Convey("Given server with client connections", t, func() {
		runned = make([]string, 0)
		c := setWSConnection()
		Convey("When server close connection with client", func() {
			wsServer.CloseConnection(1)
			Convey("Then 'OnOffline' handler should be runned", func() {
				time.Sleep(time.Second*1) //TODO: How test without sleep??
				So(runned, ShouldContain, "OnOffline")
			})
		})
		Reset(func() {
			c.Close()
		})
	})
}

func setWSConnection() *websocket.Conn {
	u := url.URL{
		Scheme: "ws",
		Host: "localhost:6008",
		Path: "/",
		RawQuery: "token=123456",
	}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Print(err)
	}
	//log.Printf("connection from: %v", c.LocalAddr())

	return c
}