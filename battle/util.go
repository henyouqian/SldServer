package main

import (
	"encoding/base64"

	"github.com/nu7hatch/gouuid"
)

func genUUID() string {
	uuid, _ := uuid.NewV4()
	return base64.URLEncoding.EncodeToString(uuid[:])
}
