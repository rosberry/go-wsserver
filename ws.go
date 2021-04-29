package wsserver

import (
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type (
	Logger interface {
		Fatal(v ...interface{})
		Fatalf(format string, v ...interface{})
		Fatalln(v ...interface{})
		Panic(v ...interface{})
		Panicf(format string, v ...interface{})
		Panicln(v ...interface{})
		Print(v ...interface{})
		Printf(format string, v ...interface{})
		Println(v ...interface{})
	}

	Handlers interface {
		SetConnCtrlr(ctrlr ConnController)
		OnAuth(token string) (id uint, ok bool)
		OnOnline(id uint)
		OnText(id uint, msg []byte)
		OnSend(id uint, msg []byte) (ok bool)
		OnOffline(id uint)
	}

	ConnController interface {
		WriteMessage(id uint, msg []byte) (err error)
		CloseConnection(id uint) (err error)
	}

	Config struct {
		Addr     string
		Handlers Handlers
		Logger   Logger
	}

	WS struct {
		conns map[uint]net.Conn
		addr  string
		h     Handlers
		l     Logger
		mutex *sync.RWMutex
	}

	Message struct {
		Body []byte
		Op   ws.OpCode
		Err  error
	}
)

const (
	TimeoutPing  = 30 * time.Second
	TimeoutClose = 15 * time.Second
)

const (
	LoggerDefaultPrefix = "[WS]"
	AuthTokenKey        = "token"
)

var (
	ErrEmptyConfig   = errors.New("Empty config")
	ErrBadAuthHeader = errors.New("Bad Authorization header")
	ErrAuthFailed    = errors.New("Bad token")
	ErrNotAuth       = errors.New("Token not found")
	ErrConnNotFound  = errors.New("Connection not found")
)

func Start(cfg *Config) (*WS, error) {
	if cfg == nil {
		return nil, ErrEmptyConfig
	}
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stdout, LoggerDefaultPrefix, log.Ldate|log.Ltime|log.LUTC)
	}

	w := WS{
		conns: make(map[uint]net.Conn),
		h:     cfg.Handlers,
		l:     cfg.Logger,
		mutex: &sync.RWMutex{},
	}

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, err
	}
	w.addr = ln.Addr().String()
	w.l.Printf("Websocket is listening on %s", w.addr)

	go func() {
		for {
			if conn, err := ln.Accept(); err == nil {
				go w.handle(conn)
			} else {
				w.l.Printf("Start connection error: %s", err)
			}
		}
	}()

	cfg.Handlers.SetConnCtrlr(&w)
	return &w, nil
}

func (w *WS) handle(conn net.Conn) {
	defer conn.Close()
	var id uint

	u := ws.Upgrader{
		OnRequest: func(uri []byte) error {
			if u, err := url.Parse(string(uri)); err == nil && u.RawQuery != "" {
				if m, e := url.ParseQuery(u.RawQuery); e == nil {
					if token, ok := m[AuthTokenKey]; ok {
						if id, ok = w.onAuthWrapper(token[0]); !ok {
							return ErrAuthFailed
						}
					}
				}
			}
			return nil
		},
		OnHeader: func(key, value []byte) error {
			if id == 0 && string(key) == "Authorization" {
				v := string(value)
				switch {
				case strings.HasPrefix(v, "Bearer "), strings.HasPrefix(v, "Basic "):
					var ok bool
					if id, ok = w.onAuthWrapper(strings.SplitN(v, " ", 2)[1]); !ok {
						return ErrAuthFailed
					}
				default:
					return ErrBadAuthHeader
				}
			}
			return nil
		},
		OnBeforeUpgrade: func() (header ws.HandshakeHeader, err error) {
			if id == 0 {
				return nil, ErrNotAuth
			}
			return
		},
	}
	if _, err := u.Upgrade(conn); err == nil {
		w.mutex.Lock()
		if existConn, ok := w.conns[id]; ok {
			if existConn != conn {
				err := existConn.Close()
				if err != nil {
					w.l.Print("Close connection err:", err)
				}
			}
		}
		w.conns[id] = conn
		w.mutex.Unlock()

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go w.onOnlineWrapper(id, wg)

		chMsg := make(chan Message)
		afterPing := false
		to := time.NewTimer(TimeoutPing)

	ReadLoop:
		for {
			go readMessage(conn, chMsg)
			select {
			case msg := <-chMsg:
				if !to.Stop() {
					<-to.C
				}
				if msg.Err == nil {
					switch msg.Op {
					case ws.OpPing:
					case ws.OpPong:
					case ws.OpText:
						go w.onTextWrapper(id, msg.Body)
					case ws.OpClose:
						break ReadLoop
					default:
						w.l.Printf("[%d] Unknown received, OpCode: %v\n", id, msg.Op)
					}
					afterPing = false
					to.Reset(TimeoutPing)
				} else {
					w.l.Printf("[%d] read error: %s\n", id, msg.Err)
					break ReadLoop //EOF
				}
			case <-to.C:
				if !afterPing {
					go wsutil.WriteServerMessage(conn, ws.OpPing, []byte{})
					afterPing = true
					to.Reset(TimeoutClose)
				} else {
					w.l.Printf("[%d] Ping timeout...\n", id)
					wsutil.WriteServerMessage(conn, ws.OpClose, []byte{0x03, 0xEA})
					break ReadLoop
				}
			}
		}
		if w.conns[id] == conn {
			w.mutex.Lock()
			delete(w.conns, id)
			w.mutex.Unlock()

			wg.Wait()
			go w.onOfflineWrapper(id)
		}
	} else {
		w.l.Printf("%s: upgrade error: %v", nameConn(conn), err)
	}
}

