package jsonrpc2ws

import (
	"github.com/powerman/rpc-codec/jsonrpc2"
	"github.com/gorilla/websocket"
	"encoding/json"
	"io"
	"sync"
)

func NewClient(ws *websocket.Conn) *jsonrpc2.Client {
	c := &client{
		ws:ws,
	}
	c.initRead()
	c.initWrite()
	return jsonrpc2.NewClient(c)
}

type client struct {
	w *io.PipeWriter
	r *io.PipeReader
	ws *websocket.Conn

	err error	   // last io error
	mu sync.Mutex  // protects err
}


func (x *client) error() (err error) {
	x.mu.Lock()
	err = x.err
	x.mu.Unlock()
	return
}

func (x *client) setError (err error) {
	x.mu.Lock()
	if x.err != nil {
		x.err = err
	}
	x.mu.Unlock()
}

func (x *client) initRead(){
	var w *io.PipeWriter
	x.r, w = io.Pipe()
	go func(){
		_,b,err := x.ws.ReadMessage()
		for  ; x.error() == nil && err == nil; _,b,err = x.ws.ReadMessage(){
			_,err = w.Write(b)
		}
		x.setError(err)
		w.Close()
	} ()
}

func (x *client) initWrite(){
	var r *io.PipeReader
	r,x.w = io.Pipe()
	go func(){
		dec := json.NewDecoder(r)
		var b json.RawMessage
		err := dec.Decode(&b)
		for  ; x.error() == nil &&  err == nil; err = dec.Decode(&b){
			err = x.ws.WriteMessage(websocket.TextMessage, b)
		}
		x.setError(err)
		r.Close()
	} ()
}

func (x *client) Write(p []byte) (int, error){
	if x.error() != nil {
		return 0, x.error()
	}
	return x.w.Write(p)
}

func (x *client) Read(p []byte) (int, error){
	if x.error() != nil {
		return 0, x.error()
	}
	return x.r.Read(p)
}

func (x *client) Close() error{
	x.ws.Close()
	x.w.Close()
	x.r.Close()
	return nil
}