/*
  Copyright 2019 The Go Authors. All rights reserved.
  Use of this source code is governed by a BSD-style
  license that can be found in the LICENSE file.
*/

window.addEventListener("load", function() {
  var elements = document.getElementsByClassName('js-feedbackButton');
  for (var i = 0; i < elements.length; i++) {
    elements[i].addEventListener('click', sendFeedback)
  };
});

// Launches the feedback interface.
function sendFeedback() {
  var configuration = { productId: '5131929', bucket: 'Default' };
  userfeedback.api.startFeedback(configuration);
}
