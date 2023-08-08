var s=document.querySelector(".js-fetchButton");s&&s.addEventListener("click",e=>{e.preventDefault(),i()});async function i(){var a;let e=document.querySelector(".js-fetchMessage"),t=document.querySelector(".js-fetchMessageSecondary"),o=document.querySelector(".js-fetchButton"),n=document.querySelector(".js-fetchLoading");if(!(e&&t&&o&&n))return;e.textContent=`Fetching ${e.dataset.path}`,t.textContent="Feel free to navigate away and check back later, we\u2019ll keep working on it!",o.style.display="none",n.style.display="block";let c=await fetch(`/fetch${window.location.pathname}`,{method:"POST"});if(c.ok){window.location.reload();return}let r=await c.text();n.style.display="none",t.textContent="";let l=new DOMParser().parseFromString(r,"text/html");e.innerText=(a=l.documentElement.textContent)!=null?a:""}
/*!
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=fetch.js.map
