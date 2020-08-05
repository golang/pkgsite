/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

function submitForm(formName, reload) {
  let form = document[formName];
  form.result.value = 'request pending...';
  let xhr = new XMLHttpRequest();
  xhr.onreadystatechange = function () {
    if (this.readyState == 4) {
      if (this.status >= 200 && this.status < 300) {
        if (reload) {
          location.reload();
        } else {
          form.result.value = 'Success.';
        }
      } else {
        form.result.value = 'ERROR: ' + this.responseText;
      }
    }
  };
  xhr.open(form.method, form.action);
  xhr.send(new FormData(form));
}
