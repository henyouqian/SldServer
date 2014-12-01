// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	// "math/rand"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/henyouqian/ssdbgo"
)

func _hublog() {
	glog.Info("")
}

const (
	H_SESSION = "H_SESSION" //key:token, value:session
	H_PACK    = "H_PACK"    //subkey:packId value:packJson
)

type Session struct {
	Userid int64
}

type PlayerInfo struct {
	UserId              int64
	NickName            string
	TeamName            string
	Gender              int
	CustomAvatarKey     string
	GravatarKey         string
	GoldCoin            int
	BattlePoint         int
	BattleWinStreak     int
	BattleWinStreakMax  int
	BattleHeartZeroTime int64
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
			if c.battle != nil && c.battle.state == MATCHING {
				makeBattleResult(c, true)
			}

			if c.foe != nil {
				c.foe.sendType("foeDisconnect")
				c.foe.foe = nil
				c.battle.state = ONELEFT
			}
			c.foe = nil
			c.battle = nil

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

func (h *Hub) authPair(c *Connection, msg []byte) error {
	//check already pair
	if c.playerInfo != nil {
		if !(c.battle != nil && c.battle.state == ONELEFT) {
			return fmt.Errorf("err_already_pair")
		}
	}

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
		Token    string
		RoomName string
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
	room, exist := BATTLE_ROOM_MAP[in.RoomName]
	if !exist {
		c.playerInfo = nil
		return fmt.Errorf("err_room_name")
	}

	//check heart
	if room.BetCoin == 0 {
		heartNum := getBattleHeartNum(c.playerInfo)
		if heartNum == 0 {
			return fmt.Errorf("err_heart")
		}
	}

	//check coin
	if c.playerInfo.GoldCoin < room.BetCoin {
		glog.Info(c.playerInfo.UserId)
		c.playerInfo = nil
		return fmt.Errorf("err_coin")
	}

	//
	h.pendingConnMu.Lock()
	pendingConn := h.pendingConnMap[in.RoomName]
	if pendingConn != nil {
		//check userId
		if pendingConn.playerInfo.UserId == session.Userid {
			return fmt.Errorf("err_same_user")
		}

		h.pendingConnMap[in.RoomName] = nil
		h.pendingConnMu.Unlock()

		//
		c.foe = pendingConn
		pendingConn.foe = c

		battle := makeBattle()
		c.battle = battle
		c.foe.battle = battle
		battle.room = room

		//get pack, fixme
		pack, err := getPack(matchdb, 2)
		if err != nil {
			return err
		}

		//heart
		if room.BetCoin == 0 {
			heartNum := getBattleHeartNum(c.playerInfo)
			if heartNum == BATTLE_HEART_TOTAL {
				c.playerInfo.BattleHeartZeroTime = time.Now().Unix() - BATTLE_HEART_ADD_SEC*(BATTLE_HEART_TOTAL-1)
			} else {
				c.playerInfo.BattleHeartZeroTime += BATTLE_HEART_ADD_SEC
			}
			playerKey := makePlayerInfoKey(c.playerInfo.UserId)
			matchdb.Do("hset", playerKey, PLAYER_BATTLE_HEART_ZERO_TIME, c.playerInfo.BattleHeartZeroTime)
		}

		//
		out := struct {
			Type          string
			Pack          *Pack
			SliderNum     int
			FoePlayer     *PlayerInfo
			Secret        string
			HeartZeroTime int64
		}{
			"paired",
			pack,
			3, //rand.Intn(3) + 4, //fixme
			c.foe.playerInfo,
			battle.secret,
			c.playerInfo.BattleHeartZeroTime,
		}
		c.sendMsg(out)

		//foe
		out.FoePlayer = c.playerInfo

		//heart
		if room.BetCoin == 0 {
			heartNum := getBattleHeartNum(c.foe.playerInfo)
			if heartNum == BATTLE_HEART_TOTAL {
				c.foe.playerInfo.BattleHeartZeroTime = time.Now().Unix() - BATTLE_HEART_ADD_SEC*(BATTLE_HEART_TOTAL-1)
			} else {
				c.foe.playerInfo.BattleHeartZeroTime += BATTLE_HEART_ADD_SEC
			}
			playerKey := makePlayerInfoKey(c.foe.playerInfo.UserId)
			matchdb.Do("hset", playerKey, PLAYER_BATTLE_HEART_ZERO_TIME, c.foe.playerInfo.BattleHeartZeroTime)
		}

		out.HeartZeroTime = c.foe.playerInfo.BattleHeartZeroTime
		c.foe.sendMsg(out)

		//timeout
		go func() {
			time.Sleep(20 * time.Second)
			if c.battle != nil && c.battle.state == PREPARE {
				c.sendErr("err_timeout")
				c.foe.sendErr("err_timeout")
			}
		}()
	} else {
		h.pendingConnMap[in.RoomName] = c
		h.pendingConnMu.Unlock()

		c.sendType("pairing")
	}

	c.roomName = in.RoomName
	c.result = 0

	return nil
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
