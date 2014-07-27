package main

import (
	"flag"
	"fmt"
	"github.com/golang/glog"
	//"github.com/henyouqian/lwutil"
	qiniuconf "github.com/qiniu/api/conf"
	"net/http"
	"runtime"
)

func init() {
	qiniuconf.ACCESS_KEY = "XLlx3EjYfZJ-kYDAmNZhnH109oadlGjrGsb4plVy"
	qiniuconf.SECRET_KEY = "FQfB3pG4UCkQZ3G7Y9JW8az2BN1aDkIJ-7LKVwTJ"
}

func staticFile(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, r.URL.Path[1:])
}

func html5(w http.ResponseWriter, r *http.Request) {
	url := fmt.Sprintf("%s%s", "..", r.URL.Path)
	http.ServeFile(w, r, url)
	//lwutil.WriteResponse(w, url)
}

func main() {
	var confFile string
	flag.StringVar(&confFile, "conf", "", "config file")
	flag.Parse()

	if len(confFile) == 0 {
		glog.Errorln("need -conf")
		return
	}

	initConf(confFile)
	initDb()
	initEvent()

	http.HandleFunc("/www/", staticFile)
	http.HandleFunc("/html5/", html5)
	regAuth()
	regPack()
	regCollection()
	regPlayer()
	regMatch()
	regAdmin()
	regStore()
	regChallenge()
	regEtc()
	regUserPack()

	runtime.GOMAXPROCS(runtime.NumCPU())

	go scoreKeeperMain()

	glog.Infof("Server running: cpu=%d, port=%d", runtime.NumCPU(), _conf.Port)
	glog.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", _conf.Port), nil))
}
