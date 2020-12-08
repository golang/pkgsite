// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/logging"
	"github.com/google/go-cmp/cmp"
)

func TestRequestLog(t *testing.T) {
	tests := []struct {
		label   string
		handler http.HandlerFunc
		want    fakeLog
	}{
		{
			label: "writes status",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(400)
			},
			want: fakeLog{Status: 400},
		},
		{
			label:   "translates 200s",
			handler: func(w http.ResponseWriter, r *http.Request) {},
			want:    fakeLog{Status: 200},
		},
	}

	for _, test := range tests {
		t.Run(test.label, func(t *testing.T) {
			lg := fakeLog{}
			mw := RequestLog(&lg)
			ts := httptest.NewServer(mw(test.handler))
			defer ts.Close()
			resp, err := ts.Client().Get(ts.URL)
			if err != nil {
				t.Fatalf("GET returned error %v", err)
			}
			resp.Body.Close()
			if diff := cmp.Diff(test.want, lg); diff != "" {
				t.Errorf("mismatching log state (-want +got):\n%s", diff)
			}
		})
	}
}

type fakeLog struct {
	Status int
}

func (l *fakeLog) Log(entry logging.Entry) {
	if entry.HTTPRequest != nil {
		l.Status = entry.HTTPRequest.Status
	}
}

func TestIsRobot(t *testing.T) {
	for _, test := range []string{
		"AHC/2.1",
		"Algolia DocSearch Crawler",
		"Apache-HttpClient/4.5.12 (Java/1.8.0_201)",
		"AppEngine-Google; (+http://code.google.com/appengine)",
		"ArchiveTeam ArchiveBot/20200413.2e71c9a (wpull 2.0.3) and not Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.90 Safari/537.36",
		"AwarioSmartBot/1.0 (+https://awario.com/bots.html; bots@awario.com)",
		"BananaBot/0.6.1",
		"Blackboard Safeassign",
		"BorneoBot/0.7.1 (crawlcheck123@gmail.com)",
		"CCBot/2.0 (https://commoncrawl.org/faq/)",
		"Camo Asset Proxy 2.3.0",
		"Faraday v0.15.4",
		"Faraday v1.0.1",
		"Go 1.1 package http",
		"Go-http-client/2.0",
		"GoogleStackdriverMonitoring-UptimeChecks(https://cloud.google.com/monitoring)",
		"Googlebot-Video/1.0",
		"Java/1.8.0_222",
		"KamelioCrawler/0.0.0",
		"Lawinsiderbot/1.0 (+http://www.lawinsider.com/about)",
		"Linguee Bot (http://www.linguee.com/bot; bot@linguee.com)",
		"Mozilla/5.0 (compatible; AhrefsBot/6.1; +http://ahrefs.com/robot/)",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Node-RED www-request",
		"Node.js/12.16.3 (macOS Mojave; x64)",
		"NovichenkoBot",
		"OpenBSD ftp",
		"Python/3.9 aiohttp/3.6.2",
		"Qwantify/1.0",
		"Ruby",
		"Scoop.it",
		"Scrapy/1.8.0 (+https://scrapy.org)",
		"SerendeputyBot/0.8.6 (http://serendeputy.com/about/serendeputy-bot)",
		"SiteSucker for macOS/3.2.2",
		"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)",
		"Sphinx/3.0.4 requests/2.23.0 python/3.7.3",
		"TelegramBot (like TwitterBot)",
		"Twitterbot/1.0",
		"Typhoeus - https://github.com/typhoeus/typhoeus",
		"WebexTeams",
		"Wget/1.20.3 (linux-gnu)",
		"WordPress/5.5.1; https://its-more.jp/ja_jp",
		"chimebot",
		"cis455crawler",
		"colly - https://github.com/gocolly/colly",
		"colly - https://github.com/gocolly/colly/v2",
		"curl/7.69.1",
		"datagnionbot (+http://www.datagnion.com/bot.html)",
		"erbbot",
		"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)",
		"fasthttp",
		"git/2.22.3",
		"git/2.22.4",
		"github-camo (da61ea2e)",
		"got (https://github.com/sindresorhus/got)",
		"ltx71 - (http://ltx71.com/)",
		"mdbook-linkcheck-0.7.0",
		"mercurial/proto-1.0 (Mercurial 4.9.1)",
		"meterian.cli-v1.0",
		"node-fetch/1.0 (+https://github.com/bitinn/node-fetch)",
		"okhttp/3.14.0",
		"pimeyes.com crawler",
		"python-requests/2.10.0",
		"rest-client/2.0.2 (linux-gnu x86_64) ruby/2.6.5p114",
	} {
		if !isRobot(test) {
			t.Errorf("isRobot(%q) = false; want true", test)
		}
	}
	for _, test := range []string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.80 Safari/537.36",
		"Safari/15609.1.20.111.8 CFNetwork/1125.2 Darwin/19.4.0 (x86_64)",
		"MobileSafari/604.1 CFNetwork/1126 Darwin/19.5.0",
	} {
		if isRobot(test) {
			t.Errorf("isRobot(%q) = true; want = false", test)
		}
	}
}
