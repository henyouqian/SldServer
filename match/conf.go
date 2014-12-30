package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	CLIENT_VERSION = "1.0"
	//APP_STORE_URL  = "itms-apps://ax.itunes.apple.com/WebObjects/MZStore.woa/wa/viewContentsUserReviews?type=Purple+Software&id=904649492"
	// APP_STORE_URL = "https://itunes.apple.com/cn/app/pin-pin-pin-pin-pin/id904649492?l=zh&ls=1&mt=8"
	APP_STORE_URL              = "https://itunes.apple.com/cn/app/man-pin-de/id923531990?l=zh&ls=1&mt=8"
	BACKUP_BUCKET              = "pintubackup"
	QINIU_KEY                  = "zkDlO9eqB8oyXqv7CLhy5qmzjkirdwZZjpNjCkbm"
	QINIU_SEC                  = "E-ptuzmtK5gzuMmeLttndkNxqIhnhJFfXOGo7HD-"
	QINIU_PRIVATE_DOMAIN       = "7tebsf.com1.z0.glb.clouddn.com"
	USER_UPLOAD_BUCKET         = "pintuuserupload"
	USER_PRIVATE_UPLOAD_BUCKET = "pintuprivate"
)

type Conf struct {
	Port          int
	RedisHost     string
	SsdbAuthPort  int
	SsdbMatchPort int

	AppName string
	// EventPublishInfoes    []EventPublishInfo
	// PickSidePublishInfoes []EventPublishInfo
	ChallengeRewards []int
}

var (
	_conf       Conf
	_clientConf = map[string]string{
		"DataHost":          "http://dn-pintugame.qbox.me",
		"UploadHost":        "http://dn-pintuuserupload.qbox.me",
		"PrivateUploadHost": "http://7tebsf.com1.z0.glb.clouddn.com",
		"StoreId":           "923531990",
		"Html5Url":          "http://pintuhtml5.qiniudn.com/index.html",
		"FlurryKey":         "2P9DTVNTFZS8YBZ36QBZ",
		"MogoKey":           "8c0728f759464dcda07c81afb00d3bf5",
		"UmengSocialKey":    "53aeb00356240bdcb8050c26",
		"WebSocketUrl":      "ws://173.255.215.104:9977/ws",
	}
)

func initConf(confFile string) {
	var f *os.File
	var err error

	if f, err = os.Open(confFile); err != nil {
		panic(fmt.Sprintf("config file not found: %s", confFile))
	}
	defer f.Close()

	//json decode
	decoder := json.NewDecoder(f)
	err = decoder.Decode(&_conf)
	if err != nil {
		panic(err)
	}

	if !isReleaseServer() {
		delete(_clientConf, "WebSocketUrl")
	}
}
