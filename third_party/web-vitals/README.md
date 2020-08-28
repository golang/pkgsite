# web-vitals (v0.2.4)

The `web-vitals` is a library for measuring all the [Web Vitals](https://web.dev/vitals/) metrics on real users, in a way that accurately matches how they're measured by Chrome and reported to other Google tools (e.g. [Chrome User Experience Report](https://developers.google.com/web/tools/chrome-user-experience-report), [Page Speed Insights](https://developers.google.com/speed/pagespeed/insights/), [Search Console's Speed Report](https://webmasters.googleblog.com/2019/11/search-console-speed-report.html)).

Built from [source](https://github.com/GoogleChrome/web-vitals) and copied to third party scripts.

To reinstall or update

```
git clone https://github.com/GoogleChrome/web-vitals
cd web-vitals
git checkout tags/<version tag>
npm install
npm build
cd dist
cp web-vitals.es5.min <path to pkgsite>/third_party
```
