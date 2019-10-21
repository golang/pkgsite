/*
  Copyright 2019 The Go Authors. All rights reserved.
  Use of this source code is governed by a BSD-style
  license that can be found in the LICENSE file.
*/

// ARIA-compliant event handlers for buttons.
// See https://www.w3.org/TR/wai-aria-practices/examples/button/js/button.js.

// addButtonHandlers takes an element acting as a button and a function of no
// arguments to run when the button is activated. It installs ARIA-compliant
// click and keypress handlers for the element.
function addButtonHandlers(el, activateFunc) {
  el.addEventListener('click', function (event) {
    event.preventDefault();
    activateFunc();
  })

  // Activate on enter-down.
  el.addEventListener('keydown', function (event) {
    // The action button is activated by space on the keyup event, but the
    // default action for space is already triggered on keydown. It needs to be
    // prevented to stop scrolling the page before activating the button.
    if (event.keyCode === 32) {
      event.preventDefault();
    }
    // If enter is pressed, activate the button.
    else if (event.keyCode === 13) {
      event.preventDefault();
      activateFunc();
    }
  });

  // Activate on space-up.
  el.addEventListener('keyup', function (event) {
    if (event.keyCode === 32) {
      event.preventDefault();
      activateActionButton();
    }
  });
}
