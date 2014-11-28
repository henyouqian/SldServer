package main

import (
	"encoding/base64"
	"fmt"

	"github.com/nu7hatch/gouuid"
)

const (
	H_PLAYER_INFO = "H_PLAYER_INFO" //key:H_PLAYER_INFO/<userId> subkey:property

	PLAYER_GOLD_COIN = "GoldCoin"
)

func genUUID() string {
	uuid, _ := uuid.NewV4()
	return base64.URLEncoding.EncodeToString(uuid[:])
}

func makePlayerInfoKey(userId int64) string {
	return fmt.Sprintf("%s/%d", H_PLAYER_INFO, userId)
}
