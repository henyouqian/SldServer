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
	MATCHED
	ONELEFT
)

type Battle struct {
	state     BattleState
	readyConn *Connection
	results   map[int]int //[index]msec
}

func makeBattle() *Battle {
	battle := new(Battle)
	battle.state = DOWNLOADING
	battle.readyConn = nil
	battle.results = make(map[int]int)
	return battle
}

func battleReady(conn *Connection, msg []byte) {
	battle := conn.battle
	if battle.state != DOWNLOADING {
		conn.sendErr("error state")
		return
	}
	if battle.readyConn == nil {
		battle.state = MATCHED
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

	//in
	var in struct {
		Msec int
	}

	err := json.Unmarshal(msg, &in)
	if err != nil {
		conn.sendErr("json error")
		return
	}

	results := conn.battle.results

	//check exist
	_, e := results[conn.index]
	if e {
		conn.sendErr("result exist")
		return
	}
	results[conn.index] = in.Msec

	//end
	if len(results) == 2 {
		myMsec := results[conn.index]
		foeMsec := results[conn.foe.index]

		out := struct {
			Result  string //win, lose, draw
			FoeMsec int
		}{
			"",
			foeMsec,
		}

		if myMsec < foeMsec { //win
			out.Result = "win"
		} else if myMsec > foeMsec { //lose
			out.Result = "lose"
		} else { //draw
			out.Result = "draw"
		}
		conn.sendMsg(out)

		//foe
		if myMsec < foeMsec {
			out.Result = "lose"
		} else if myMsec > foeMsec {
			out.Result = "win"
		}
		out.FoeMsec = myMsec
		conn.foe.sendMsg(out)
	} else {
		conn.sendType("end")
	}
}

func talk(conn *Connection, msg []byte) {
	var in struct {
		Text string
	}

	err := json.Unmarshal(msg, &in)
	if err != nil {
		conn.sendErr("json error")
		return
	}
	if len(in.Text) > 0 && conn.foe != nil {
		conn.foe.send <- msg
	}
}

func regBattle() {
	regHandler("ready", MsgHandler(battleReady))
	regHandler("progress", MsgHandler(battleProgress))
	regHandler("end", MsgHandler(battleEnd))
	regHandler("talk", MsgHandler(talk))
}