func readMessage(rw io.ReadWriter, chMsg chan Message) {
	s := ws.StateServerSide
	ch := wsutil.ControlFrameHandler(rw, s)

	rd := wsutil.Reader{
		Source:         rw,
		State:          s,
		CheckUTF8:      true,
		OnIntermediate: ch,
	}

	hdr, err := rd.NextFrame()
	if err != nil {
		chMsg <- Message{Err: err}
		return
	}
	if hdr.OpCode.IsControl() {
		if err := ch(hdr, &rd); err != nil {
			chMsg <- Message{Err: err}
			return
		}
		chMsg <- Message{Op: hdr.OpCode}
		return
	}

	bts, err := ioutil.ReadAll(&rd)

	chMsg <- Message{
		Body: bts,
		Op:   hdr.OpCode,
		Err:  err,
	}
	return
}

func (w *WS) WriteMessage(id uint, msg []byte) error {
	if w.onSendWrapper(id, msg) {
		w.mutex.RLock()
		defer w.mutex.RUnlock()
		if conn, ok := w.conns[id]; ok {
			err := wsutil.WriteServerMessage(conn, ws.OpText, msg)
			if err != nil {
				w.l.Printf("[%d] Write error: %s\n", id, err)
			}
			return err
		}
		w.l.Printf("Connection not found for device: %d\n", id)
		return ErrConnNotFound
	}
	return nil
}

func (w *WS) CloseConnection(id uint) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if conn, ok := w.conns[id]; ok {
		wsutil.WriteServerMessage(conn, ws.OpClose, []byte{0x03, 0xEA})
		return conn.Close()
	}
	w.l.Printf("Connection not found for device: %d\n", id)
	return ErrConnNotFound
}

func (w *WS) onAuthWrapper(token string) (id uint, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
			w.l.Printf("[Recovery OnAuth] panic recovered:\n%s\n\n", r)
		}
	}()
	return w.h.OnAuth(token)
}

func (w *WS) onOnlineWrapper(id uint, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			w.l.Printf("[Recovery OnOnline] panic recovered:\n%s\n\n", r)
		}
	}()
	w.h.OnOnline(id)
}

func (w *WS) onTextWrapper(id uint, msg []byte) {
	defer func() {
		if r := recover(); r != nil {
			w.l.Printf("[Recovery OnText] panic recovered:\n%s\n\n", r)
		}
	}()
	w.h.OnText(id, msg)
}

func (w *WS) onSendWrapper(id uint, msg []byte) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			w.l.Printf("[Recovery OnWriteText] panic recovered:\n%s\n\n", r)
		}
	}()
	return w.h.OnSend(id, msg)
}

func (w *WS) onOfflineWrapper(id uint) {
	defer func() {
		if r := recover(); r != nil {
			w.l.Printf("[Recovery OnOffline] panic recovered:\n%s\n\n", r)
		}
	}()
	w.h.OnOffline(id)
}

func nameConn(conn net.Conn) string {
	return conn.LocalAddr().String() + " > " + conn.RemoteAddr().String()
}
