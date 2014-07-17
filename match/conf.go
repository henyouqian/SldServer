package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type Conf struct {
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

	//init Z_EVENT AND Q_EVENT_BUFF
	Z_EVENT = fmt.Sprintf("Z_EVENT/%s", _conf.AppName)
	Z_EVENT_BUFF = fmt.Sprintf("Z_EVENT_BUFF/%s", _conf.AppName)

	//
	EventPublishInfoes = _conf.EventPublishInfoes
}
