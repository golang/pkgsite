/*
  Copyright 2019 The Go Authors. All rights reserved.
  Use of this source code is governed by a BSD-style
  license that can be found in the LICENSE file.
*/

window.onload = function() {
  var el = document.getElementById('Feedback-button');
  if (el) {
    el.addEventListener('click', sendFeedback);
  } else {
    console.log('No Feedback-button');
  }
};

// Launches the feedback interface.
function sendFeedback() {
  var configuration = { productId: '5131929', bucket: 'Default' };
  userfeedback.api.startFeedback(configuration);
}
