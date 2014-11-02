// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang/glog"
	"github.com/gorilla/websocket"
)

func _connlog() {
	glog.Info("")
}

type MsgHandler func(conn *Connection, msg []byte)

var (
	_msgHandlerMap map[string]MsgHandler
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  128,
	WriteBufferSize: 128,
}

// connection is an middleman between the websocket connection and the hub.
type Connection struct {
	// The websocket connection.
	ws *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte

	foe    *Connection
	battle *Battle

	//player info
	nickName string
}

func init() {
	_msgHandlerMap = make(map[string]MsgHandler)
}

func regHandler(msgType string, handler MsgHandler) {
	_msgHandlerMap[msgType] = handler
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Connection) readPump() {
	defer func() {
		h.unregister <- c
		c.ws.Close()
	}()
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error { c.ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, message, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		msg := struct {
			Type string
		}{}

		err = json.Unmarshal(message, &msg)
		if err != nil || msg.Type == "" {
			break
		}

		if msg.Type == "pair" {
			if h.pair(c, message) != nil {
				break
			}
		} else {
			handler, e := _msgHandlerMap[msg.Type]
			if e {
				handler(c, message)
			} else {
				break
			}
		}

		// if c.foe != nil {
		// 	c.foe.send <- message
		// }

		// h.broadcast <- message
	}
}

// write writes a message with the given message type and payload.
func (c *Connection) write(mt int, payload []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(mt, payload)
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.write(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func (c *Connection) sendErr(str string) {
	msg := fmt.Sprintf(`{"Type":"err", "String":%s}`, str)
	c.send <- []byte(msg)
}

func (c *Connection) sendMsg(msg interface{}) {
	js, _ := json.Marshal(msg)
	c.send <- js
}

func (c *Connection) sendType(tp string) {
	js := fmt.Sprintf(`{"Type":"%s"}`, tp)
	c.send <- []byte(js)
}

// serverWs handles websocket requests from the peer.
func serveWs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	c := &Connection{send: make(chan []byte, 256), ws: ws}
	h.register <- c
	go c.writePump()
	c.readPump()
}
