package main

import (
	// "encoding/json"
	"github.com/golang/glog"
	"github.com/henyouqian/lwutil"
	"net/http"
)

func glogTumblr() {
	glog.Info("")
}

type TumblrImage struct {
	BlogName string
	PostId   int64
}

func apiTumblrAddImage(w http.ResponseWriter, r *http.Request) {
	var err error
	lwutil.CheckMathod(r, "POST")

	//in
	var in struct {
		UserName string
		PostId   int64
	}
	err = lwutil.DecodeRequestBody(r, &in)
	lwutil.CheckError(err, "err_decode_body")

	//ssdb
	ssdb, err := ssdbPool.Get()
	lwutil.CheckError(err, "")
	defer ssdb.Close()

}

func regTumblr() {
	http.Handle("/tumblr/addImage", lwutil.ReqHandler(apiTumblrAddImage))
}
