package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	CLIENT_VERSION = "1.0"
	APP_STORE_URL  = "itms-apps://ax.itunes.apple.com/WebObjects/MZStore.woa/wa/viewContentsUserReviews?type=Purple+Software&id=904649492"
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
	_conf Conf
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
