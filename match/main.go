package main

import (
	"flag"
	"fmt"
	"github.com/golang/glog"
	//"github.com/henyouqian/lwutil"
	qiniuconf "github.com/qiniu/api/conf"
	"github.com/robfig/cron"
	"net/http"
	"runtime"
)

var (
	_cron cron.Cron
)

func init() {
	qiniuconf.ACCESS_KEY = QINIU_KEY
	qiniuconf.SECRET_KEY = QINIU_SEC
}

func staticFile(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, r.URL.Path[1:])
}

func html5(w http.ResponseWriter, r *http.Request) {
	url := fmt.Sprintf("%s%s", "../../delight/html5/slider/", r.URL.Path[7:])
	glog.Info(url)
	// glog.Info(r.URL.Path[1:])
	http.ServeFile(w, r, url)
}

// func rootTextFile(w http.ResponseWriter, r *http.Request) {
// 	http.ServeFile(w, r, "./root/"+r.URL.Path[1:])
// }

func main() {
	// var confFile string
	// flag.StringVar(&confFile, "conf", "", "config file")
	flag.Parse()

	// if len(confFile) == 0 {
	// 	glog.Errorln("need -conf")
	// 	return
	// }

	confFile := "conf.json"
	initConf(confFile)
	initDb()
	// initEvent()
	// initPickSide()
	initAdmin()
	initStore()

	if isReleaseServer() {
		_cron.AddFunc("0 19 3 * * *", backupTask)
	}

	_cron.Start()

	http.HandleFunc("/www/", staticFile)
	http.HandleFunc("/html5/", html5)
	// http.HandleFunc("/", rootTextFile)

	regAuth()
	regPack()
	regCollection()
	regPlayer()
	regMatch()
	regAdmin()
	regStore()
	regEtc()
	regSocial()
	regEcoMonitor()
	regBattle()
	regTumblr()
	regChannel()
	// regEvent()
	// regChallenge()
	// regUserPack()
	// regPickSide()

	runtime.GOMAXPROCS(runtime.NumCPU())
	glog.Infof("Server running: cpu=%d, port=%d", runtime.NumCPU(), _conf.Port)

	runMatchCron()
	// backupTask()

	glog.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", _conf.Port), nil))
}
