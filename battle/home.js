$(function() {
    $("#pair").click(function() {
        var msg = '{"Type":"authPair", "Token":"l3fJlzstRDZikuEJQopGJA=="}'
        conn.send(msg)
    })
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