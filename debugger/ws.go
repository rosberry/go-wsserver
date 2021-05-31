package debugger

import (
	"github.com/rosberry/go-wsserver"
)

type (
	Debugger struct {
		cc        wsserver.ConnController
		ws        extHandlers
		debuggers map[uint]bool
	}

	Config struct {
		Addr     string
		Handlers *wsserver.Handlers
	}

	extHandlers struct {
		cc          wsserver.ConnController
		appHandlers wsserver.Handlers
		devices     map[uint]bool
		d           *Debugger
	}
)

func New(cfg *Config) (*Debugger, error) {
	if cfg == nil {
		return nil, wsserver.ErrEmptyConfig
	}

	d := &Debugger{
		ws: extHandlers{
			appHandlers: *cfg.Handlers,
			devices:     make(map[uint]bool),
		},
		debuggers: make(map[uint]bool),
	}
	d.ws.d = d
	*cfg.Handlers = &d.ws

	if _, err := wsserver.Start(
		&wsserver.Config{
			Addr:     cfg.Addr,
			Handlers: d,
		}); err != nil {

		return nil, err
	}

	return d, nil
}

func (d *Debugger) SetConnCtrlr(ctrlr wsserver.ConnController) {
	d.cc = ctrlr
}

func (d *Debugger) OnAuth(token string) (id uint, ok bool) {
	return d.ws.appHandlers.OnAuth(token)
}

func (d *Debugger) OnOnline(id uint) {
	if _, ok := d.debuggers[id]; !ok {
		d.debuggers[id] = true
		if _, ok := d.ws.devices[id]; !ok {
			d.ws.appHandlers.OnOnline(id)
		}
	}
}

func (d *Debugger) OnText(id uint, msg []byte) {
	if _, ok := d.debuggers[id]; ok {
		go d.cc.WriteMessage(id, msg)
	}
	if _, ok := d.ws.devices[id]; ok {
		go d.ws.cc.WriteMessage(id, msg)
	}
	d.ws.appHandlers.OnText(id, msg)
}

func (d *Debugger) OnSend(id uint, msg []byte) (ok bool) {
	return true
}

func (d *Debugger) OnOffline(id uint) {
	if _, ok := d.debuggers[id]; ok {
		delete(d.debuggers, id)
		if _, ok := d.ws.devices[id]; !ok {
			d.ws.appHandlers.OnOffline(id)
		}
	}
}

func (e *extHandlers) SetConnCtrlr(ctrlr wsserver.ConnController) {
	e.cc = ctrlr
	e.appHandlers.SetConnCtrlr(e)
}

func (e *extHandlers) OnAuth(token string) (id uint, ok bool) {
	return e.appHandlers.OnAuth(token)
}

func (e *extHandlers) OnOnline(id uint) {
	if _, ok := e.devices[id]; !ok {
		e.devices[id] = true
		if _, ok := e.d.debuggers[id]; !ok {
			e.appHandlers.OnOnline(id)
		}
	}
}

func (e *extHandlers) OnText(id uint, msg []byte) {
	if _, ok := e.d.debuggers[id]; ok {
		go e.d.cc.WriteMessage(id, msg)
	}
	e.appHandlers.OnText(id, msg)
}

func (e *extHandlers) OnSend(id uint, msg []byte) (ok bool) {
	return true
}

func (e *extHandlers) OnOffline(id uint) {
	if _, ok := e.devices[id]; ok {
		delete(e.devices, id)
		if _, ok := e.d.debuggers[id]; !ok {
			e.appHandlers.OnOffline(id)
		}
	}
}

func (e *extHandlers) WriteMessage(id uint, msg []byte) (err error) {
	if _, ok := e.d.debuggers[id]; ok {
		go e.d.cc.WriteMessage(id, msg)
	}
	if _, ok := e.devices[id]; ok {
		go e.cc.WriteMessage(id, msg)
	}
	e.appHandlers.OnSend(id, msg)
	return nil
}

func (e *extHandlers) CloseConnection(id uint) (err error) {
	if _, ok := e.d.debuggers[id]; ok {
		go e.d.cc.CloseConnection(id)
	}
	if _, ok := e.devices[id]; ok {
		go e.cc.CloseConnection(id)
	}
	return nil
}
