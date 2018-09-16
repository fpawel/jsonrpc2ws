// An implementation of rpc.CleintCodec over gorilla/webscket.
// Based on github.com/powerman/rpc-codec/jsonrpc2.
// Use of this source code is governed by a BSD-style.
// license that can be found in https://github.com/powerman/rpc-codec/blob/master/LICENSE

package jsonrpc2ws

import (
	"encoding/json"
	"sync"
	"github.com/powerman/rpc-codec/jsonrpc2"
	"github.com/gorilla/websocket"
	"net/rpc"
	"reflect"
	"math"
	"io"
)

const errInternalCode = -32603

// NewClientCodec returns a new rpc.ClientCodec using JSON-RPC 2.0 on ws.
func NewClientWithCodec(ws *websocket.Conn) *jsonrpc2.Client {
	return jsonrpc2.NewClientWithCodec(&clientCodec{
		ws:ws,
		pending: make(map[uint64]string),
	})
}

type clientRequest struct {
	Version string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      *uint64     `json:"id,omitempty"`
}

type clientCodec struct {
	ws             *websocket.Conn
	mutexWebSocket sync.Mutex // protects ws

	// temporary work space
	resp clientResponse

	// JSON-RPC responses include the request id but not the request method.
	// Package rpc expects both.
	// We save the request method in pending when sending a request
	// and then look it up by request ID when filling out the rpc Response.
	mutexPending sync.Mutex        // protects pending
	pending      map[uint64]string // map request id to method name
}

type clientResponse struct {
	Version string           `json:"jsonrpc"`
	ID      *uint64          `json:"id"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpc2.Error  `json:"error,omitempty"`
}


func (c *clientCodec) ReadResponseHeader(r *rpc.Response) error {

	err := c.ws.ReadJSON(&c.resp)
	if err == io.EOF {
		return err
	}

	if c.resp.ID == nil {
		return c.resp.Error
	}

	c.mutexPending.Lock()
	r.ServiceMethod = c.pending[*c.resp.ID]
	delete(c.pending, *c.resp.ID)
	c.mutexPending.Unlock()

	r.Error = ""
	r.Seq = *c.resp.ID
	if c.resp.Error != nil {
		r.Error = c.resp.Error.Error()
	}
	return nil
}

func (c *clientCodec) ReadResponseBody(x interface{}) error {
	// If x!=nil and return error e:
	// - this call get e.Error() appended to "reading body "
	// - other pending calls get error as is XXX actually other calls
	//   shouldn't be affected by this error at all, so let's at least
	//   provide different error message for other calls
	if x == nil {
		return nil
	}
	if err := json.Unmarshal(*c.resp.Result, x); err != nil {
		e := jsonrpc2.NewError(errInternalCode, err.Error())
		e.Data = jsonrpc2.NewError(errInternalCode, "some other Call failed to unmarshal Reply")
		return e
	}
	return nil
}

func (c *clientCodec) Close() error {
	return c.ws.Close()
}

const seqNotify = math.MaxUint64

func (c *clientCodec) WriteRequest(r *rpc.Request, param interface{}) error {
	// If return error: it will be returned as is for this call.
	// Allow param to be only Array, Slice, Map or Struct.
	// When param is nil or uninitialized Map or Slice - omit "params".
	if param != nil {
		switch k := reflect.TypeOf(param).Kind(); k {
		case reflect.Map:
			if reflect.TypeOf(param).Key().Kind() == reflect.String {
				if reflect.ValueOf(param).IsNil() {
					param = nil
				}
			}
		case reflect.Slice:
			if reflect.ValueOf(param).IsNil() {
				param = nil
			}
		case reflect.Array, reflect.Struct:
		case reflect.Ptr:
			switch k := reflect.TypeOf(param).Elem().Kind(); k {
			case reflect.Map:
				if reflect.TypeOf(param).Elem().Key().Kind() == reflect.String {
					if reflect.ValueOf(param).Elem().IsNil() {
						param = nil
					}
				}
			case reflect.Slice:
				if reflect.ValueOf(param).Elem().IsNil() {
					param = nil
				}
			case reflect.Array, reflect.Struct:
			default:
				return jsonrpc2.NewError(errInternalCode, "unsupported param type: Ptr to "+k.String())
			}
		default:
			return jsonrpc2.NewError(errInternalCode, "unsupported param type: "+k.String())
		}
	}

	var req clientRequest
	if r.Seq != seqNotify {
		c.mutexPending.Lock()
		c.pending[r.Seq] = r.ServiceMethod
		c.mutexPending.Unlock()
		req.ID = &r.Seq
	}
	req.Version = "2.0"
	req.Method = r.ServiceMethod
	req.Params = param

	c.mutexWebSocket.Lock()
	err := c.ws.WriteJSON(&req)
	c.mutexWebSocket.Unlock()

	return err
}

