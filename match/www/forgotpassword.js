(function() {
    $("#form").submit(function(event) {
    	event.preventDefault();
    	$("#btn").prop('disabled', true);

    	var onReceive = function(json) {
			alert("重设密码邮件发送成功，请根据邮件内容提示重设密码。如找不到邮件，可能在垃圾箱里。")
		}

		var onFail = function(obj) {
			json = obj.responseJSON
			var t = JSON.stringify(json, null, '\t')
			if (json.Error && json.Error == "err_not_exist") {
				alert("账号不存在")
			} else {
				alert(t)
			}
			
    		$("#btn").prop('disabled', false);
		}

		var body = {"Email":$('#email').val()}

    	$.post("../auth/forgotPassword",JSON.stringify(body, null, '\t'), onReceive, "json")
			.fail(onFail)
	});
}());











