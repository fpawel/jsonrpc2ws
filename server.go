package jsonrpc2ws

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/powerman/rpc-codec/jsonrpc2"
	"io"
	"io/ioutil"
	"net/http"
	"net/rpc"
	"time"
)

func DefaultUpgrader() websocket.Upgrader{
	return websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
}

func Serve(conn *websocket.Conn) {
	wsHandleRequest(conn)
	wsClose(conn)
}

func wsHandleRequest(ws *websocket.Conn) {
	for {
		_, req, err := ws.ReadMessage()
		if err != nil {
			return
		}
		var res bytes.Buffer

		err = rpc.ServeRequest(jsonrpc2.NewServerCodec(struct {
			io.ReadCloser
			io.Writer
		}{
			ioutil.NopCloser(bytes.NewReader(req)),
			&res,
		}, nil))

		var b []byte
		switch err.(type) {
		case nil:
			b = res.Bytes()
		case *jsonrpc2.Error:
			var v clientResponse
			v.Error = err.(*jsonrpc2.Error)
			v.Version = "2.0"
			if b,err = json.Marshal(v); err != nil {
				panic(err)
			}
		default:
			return
		}
		if err = ws.WriteMessage(websocket.TextMessage, b); err != nil {
			return
		}
	}
}

func wsClose(ws *websocket.Conn) error {
	const deadline = time.Second
	return ws.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(deadline))
}