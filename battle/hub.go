// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"

	"github.com/golang/glog"
)

func _hublog() {
	glog.Info("")
}

// hub maintains the set of active connections and broadcasts messages to the
// connections.
type Hub struct {
	// Registered connections.
	connections map[*Connection]bool

	// Inbound messages from the connections.
	broadcast chan []byte

	// Register requests from the connections.
	register chan *Connection

	// Unregister requests from connections.
	unregister chan *Connection

	pendingConn *Connection
}

var h = Hub{
	broadcast:   make(chan []byte),
	register:    make(chan *Connection),
	unregister:  make(chan *Connection),
	connections: make(map[*Connection]bool),
	pendingConn: nil,
}

func (h *Hub) run() {
	for {
		select {
		case c := <-h.register:
			h.connections[c] = true
			c.send <- []byte("register")
		case c := <-h.unregister:
			if c.foe != nil {
				c.foe.send <- []byte("foe disconnect")
				c.foe.foe = nil
			}
			if _, ok := h.connections[c]; ok {
				delete(h.connections, c)
				close(c.send)
			}
			if c == h.pendingConn {
				h.pendingConn = nil
			}
			// case m := <-h.broadcast:

			// 	for c := range h.connections {
			// 		select {
			// 		case c.send <- m:
			// 		default:
			// 			close(c.send)
			// 			delete(h.connections, c)
			// 		}
			// 	}
		}
	}
}

func (h *Hub) pair(c *Connection, msg []byte) error {
	if len(c.nickName) > 0 {
		return nil
	}

	glog.Info("pair")

	in := struct {
		NickName string
	}{}
	err := json.Unmarshal(msg, &in)
	if err != nil {
		return err
	}

	c.nickName = in.NickName

	//
	if h.pendingConn != nil {
		c.foe = h.pendingConn
		h.pendingConn.foe = c
		c.send <- []byte("paired")
		c.foe.send <- []byte("paired")
		h.pendingConn = nil

		battle := makeBattle()
		c.battle = battle
		c.foe.battle = battle
	} else {
		h.pendingConn = c
		c.send <- []byte("pairing...")
	}

	return nil
}
