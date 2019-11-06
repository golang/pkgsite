// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/**
 * A bit of navigation related code for handling dismissible elements.
 */
(function() {
  'use strict';

  function registerHeaderListeners() {
    const header = document.querySelector('.js-header');
    const menuButtons = document.querySelectorAll('.js-headerMenuButton');
    menuButtons.forEach(button => {
      button.addEventListener('click', e => {
        e.preventDefault();
        header.classList.toggle('is-active');
        button.setAttribute(
          'aria-expanded',
          header.classList.contains('is-active')
        );
      });
    });

    const scrim = document.querySelector('.js-scrim');
    if (scrim && scrim.hasOwnProperty('addEventListener')) {
      scrim.addEventListener('click', e => {
        e.preventDefault();
        header.classList.remove('is-active');
        menuButtons.forEach(button => {
          button.setAttribute(
            'aria-expanded',
            header.classList.contains('is-active')
          );
        });
      });
    }
  }

  registerHeaderListeners();
})();
