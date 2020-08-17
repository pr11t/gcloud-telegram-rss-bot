package rssbot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

var feedXML = `
<?xml version="1.0" encoding="utf-8"?><?xml-stylesheet type="text/xsl" href="https://www.example.com/xsl"?>
<rss xmlns:media="http://search.yahoo.com/mrss/" version="2.0">
    <channel>
        <title>news | TEST</title>
        <description>newsdescription</description>
        <link>http://www.example.com</link>
        <language>en</language>
        <copyright>null</copyright>
        <webMaster>web@example.com</webMaster>
        <pubDate>Sun, 16 Aug 2020 13:43:44 +0300</pubDate>
        <lastBuildDate>Sun, 16 Aug 2020 13:30:00 +0300</lastBuildDate>
        <generator>Test generator</generator>
        <item>
            <title><![CDATA[Example news title1]]></title>
            <link>https://example.com/news/item1</link>
            <description><![CDATA[Example news description1 ]]></description>
            <media:thumbnail url='https://example.com/news/item1/picture1.jpg' height='75' width='75' />
            <guid isPermaLink="true">https://example.com/11111111</guid>
            <pubDate>Sun, 16 Aug 2020 13:30:00 +0300</pubDate>
            <category><![CDATA[Category1]]></category>
        </item>
        <item>
            <title><![CDATA[Example news title2]]></title>
            <link>https://example.com/news/item2</link>
            <description><![CDATA[Example news description2 ]]></description>
            <media:thumbnail url='https://example.com/news/item2/picture2.jpg' height='75' width='75' />
            <guid isPermaLink="true">https://example.com/222222222</guid>
            <pubDate>Sun, 16 Aug 2020 13:30:00 +0300</pubDate>
            <category><![CDATA[Category2]]></category>
		</item>
		<item>
			<title><![CDATA[Example news title3]]></title>
			<link>https://example.com/news/item3</link>
			<description><![CDATA[Example news description3 ]]></description>
			<media:thumbnail url='https://example.com/news/item3/picture3.jpg' height='75' width='75' />
			<guid isPermaLink="true">https://example.com/333333333</guid>
			<pubDate>Sun, 16 Aug 2020 13:30:00 +0300</pubDate>
			<category><![CDATA[Category3]]></category>
		</item>
		</channel>
		</rss>`

func TestRSSFeedFetchLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write([]byte(feedXML))
	}))
	defer server.Close()
	r := RSSFeed{URL: server.URL}
	r.Fetch()
	if len(r.RSS.Channel.Items) != 3 {
		t.Errorf("Wrong number of links extracted")
	}
	if r.RSS.Channel.Items[1].Link != "https://example.com/news/item2" {
		t.Errorf("Unexpected link in position 2: %v", r.RSS.Channel.Items[1].Link)
	}
}

func TestRSSFeedReverse(t *testing.T) {
	orig := RSS{Channel: Channel{Items: []Item{{Link: "example.com/1"}, {Link: "example.com/2"}, {Link: "example.com/3"}}}}
	want := RSS{Channel: Channel{Items: []Item{{Link: "example.com/3"}, {Link: "example.com/2"}, {Link: "example.com/1"}}}}
	r := RSSFeed{RSS: orig}
	r.reverse()
	if !reflect.DeepEqual(r.RSS, want) {
		t.Errorf("Reversing the slice produced unexpected results, want:%v got:%v", want, r.RSS)
	}
}

func TestRSSFeedRemoveOlderThan(t *testing.T) {
	orig := RSS{Channel: Channel{Items: []Item{{Link: "example.com/1"}, {Link: "example.com/2"}, {Link: "example.com/3"}}}}
	want := RSS{Channel: Channel{Items: []Item{{Link: "example.com/1"}, {Link: "example.com/2"}}}}
	olderThan := "example.com/3"
	r := RSSFeed{RSS: orig}
	r.removeOlderThan(olderThan)
	if !reflect.DeepEqual(r.RSS, want) {
		t.Errorf("Removing older than %v failed, expected: %v, got: %v", olderThan, want, r.RSS)
	}
}

func TestTelegramSuccessfulAPICall(t *testing.T) {
	wantResponse := struct {
		OK     bool `json:"ok"`
		Result bool `json:"result"`
	}{
		OK:     true,
		Result: true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		msg, err := json.Marshal(wantResponse)
		if err != nil {
			t.Error(err)
		}
		rw.Write(msg)
	}))
	defer server.Close()
	chatID := "@test_chat"
	apiToken := "123456-aaaaaaa"
	params := struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: chatID,
		Text:   "test",
	}

	api := TelegramAPI{APIToken: apiToken, APIURL: server.URL, ChatID: chatID}
	gotResponse, err := api.call("testCommand", params)
	if err != nil {
		t.Error(err)
	}
	if reflect.DeepEqual(wantResponse, gotResponse) {
		t.Errorf("Received invalid response want:%v got:%v", wantResponse, gotResponse)
	}

}

func TestFailingAPICall(t *testing.T) {
	wantResponse := struct {
		OK     bool `json:"ok"`
		Result bool `json:"result"`
	}{
		OK:     false,
		Result: true,
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		msg, err := json.Marshal(wantResponse)
		if err != nil {
			t.Error(err)
		}
		rw.Write(msg)
	}))
	defer server.Close()
	chatID := "@test_chat"
	apiToken := "123456-aaaaaaa"
	params := struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: chatID,
		Text:   "test",
	}

	api := TelegramAPI{APIToken: apiToken, APIURL: server.URL, ChatID: chatID}
	_, err := api.call("testCommand", params)
	if err == nil {
		t.Errorf("Expected an error, but didn't get one.")
	}
	if err.Error() != "API did not return OK" {
		t.Errorf("Expected different error, but got %v", err.Error())
	}
}
