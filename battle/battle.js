function Controller($scope, $http) {
    //cookie
    function getCookie(c_name) {
        if (document.cookie.length>0) {
            c_start=document.cookie.indexOf(c_name + "=");
            if (c_start!=-1) {
                c_start=c_start + c_name.length+1;
                c_end=document.cookie.indexOf(";",c_start);
                if (c_end==-1) c_end=document.cookie.length;
                return unescape(document.cookie.substring(c_start,c_end))
            }
        }
        return "";
    }

    // var HOST = "http://sld.pintugame.com/"
    var HOST = "http://localhost:9998/"
    var RES_HOST = "http://dn-pintuuserupload.qbox.me/"
    var HTML5_HOST = "http://pintuhtml5.qiniudn.com/"

    var conn
    var procMap = {}
    var token

    if (window["WebSocket"]) {
        conn = new WebSocket("ws://127.0.0.1:8080/ws");
        conn.onclose = function(evt) {
            console.log("Connection closed")
        }
        conn.onmessage = function(evt) {
            msg = JSON.parse(evt.data)
            if (msg.Type in procMap) {
                (procMap[msg.Type])(msg)
            } else {
                console.log("Msg unproc:", evt.data)
            }
        }
    } else {
        console.log("Your browser does not support WebSockets")
    }

    $('#email').val(localStorage.email)

    $scope.login = function() {
        var url = HOST+"auth/login"
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
            token = json.Token
        }, "json")
        .fail(function(){alert("login failed")})
    }

    $scope.pair = function() {
        msg = {Type:"pair", Token:token}
        conn.send(JSON.stringify(msg, null))
    }

    procMap.paired = function(msg) {
        console.log("onPaired:", msg)
    }
}