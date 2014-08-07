package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	CLIENT_VERSION = "1.0"
	APP_STORE_URL  = "itms-apps://ax.itunes.apple.com/WebObjects/MZStore.woa/wa/viewContentsUserReviews?type=Purple+Software&id=904649492"
	BACKUP_BUCKET  = "pintubackup"
)

type Conf struct {
	Port          int
	RedisHost     string
	SsdbAuthPort  int
	SsdbMatchPort int

	AppName            string
	EventPublishInfoes []EventPublishInfo
	ChallengeRewards   []int
}

var (
	_conf       Conf
	_clientConf = map[string]string{
		"DataHost":       "http://dn-pintugame.qbox.me",
		"StoreId":        "904649492",
		"Html5Url":       "http://pintuhtml5.qiniudn.com/index.html",
		"FlurryKey":      "2P9DTVNTFZS8YBZ36QBZ",
		"MogoKey":        "8c0728f759464dcda07c81afb00d3bf5",
		"UmengSocialKey": "53aeb00356240bdcb8050c26",
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
}
