function Controller($scope, $http) {
	$scope.apilists = [
		{
			"tab":"auth",
			"path":"auth",
			"apis":[
				{
					"name": "login",
					"method": "POST",
					"data": {"Username":"aa", "Password":"aa"}
				},{
					"name": "logout",
					"method": "POST",
					"data": ""
				},{
					"name": "info",
					"method": "POST",
					"data": ""
				},{
					"name": "register",
					"method": "POST",
					"data": {"Username":"?", "Password":"?"}
				},{
					"name": "forgotPassword",
					"method": "POST",
					"data": {"Email":""}
				},{
					"name": "checkVersion",
					"method": "POST",
					"data": {"Version":""}
				},{
					"name": "ssdbTest",
					"method": "POST",
					"data": ""
				},
			] 
		},{
			"tab":"pack",
			"path":"pack",
			"apis":[
				{
					"name": "new",
					"method": "POST",
					"data": {
						"Title":"",
						"Text":"",
						"Thumb":"thumbImage=.jpg",
						"Cover":"coverImage=.jpg",
						"CoverBlur":"coverBlurImage=.jpg",
						"Images":[
							{
								"File": "aaa.jpg",
								"Key": "qiniuImage=.jpg",
								"Title": "",
								"Text": ""
							}
						],
						"Tags":["art", "portrait"]
					}
				},{
					"name": "mod",
					"method": "POST",
					"data": {
						"Id": 0,
						"Title":"",
						"Text":"",
						"Thumb":"qiniuThumb=.jpg",
						"Cover":"qiniuImage=.jpg",
						"Images":[
							{
								"File": "aaa.jpg",
								"Key": "qiniuImage=.jpg",
								"Title": "",
								"Text": ""
							}
						],
						"Tags":["art", "portrait"]
					}
				},{
					"name": "del",
					"method": "POST",
					"data": {"Id": 0}
				},{
					"name": "list",
					"method": "POST",
					"data": {"UserId": 0, "StartId": 0, "Limit":12}
				},{
					"name": "get",
					"method": "POST",
					"data": {"Id": 1}
				},{
					"name": "listMatch",
					"method": "POST",
					"data": {"StartId": 0, "Limit":12}
				},{
					"name": "listByTag",
					"method": "POST",
					"data": {"Tag": "art", "StartId": 0, "Limit":12}
				},{
					"name": "addComment",
					"method": "POST",
					"data": {"PackId": 0, "Text":"好喜欢"}
				},{
					"name": "getComments",
					"method": "POST",
					"data": {"PackId": 0, "BottomCommentId": 0, "Limit": 20}
				}
			]
		},{
			"tab":"player",
			"path":"player",
			"apis":[
				{
					"name": "getInfo",
					"method": "POST",
					"data": ""
				},{
					"name": "setInfo",
					"method": "POST",
					"data": {"NickName":"", "Gender":0, "TeamName":"上海", "CustomAvatarKey":"", "GravatarKey":""}
				},{
					"name": "addPrizeFromCache",
					"method": "POST",
					"data": ""
				},{
					"name": "getUptoken",
					"method": "POST",
					"data": ""
				},{
					"name": "getUptokens",
					"method": "POST",
					"data": ["test1.jpg", "test2.jpg"]
				},{
					"name": "listMyPrize",
					"method": "POST",
					"data": {"StartId":0, "LastScore":0, "Limit":20}
				},{
					"name": "listMyReward",
					"method": "POST",
					"data": {"StartId":0, "Limit":20}
				}

			]
		},{
			"tab":"event",
			"path":"event",
			"apis":[
				{
					"name": "new",
					"method": "POST",
					"data": {
						"PackId": 1, 
						"SliderNum": 6,
						"BeginTimeString": "2014-04-01T00:00",				
						"EndTimeString": "2014-04-02T00:00",
						"ChallengeSecs": [32, 35, 40]
						}
				},{
					"name": "del",
					"method": "POST",
					"data": {"EventId": 0}
				},{
					"name": "mod",
					"method": "POST",
					"data": ""
				},{
					"name": "list",
					"method": "POST",
					"data": {"StartId": 0, "Limit": 20}
				},{
					"name": "get",
					"method": "POST",
					"data": {"EventId": 0}
				},{
					"name": "buffAdd",
					"method": "POST",
					"data": {
						"PackId": 0,
						"SliderNum": 5,
						"ChallengeSecs": [32, 35, 40]
						}
				},{
					"name": "buffList",
					"method": "POST",
					"data": ""
				},{
					"name": "buffDel",
					"method": "POST",
					"data": {"EventId": 0}
				},{
					"name": "buffMod",
					"method": "POST",
					"data": ""
				},{
					"name": "getUserPlay",
					"method": "POST",
					"data": {"EventId": 0, "UserId": 0}
				},{
					"name": "playBegin",
					"method": "POST",
					"data": {"EventId": 0}
				},{
					"name": "playEnd",
					"method": "POST",
					"data": {"EventId": 0, "Secret": "", "Score": 100, "Checksum":"cks"}
				},{
					"name": "getRanks",
					"method": "POST",
					"data": {"EventId": 0, "Offset": 0, "Limit": 20}
				},{
					"name": "getBettingPool",
					"method": "POST",
					"data": {"EventId": 0}
				},{
					"name": "bet",
					"method": "POST",
					"data": {"EventId": 0, "TeamName":"", "Money":100}
				},{
					"name": "listPlayResult",
					"method": "POST",
					"data": {"StartEventId": 0, "Limit":20}
				},{
					"name": "getPublish",
					"method": "POST",
					"data": ""
				},{
					"name": "setPublish",
					"method": "POST",
					"data": [{"PublishTime":[5, 0], "BeginTime":[5, 0], "EndTime":[13, 0], "EventNum":1}]
				}
			]
		},{
			"tab":"pickSide",
			"path":"pickSide",
			"apis":[
				{
					"name": "buffAdd",
					"method": "POST",
					"data": {
						"PackId": 0,
						"SliderNum": 5,
						"ChallengeSecs": [32, 35, 40]
						}
				},{
					"name": "buffList",
					"method": "POST",
					"data": ""
				},{
					"name": "buffDel",
					"method": "POST",
					"data": {"Id": 0}
				},{
					"name": "buffMod",
					"method": "POST",
					"data": ""
				},{
					"name": "buffMoveToFront",
					"method": "POST",
					"data": {"Id": 0}
				},{
					"name": "list",
					"method": "POST",
					"data": {"StartId": 0, "Limit": 20}
				},{
					"name": "mod",
					"method": "POST",
					"data": ""
				},{
					"name": "getPublish",
					"method": "POST",
					"data": ""
				},{
					"name": "setPublish",
					"method": "POST",
					"data": [{"PublishTime":[5, 0], "BeginTime":[5, 0], "EndTime":[13, 0], "EventNum":1}]
				}
			]
		},{
			"tab":"challenge",
			"path":"challenge",
			"apis":[
				{
					"name": "count",
					"method": "POST",
					"data": ""
				},{
					"name": "list",
					"method": "POST",
					"data": {"Offset": 0, "Limit": 20}
				},{
					"name": "mod",
					"method": "POST",
					"data": ""
				},{
					"name": "getPlay",
					"method": "POST",
					"data": {"ChallengeId": 0}
				},{
					"name": "submitScore",
					"method": "POST",
					"data": {"ChallengeId": 0, "Score": 100, "Checksum":"cks"}
				}
			]
		},{
			"tab":"admin",
			"path":"admin",
			"apis":[
				{
					"name": "getUserInfo",
					"method": "POST",
					"data": {"UserId": 0, "Email":""}
				},{
					"name": "addGoldCoin",
					"method": "POST",
					"data": {"UserId":1030, "AddGoldCoin": 100}
				},{
					"name": "addPrize",
					"method": "POST",
					"data": {"UserId": 0, "UserName": "aa", "prize": 100}
				},{
					"name": "setAdsConf",
					"method": "POST",
					"data": {"ShowPercent": 0.5, "DelayPercent": 0.5, "DelaySec": 1.0}
				},{
					"name": "clearPlayer",
					"method": "POST",
					"data": {"UserId": 0}
				},{
					"name": "delMatch",
					"method": "POST",
					"data": {"MatchId": 0}
				}
			]
		},{
			"tab":"store",
			"path":"store",
			"apis":[
				{
					"name": "listGameCoinPack",
					"method": "POST",
					"data": ""
				},{
					"name": "listIapProductId",
					"method": "POST",
					"data": ""
				},{
					"name": "getIapSecret",
					"method": "POST",
					"data": ""
				},{
					"name": "buyIap",
					"method": "POST",
					"data": {"ProductId":"", "Checksum":""}
				},{
					"name": "addEcardType",
					"method": "POST",
					"data": {"Name":"亚马逊电子礼品卡10元", "Provider":"amazon", "RmbPrice":10, "NeedPrize":12345, "Thumb":"lw/amazon.jpg"}
				},{
					"name": "editEcardType",
					"method": "POST",
					"data": {"Key":"", "Name":"", "Thumb":""}
				},{
					"name": "delEcardType",
					"method": "POST",
					"data": {"Key":""}
				},{
					"name": "listEcardType",
					"method": "POST",
					"data": ""
				},{
					"name": "addEcard",
					"method": "POST",
					"data": {"Provider":"amazon", "RmbPrice":0, "Code":"", "ExpireDate":""}
				},{
					"name": "addEcards",
					"method": "POST",
					"data": ""
				},{
					"name": "buyEcard",
					"method": "POST",
					"data": {"TypeKey":""}
				},{
					"name": "setEcardStoreClose",
					"method": "POST",
					"data": {"Close":"true or false"}
				}

			]
		},{
			"tab":"etc",
			"path":"etc",
			"apis":[
				{
					"name": "betHelp",
					"method": "POST",
					"data": ""
				},{
					"name": "addAdvice",
					"method": "POST",
					"data": {"Text":"this is advice"}
				},{
					"name": "listAdvice",
					"method": "POST",
					"data": {"StartId": 0, "Limit":20}
				}
			]
		},{
			"tab":"userPack",
			"path":"userPack",
			"apis":[
				{
					"name": "new",
					"method": "POST",
					"data": ""
				},{
					"name": "del",
					"method": "POST",
					"data": {"UserPackId": 0}
				},{
					"name": "listMine",
					"method": "POST",
					"data": {"UserId": 0, "StartId": 0, "Limit":12}
				},{
					"name": "listLatest",
					"method": "POST",
					"data": {"UserId": 0, "StartId": 0, "Limit":12}
				},{
					"name": "get",
					"method": "POST",
					"data": {"UserPackId": 0}
				}
			]
		},{
			"tab":"social",
			"path":"social",
			"apis":[
				{
					"name": "newPack",
					"method": "POST",
					"data": {"PackId": 0, "SliderNum":5, "Msec":50000}
				},{
					"name": "getPack",
					"method": "POST",
					"data":{"Key": "aaabbb"}
				},{
					"name": "play",
					"method": "POST",
					"data":{"Key": "aaabbb", "CheckSum": "xxxx", "UserName":"liwei", "Msec":50000}
				}
			]
		},{
			"tab":"match",
			"path":"match",
			"apis":[
				{
					"name": "new",
					"method": "POST",
					"data": ""
				},{
					"name": "del",
					"method": "POST",
					"data": {"MatchId": 0}
				},{
					"name": "list",
					"method": "POST",
					"data": {"StartId": 0, "BeginTime": 0, "Limit":12}
				},{
					"name": "get",
					"method": "POST",
					"data": {"MatchId": 0}
				},{
					"name": "listMine",
					"method": "POST",
					"data": {"StartId": 0, "BeginTime": 0, "Limit":12}
				},{
					"name": "listMyPlayed",
					"method": "POST",
					"data": {"StartId": 0, "PlayedTime": 0, "Limit":12}
				},{
					"name": "listHot",
					"method": "POST",
					"data": {"StartId": 0, "PrizeSum": 0, "Limit":12}
				},{
					"name": "getDynamicData",
					"method": "POST",
					"data": {"MatchId": 0}
				}

			]
		},{
			"tab":"ecoMonitor",
			"path":"ecoMonitor",
			"apis":[
				{
					"name": "ecoPlayerList",
					"method": "POST",
					"data": {"UserId": 0, "StartId":0, "Limit":20, "Time":""}
				},{
					"name": "ecoDailyCount",
					"method": "POST",
					"data":{"Date": "2006-01-02"}
				}
			]
		},{
			"tab":"tumblr",
			"path":"tumblr",
			"apis":[
				{
					"name": "listBlog",
					"method": "POST",
					"data": {"LastKey":"", "LastScore":0, "Limit":20}
				},{
					"name": "listImage",
					"method": "POST",
					"data": {"BlogName":"", "LastKey":"", "LastScore":0, "Limit":20}
				}
			]
		},{
			"tab":"channel",
			"path":"channel",
			"apis":[
				{
					"name": "set",
					"method": "POST",
					"data": {"Channels":[]}
				},{
					"name": "list",
					"method": "POST",
					"data": ""
				},{
					"name": "addUser",
					"method": "POST",
					"data": {"ChannelName":"", "UserId":-2222}
				},{
					"name": "delUser",
					"method": "POST",
					"data": {"ChannelName":"", "UserId":-2222}
				},{
					"name": "listUser",
					"method": "POST",
					"data": {"ChannelName":""}
				},{
					"name": "listDetail",
					"method": "POST",
					"data": ""
				}
			]
		}
	]

	//
	$('#email').val(localStorage.email)

	//
	var sendCodeMirror = CodeMirror.fromTextArea(sendTextArea, 
		{
			theme: "elegant",
		}
	);
	var recvCodeMirror = CodeMirror.fromTextArea(recvTextArea, 
		{
			theme: "elegant",
		}
	);

	sendCodeMirror.setSize("100%", 500)
	recvCodeMirror.setSize("100%", 500)
	sendCodeMirror.addKeyMap({
		"Ctrl-,": function(cm) {
			var hisList = inputHistory[$scope.currUrl]
			if (isdef(hisList)) {
				var idx = Math.max(0, Math.min(hisList.length-1, inputHisIdx-1))
				if (inputHisIdx != idx) {
					inputHisIdx = idx
					sendCodeMirror.doc.setValue(hisList[inputHisIdx][0])
					recvCodeMirror.doc.setValue(hisList[inputHisIdx][1])
				}
			}
		},
		"Ctrl-.": function(cm) {
			var hisList = inputHistory[$scope.currUrl]
			if (isdef(hisList)) {
				var idx = Math.max(0, Math.min(hisList.length-1, inputHisIdx+1))
				if (inputHisIdx != idx) {
					inputHisIdx = idx
					sendCodeMirror.doc.setValue(hisList[inputHisIdx][0])
					recvCodeMirror.doc.setValue(hisList[inputHisIdx][1])
				}
			}
		},
		"Esc":function(cm) {
			var api = $scope.currApi
			if (api && api.data) {
				sendCodeMirror.doc.setValue(JSON.stringify(api.data, null, '\t'))
			} else {
				sendCodeMirror.doc.setValue("")
			}
			recvCodeMirror.doc.setValue("")
		}
	}) 

	CodeMirror.signal(sendCodeMirror, "keydown", 2)

	var inputHistory = {}
	var sendInput = ""
	var sendUrl=""
	var inputHisIdx = 0

	$scope.currApi = null

	$scope.onApiClick = function(api, path) {
		if ($scope.currApi != api) {
			$("#btn-send").removeAttr("disabled")
			$scope.currApi = api
			$scope.currUrl = path+"/"+$scope.currApi.name
			inputHisIdx = 0
			var hisList = inputHistory[$scope.currUrl]
			if (isdef(hisList)) {
				inputHisIdx = hisList.length-1
				sendCodeMirror.doc.setValue(hisList[inputHisIdx][0])
				recvCodeMirror.doc.setValue(hisList[inputHisIdx][1])
			} else {
				if (api.data) {
					sendCodeMirror.doc.setValue(JSON.stringify(api.data, null, '\t'))
				}else{
					sendCodeMirror.doc.setValue("")
				}
				recvCodeMirror.doc.setValue("")
			}
		}
		sendCodeMirror.focus()
	}

	$scope.login = function() {
		var url = "/auth/login"
		var email = $('#email').val()
		var password = $('#password').val()
		var body = {
			"Username": email,
			"Password": password
		}
		var bodyStr = JSON.stringify(body, null)
		$.post(url, bodyStr, function(json){
			alert("login ok")
			localStorage.email = email
		}, "json")
		.fail(function(){alert("login failed")})
	}

	$scope.queryTick = 0
	var lastHisText = ""
	$scope.send = function() {
		var input = sendCodeMirror.doc.getValue()
		var inputText = input
		if (input) {
			try {
				input = JSON.parse(input)
			} catch(err) {
				alert("parse json error")
				return
			}	
		}

		var onReceive = function(json) {
			printQueryTick()
			var replyText = JSON.stringify(json, null, '\t')
			recvCodeMirror.doc.setValue(replyText)

			//history
			if (isdef(inputHistory[sendUrl])) {
				var inHisList = inputHistory[sendUrl]
				if (inHisList[inHisList.length-1] != sendInput) {
					inputHistory[sendUrl].push([sendInput, replyText])
				}
			} else {
				inputHistory[sendUrl] = [[sendInput, replyText]]
			}
			inputHisIdx = inputHistory[sendUrl].length-1
	
			sendCodeMirror.focus()
		}

		var onFail = function(obj) {
			printQueryTick()
			var t = JSON.stringify(obj.responseJSON, null, '\t')
			if (isdef(t))
				t = t.replace(/\\n/g, "\n")
			if (isdef(t))
				t = t.replace(/\\t/g, "  ")
			var text = obj.status + ":" + obj.statusText + "\n\n" + t
			recvCodeMirror.doc.setValue(text)
		}

		function printQueryTick() {
			$scope.$apply(function(){
				$scope.queryTick = Math.round(window.performance.now() - t)
			});
		}

		sendInput = inputText
		sendUrl = $scope.currUrl
		var url = "/"+sendUrl
		var t = window.performance.now()
		if ($scope.currApi.method == "GET") {
			$.getJSON(url, input, onReceive)
			.fail(onFail)
		}else if ($scope.currApi.method == "POST") {
			$.post(url, sendCodeMirror.doc.getValue(), onReceive, "json")
			.fail(onFail)
		}
	}
}



