(function(){

    var oauth = OAuth({
        consumer: {
            public: 'xvz1evFS4wEEPTGEFPHBog',
            secret: 'kAcSOqF21Fu85e7zjz7ZN2U4ZRhfV3WpwPAoE3Z7kBw'
        },
        signature_method: 'HMAC-SHA1'
    });

    var request_data = {
        url: 'https://api.twitter.com/1/statuses/update.json?include_entities=true',
        method: 'POST',
        data: {
            status: 'Hello Ladies + Gentlemen, a signed OAuth request!'
        }
    };

    var token = {
        public: '370773112-GmHxMAgYyLbNEtIKZeRNFsMKPR9EyMZeS9weJAEb',
        secret: 'LswwdoUaIvS8ltyTt5jkRh4J50vUPVVHtR2YPi5kE'
    };

    oauth.getTimeStamp = function() {
        return 1318622958;
    };

    //overide for testing only !!!
    oauth.getNonce = function(length) {
        return 'kYjzVBB8Y0ZFabxSWbWovY3uYSQ2pTgmZeNu2VS4cg';
    };

    var oauth_data = {
        oauth_consumer_key: oauth.consumer.public,
        oauth_nonce: oauth.getNonce(),
        oauth_signature_method: oauth.signature_method,
        oauth_timestamp: oauth.getTimeStamp(),
        oauth_version: '1.0',
        oauth_token: token.public
    };

    // alert(oauth.getParameterString(request_data, oauth_data))

    $.ajax({
        url: request_data.url+"&callback=?",
        type: request_data.method,
        data: oauth.authorize(request_data, token)
    }).done(function(data) {
        console.log(data)
    });
    
})();