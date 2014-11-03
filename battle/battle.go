// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"github.com/golang/glog"
)

func _battlelog() {
	glog.Info("")
}

type BattleState int

const (
	DOWNLOADING BattleState = iota
	MATCHING
)

type Battle struct {
	state     BattleState
	readyConn *Connection
	results   map[int64]int64 //[userId]msec
}

func makeBattle() *Battle {
	battle := new(Battle)
	battle.state = DOWNLOADING
	battle.readyConn = nil
	battle.results = make(map[int64]int64)
	return battle
}

func battleReady(conn *Connection, msg []byte) {
	battle := conn.battle
	if battle.state != DOWNLOADING {
		conn.sendErr("error state")
		return
	}
	if battle.readyConn == nil {
		battle.readyConn = conn
		conn.sendType("ready")
	} else if battle.readyConn == conn.foe {
		battle.state = MATCHING
		msg := []byte(`{"Type":"start"}`)
		conn.send <- msg
		conn.foe.send <- msg
	}
}

func battleProgress(conn *Connection, msg []byte) {
	if conn.battle == nil {
		conn.sendErr("need pair")
		return
	}
	if conn.battle.state != MATCHING {
		conn.sendErr("battle state error")
		return
	}

	var in struct {
		CompleteNum int
	}

	err := json.Unmarshal(msg, &in)
	if err != nil {
		conn.sendErr("json error")
		return
	}

	if in.CompleteNum <= 0 {
		conn.sendErr("in.CompleteNum")
		return
	}

	conn.foe.send <- msg
}

func battleEnd(conn *Connection, msg []byte) {
	if conn.battle == nil {
		conn.sendErr("need pair")
		return
	}
	if conn.battle.state != MATCHING {
		conn.sendErr("battle state error")
		return
	}
}

func regBattle() {
	regHandler("ready", MsgHandler(battleReady))
	regHandler("progress", MsgHandler(battleProgress))
	regHandler("end", MsgHandler(battleEnd))
}
