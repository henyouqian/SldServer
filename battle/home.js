$(function() {
    $("#pair").click(function() {
        var token = $('#token').val()
        var msg = '{"Type":"authPair", "Token":"'+token+'", "RoomName":"free"}'
        conn.send(msg)

        localStorage.token = token
    })
    $('#token').val(localStorage.token)

    $("#emoji").click(function() {
        var msg = '{"Type":"talk", "Text":"😝"}'
        conn.send(msg)
    })
    $("#ready").click(function() {
        var msg = '{"Type":"ready"}'
        conn.send(msg)
    })
    $("#finish").click(function() {
        var msg = '{"Type":"finish", "Msec":1000}'
        conn.send(msg)
    })
})