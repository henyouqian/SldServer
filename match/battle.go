package main

import (
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
)

func battleGlog() {
	glog.Info("")
}

type BattleRoom struct {
	Name       string
	RewardCoin int
	EnterCoin  int
}

var (
	BATTLE_ROOM_LIST = []BattleRoom{
		{"free", 0, 0},
		{"coin1", 1, 2},
		{"coin2", 2, 4},
		{"coin5", 5, 10},
		{"coin10", 10, 20},
		{"coin20", 20, 40},
	}
)

func apiBattleRoomList(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//session
	_, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//out
	lwutil.WriteResponse(w, BATTLE_ROOM_LIST)
}

func regBattle() {
	http.Handle("/battle/roomList", lwutil.ReqHandler(apiBattleRoomList))
}
