(function() {
    var key = getUrlParam("key")

    $("#form").submit(function(event) {
    	event.preventDefault();
    	if ($('#pwd').val() != $('#pwd2').val()) {
    		alert("两次密码输入不一致，请重新输入")
    		$('#pwd').val("")
    		$('#pwd2').val("")
    		return;
    	}
    	$("#btn").prop('disabled', true);

    	var onReceive = function(json) {
			alert("重设密码成功，请使用新密码登录游戏")
		}

		var onFail = function(obj) {
			json = obj.responseJSON
			var t = JSON.stringify(json, null, '\t')
			console.log(json)
			if (json.Error && json.Error == "err_key") {
				alert("此页面已使用或已过期，请重新找回密码")
				window.location.href='forgotpassword.html'
			} else {
				alert(t)
			}
			$('#pwd').val("")
    		$('#pwd2').val("")
    		$("#btn").prop('disabled', false);
		}

		var body = {"ResetKey":key, "Password":$('#pwd').val()}

    	$.post("../auth/resetPassword",JSON.stringify(body, null, '\t'), onReceive, "json")
			.fail(onFail)
	});
}());




