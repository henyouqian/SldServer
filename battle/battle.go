// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
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
}

func makeBattle() *Battle {
	battle := new(Battle)
	battle.state = DOWNLOADING
	battle.readyConn = nil
	return battle
}

func battleReady(conn *Connection) {
	battle := conn.battle
	if battle.state != DOWNLOADING {
		conn.sendErr("error state")
		return
	}
	if battle.readyConn == nil {
		conn.battle.readyConn = conn
		conn.sendOk("battle ready")
	} else if conn.battle.readyConn == conn.foe {
		conn.battle.state = MATCHING
		msg := []byte(`{"Type":"battleStart"}`)
		conn.send <- msg
		conn.foe.send <- msg
	}
}

func test(conn *Connection) {
	str := fmt.Sprintf("%s: test send", conn.nickName)
	conn.send <- []byte(str)
	str = fmt.Sprintf("%s: test recv", conn.foe.nickName)
	conn.foe.send <- []byte(str)
}

func regBattle() {
	regHandler("test", MsgHandler(test))
	regHandler("battleReady", MsgHandler(battleReady))
}
