package main

import (
	"github.com/henyouqian/lwutil"
	"net/http"
)

func apiBetHelp(w http.ResponseWriter, r *http.Request) {
	lwutil.CheckMathod(r, "POST")

	//session
	_, err := findSession(w, r, nil)
	lwutil.CheckError(err, "err_auth")

	//
	text := `• 只有投中第一名的队伍才能获得奖金。

• 获得奖金到数量为投注金额乘以赔率。

• 赔率根据每支队伍投注数量动态改变，投注越少到队伍赔率越高。

• 奖金以投注结束时的赔率来计算。

• 队伍积分计算方法为个人前一百名的积分相加。第一名150分，第二名99分，第三名98分，第四名97分...依此类推...第一百名1分。
`

	//out
	out := struct {
		Text string
	}{
		text,
	}

	lwutil.WriteResponse(w, out)
}

func regEtc() {
	http.Handle("/etc/betHelp", lwutil.ReqHandler(apiBetHelp))
	// http.Handle("/etc/getAppConf", lwutil.ReqHandler(apiGetAppConf))
}
