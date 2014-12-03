$(function() {
    $("#pair").click(function() {
        var room = $('#room').val()
        var token = $('#token').val()
        var msg = '{"Type":"authPair", "Token":"'+token+'", "RoomName":"'+room+'"}'
        conn.send(msg)
        localStorage.token = token
        localStorage.room = room
    })
    $('#token').val(localStorage.token)
    $('#room').val(localStorage.room)

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