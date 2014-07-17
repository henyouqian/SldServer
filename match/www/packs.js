function Controller($scope, $http) {
	$scope.packs = [
		{"id":1, "icon":"img/icon.jpg"},
		{"id":2, "icon":"img/icon.jpg"},
		{"id":3, "icon":"img/icon.jpg"},
		{"id":4, "icon":"img/icon.jpg"},
		{"id":5, "icon":"img/icon.jpg"}
	]
	var addPackTemplate = {
		"icon":"",
		"cover":"",
		"images":[
			{
				"url":""
			}
		]
	}

	addPackCodeMirror = CodeMirror.fromTextArea(
		addPackTextArea, 
		{
			theme: "elegant",
		}
	)

	editPackCodeMirror = CodeMirror.fromTextArea(
		editPackTextArea, 
		{
			theme: "elegant",
		}
	)

	$scope.addPackDlg = function() {
		$('#addModal').modal({})
		console.log(addPackCodeMirror)
		addPackCodeMirror.doc.setValue(JSON.stringify(addPackTemplate, null, '\t'))
		setTimeout("addPackCodeMirror.refresh()",500);
		
	}

	$scope.addPack = function() {
		var input = addPackCodeMirror.doc.getValue()
		var inputText = input
		if (input) {
			try {
				input = JSON.parse(input)
			} catch(err) {
				alert("parse json error")
				return
			}
		}
		$('#addModal').modal('hide')
	}

	$scope.editPackDlg = function(pack) {
		$('#editModal').modal({})
	}
}



