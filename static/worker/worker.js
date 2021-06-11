/*!
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
function submitForm(formName, reload) {
  const form = document.querySelector(`form[name="${formName}" ]`);
  if (!form) {
    throw Error(`Form "${formName}" not found.`);
  }
  form.result.value = "request pending...";
  const xhr = new XMLHttpRequest();
  xhr.onreadystatechange = function() {
    if (this.readyState == 4) {
      if (this.status >= 200 && this.status < 300) {
        if (reload) {
          location.reload();
        } else {
          form.result.value = "Success.";
        }
      } else {
        form.result.value = "ERROR: " + this.responseText;
      }
    }
  };
  xhr.open(form.method, form.action);
  xhr.send(new FormData(form));
}
window.submitForm = submitForm;
//# sourceMappingURL=data:application/json;base64,ewogICJ2ZXJzaW9uIjogMywKICAic291cmNlcyI6IFsid29ya2VyLnRzIl0sCiAgInNvdXJjZXNDb250ZW50IjogWyIvKiFcbiAqIEBsaWNlbnNlXG4gKiBDb3B5cmlnaHQgMjAyMSBUaGUgR28gQXV0aG9ycy4gQWxsIHJpZ2h0cyByZXNlcnZlZC5cbiAqIFVzZSBvZiB0aGlzIHNvdXJjZSBjb2RlIGlzIGdvdmVybmVkIGJ5IGEgQlNELXN0eWxlXG4gKiBsaWNlbnNlIHRoYXQgY2FuIGJlIGZvdW5kIGluIHRoZSBMSUNFTlNFIGZpbGUuXG4gKi9cblxuZnVuY3Rpb24gc3VibWl0Rm9ybShmb3JtTmFtZTogc3RyaW5nLCByZWxvYWQ6IGJvb2xlYW4pIHtcbiAgY29uc3QgZm9ybSA9IGRvY3VtZW50LnF1ZXJ5U2VsZWN0b3I8SFRNTEZvcm1FbGVtZW50PihgZm9ybVtuYW1lPVwiJHtmb3JtTmFtZX1cIiBdYCk7XG4gIGlmICghZm9ybSkge1xuICAgIHRocm93IEVycm9yKGBGb3JtIFwiJHtmb3JtTmFtZX1cIiBub3QgZm91bmQuYCk7XG4gIH1cbiAgZm9ybS5yZXN1bHQudmFsdWUgPSAncmVxdWVzdCBwZW5kaW5nLi4uJztcbiAgY29uc3QgeGhyID0gbmV3IFhNTEh0dHBSZXF1ZXN0KCk7XG4gIHhoci5vbnJlYWR5c3RhdGVjaGFuZ2UgPSBmdW5jdGlvbiAoKSB7XG4gICAgaWYgKHRoaXMucmVhZHlTdGF0ZSA9PSA0KSB7XG4gICAgICBpZiAodGhpcy5zdGF0dXMgPj0gMjAwICYmIHRoaXMuc3RhdHVzIDwgMzAwKSB7XG4gICAgICAgIGlmIChyZWxvYWQpIHtcbiAgICAgICAgICBsb2NhdGlvbi5yZWxvYWQoKTtcbiAgICAgICAgfSBlbHNlIHtcbiAgICAgICAgICBmb3JtLnJlc3VsdC52YWx1ZSA9ICdTdWNjZXNzLic7XG4gICAgICAgIH1cbiAgICAgIH0gZWxzZSB7XG4gICAgICAgIGZvcm0ucmVzdWx0LnZhbHVlID0gJ0VSUk9SOiAnICsgdGhpcy5yZXNwb25zZVRleHQ7XG4gICAgICB9XG4gICAgfVxuICB9O1xuICB4aHIub3Blbihmb3JtLm1ldGhvZCwgZm9ybS5hY3Rpb24pO1xuICB4aHIuc2VuZChuZXcgRm9ybURhdGEoZm9ybSkpO1xufVxuXG53aW5kb3cuc3VibWl0Rm9ybSA9IHN1Ym1pdEZvcm07XG4iXSwKICAibWFwcGluZ3MiOiAiQUFBQTtBQUFBO0FBQUE7QUFBQTtBQUFBO0FBQUE7QUFPQSxvQkFBb0IsVUFBa0IsUUFBaUI7QUFDckQsUUFBTSxPQUFPLFNBQVMsY0FBK0IsY0FBYztBQUNuRSxNQUFJLENBQUMsTUFBTTtBQUNULFVBQU0sTUFBTSxTQUFTO0FBQUE7QUFFdkIsT0FBSyxPQUFPLFFBQVE7QUFDcEIsUUFBTSxNQUFNLElBQUk7QUFDaEIsTUFBSSxxQkFBcUIsV0FBWTtBQUNuQyxRQUFJLEtBQUssY0FBYyxHQUFHO0FBQ3hCLFVBQUksS0FBSyxVQUFVLE9BQU8sS0FBSyxTQUFTLEtBQUs7QUFDM0MsWUFBSSxRQUFRO0FBQ1YsbUJBQVM7QUFBQSxlQUNKO0FBQ0wsZUFBSyxPQUFPLFFBQVE7QUFBQTtBQUFBLGFBRWpCO0FBQ0wsYUFBSyxPQUFPLFFBQVEsWUFBWSxLQUFLO0FBQUE7QUFBQTtBQUFBO0FBSTNDLE1BQUksS0FBSyxLQUFLLFFBQVEsS0FBSztBQUMzQixNQUFJLEtBQUssSUFBSSxTQUFTO0FBQUE7QUFHeEIsT0FBTyxhQUFhOyIsCiAgIm5hbWVzIjogW10KfQo=
