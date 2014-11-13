// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"

	"github.com/golang/glog"
	"github.com/henyouqian/ssdbgo"
)

func _hublog() {
	glog.Info("")
}

const (
	H_SESSION     = "H_SESSION"     //key:token, value:session
	H_PLAYER_INFO = "H_PLAYER_INFO" //key:H_PLAYER_INFO/<userId> subkey:property
	H_PACK        = "H_PACK"        //subkey:packId value:packJson
)

type Session struct {
	Userid int64
}

type PlayerInfo struct {
	UserId          int64
	NickName        string
	TeamName        string
	Gender          int
	CustomAvatarKey string
	GravatarKey     string
}

type Image struct {
	Key string
}

type Pack struct {
	Id        int64
	AuthorId  int64
	Title     string
	Text      string
	Thumb     string
	CoverBlur string
	Images    []Image
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

	pendingConnMap map[string]*Connection
	pendingConnMu  sync.Mutex
}

var h = Hub{
	broadcast:      make(chan []byte),
	register:       make(chan *Connection),
	unregister:     make(chan *Connection),
	connections:    make(map[*Connection]bool),
	pendingConnMap: make(map[string]*Connection),
}

func (h *Hub) run() {
	for {
		select {
		case c := <-h.register:
			h.connections[c] = true
			// c.sendType("connected")
		case c := <-h.unregister:
			if c.foe != nil {
				c.foe.sendType("foeDisconnect")
				c.foe.foe = nil
			}
			if _, ok := h.connections[c]; ok {
				delete(h.connections, c)
				close(c.send)
			}

			h.pendingConnMu.Lock()
			if c == h.pendingConnMap[c.roomName] {
				h.pendingConnMap[c.roomName] = nil
			}
			h.pendingConnMu.Unlock()

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

func (h *Hub) simplePair(c *Connection, msg []byte, roomName string) error {
	//
	in := struct {
		NickName string
	}{}
	if err := json.Unmarshal(msg, &in); err != nil {
		return err
	}

	h.pendingConnMu.Lock()

	pendingConn := h.pendingConnMap[roomName]
	if pendingConn != nil {
		if pendingConn == c {
			h.pendingConnMu.Unlock()
			return fmt.Errorf("same user")
		}

		h.pendingConnMap[roomName] = nil
		h.pendingConnMu.Unlock()

		c.foe = pendingConn
		pendingConn.foe = c

		battle := makeBattle()
		c.battle = battle
		c.foe.battle = battle

		c.index = 1
		c.foe.index = 0

		//
		matchdb, err := ssdbMatchPool.Get()
		if err != nil {
			return err
		}
		defer matchdb.Close()

		//get pack, fixme
		pack, err := getPack(matchdb, 2)
		if err != nil {
			return err
		}

		//
		out := struct {
			Type    string
			FoeName string
			Pack    *Pack
		}{
			"paired",
			c.foe.nickName,
			pack,
		}
		c.sendMsg(out)

		out.FoeName = c.nickName
		c.foe.sendMsg(out)
	} else {
		h.pendingConnMap[roomName] = c
		h.pendingConnMu.Unlock()

		c.sendType("pairing")
	}

	c.roomName = roomName

	return nil
}

func (h *Hub) authPair(c *Connection, msg []byte, roomName string) error {
	//ssdb
	authdb, err := ssdbAuthPool.Get()
	if err != nil {
		return err
	}
	defer authdb.Close()

	matchdb, err := ssdbMatchPool.Get()
	if err != nil {
		return err
	}
	defer matchdb.Close()

	//
	in := struct {
		Token string
	}{}
	if err = json.Unmarshal(msg, &in); err != nil {
		return err
	}

	//
	sessionKey := fmt.Sprintf("%s/%s", H_SESSION, in.Token)
	resp, err := authdb.Do("get", sessionKey)
	if err != nil {
		return err
	}
	if resp[0] != "ok" {
		return fmt.Errorf("ssdb err:%s", resp[0])
	}

	var session Session
	err = json.Unmarshal([]byte(resp[1]), &session)
	if err != nil {
		return err
	}

	//
	c.playerInfo, err = getPlayerInfo(matchdb, session.Userid)
	if err != nil {
		return err
	}
	c.playerInfo.UserId = session.Userid

	//
	h.pendingConnMu.Lock()
	pendingConn := h.pendingConnMap[roomName]
	if pendingConn != nil {
		h.pendingConnMap[roomName] = nil
		h.pendingConnMu.Unlock()

		//check userId
		if pendingConn.playerInfo.UserId == session.Userid {
			return fmt.Errorf("same user")
		}

		//
		c.foe = pendingConn
		pendingConn.foe = c

		battle := makeBattle()
		c.battle = battle
		c.foe.battle = battle

		c.index = 1
		c.foe.index = 0

		//get pack, fixme
		pack, err := getPack(matchdb, 2)
		if err != nil {
			return err
		}

		//
		out := struct {
			Type      string
			FoeName   string
			Pack      *Pack
			SliderNum int
		}{
			"paired",
			c.foe.playerInfo.NickName,
			pack,
			rand.Intn(3) + 4,
		}
		c.sendMsg(out)

		out.FoeName = c.playerInfo.NickName
		c.foe.sendMsg(out)
	} else {
		h.pendingConnMap[roomName] = c
		h.pendingConnMu.Unlock()

		c.sendType("pairing")
	}

	c.roomName = roomName

	return nil
}

func makePlayerInfoKey(userId int64) string {
	return fmt.Sprintf("%s/%d", H_PLAYER_INFO, userId)
}

func getPlayerInfo(ssdbc *ssdbgo.Client, userId int64) (*PlayerInfo, error) {
	key := makePlayerInfoKey(userId)

	var playerInfo PlayerInfo
	err := ssdbc.HGetStruct(key, &playerInfo)
	if err != nil {
		return nil, err
	}

	return &playerInfo, nil
}

func getPack(ssdbc *ssdbgo.Client, packId int64) (*Pack, error) {
	var pack Pack
	resp, err := ssdbc.Do("hget", H_PACK, packId)
	if err != nil {
		return nil, err
	}
	if resp[0] == ssdbgo.NOT_FOUND {
		return nil, fmt.Errorf("not_found:packId=%d", packId)
	}

	err = json.Unmarshal([]byte(resp[1]), &pack)
	if err != nil {
		return nil, err
	}

	return &pack, err
}
