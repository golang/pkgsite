/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
declare global {
  interface Window {
    submitForm: typeof submitForm;
  }
}

function submitForm(formName: string, reload: boolean) {
  const form = document.querySelector<HTMLFormElement>(`form[name="${formName}" ]`);
  if (!form) {
    throw Error(`Form "${formName}" not found.`);
  }
  form.result.value = 'request pending...';
  const xhr = new XMLHttpRequest();
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

window.submitForm = submitForm;

export {};
