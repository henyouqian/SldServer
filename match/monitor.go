package main

import (
	// "encoding/json"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	// "math"
	"net/http"
	// "time"
)

func monitorGlog() {
	glog.Info("")
}

func apiGoldCoinDailyPurchase(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")
	var err error

	//ssdb
	ssdbc, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdbc.Close()

	//in
	var in struct {
		Date string
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")
}

func regMonitor() {
	http.Handle("/monitor/goldCoinDailyPurchase", lwutil.ReqHandler(apiGoldCoinDailyPurchase))
}
