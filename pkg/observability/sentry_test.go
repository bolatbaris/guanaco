// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package observability

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/config"

	"github.com/getsentry/sentry-go"
	"github.com/getsentry/sentry-go/attribute"
	"github.com/spf13/viper"
)

func TestInitIsNoopWhenDisabledOrDSNIsEmpty(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
		dsn     string
	}{
		{name: "disabled", enabled: false, dsn: "https://public@example.invalid/1"},
		{name: "empty dsn", enabled: true, dsn: ""},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			resetStateForTest()
			setSentryConfigForTest(t, tt.enabled, tt.dsn, "beta", 0.25)

			if err := Init(); err != nil {
				t.Fatalf("Init() error = %v", err)
			}
			if Enabled() {
				t.Fatal("Enabled() = true, want false")
			}
			if got := CaptureException(errors.New("must not be captured")); got != nil {
				t.Fatal("CaptureException() returned an event ID for a no-op client")
			}
			if got := CaptureExceptionContext(context.Background(), errors.New("must not be captured")); got != nil {
				t.Fatal("CaptureExceptionContext() returned an event ID for a no-op client")
			}
			if got := CaptureMessage("must not be captured"); got != nil {
				t.Fatal("CaptureMessage() returned an event ID for a no-op client")
			}
			if HubFromContext(context.Background()).Client() != nil {
				t.Fatal("HubFromContext() returned a configured client for a no-op client")
			}
			if !Close() {
				t.Fatal("Close() = false for a no-op client")
			}
		})
	}
}

func TestClientOptionsMapEnvironmentAndTraceSampleRate(t *testing.T) {
	resetStateForTest()
	setSentryConfigForTest(t, true, "https://public@example.invalid/1", "beta", 0.25)

	options := clientOptions()
	if options.Environment != "beta" {
		t.Errorf("Environment = %q, want %q", options.Environment, "beta")
	}
	if options.TracesSampleRate != 0.25 {
		t.Errorf("TracesSampleRate = %v, want %v", options.TracesSampleRate, 0.25)
	}
	if !options.EnableTracing {
		t.Fatal("EnableTracing = false, want true for a positive sample rate")
	}
	if options.SampleRate != 1 {
		t.Errorf("SampleRate = %v, want 1", options.SampleRate)
	}
	if options.DataCollection == nil {
		t.Fatal("DataCollection = nil, want explicit privacy settings")
	}
	dataCollection := options.DataCollection
	if !dataCollection.UserInfo.IsSet || dataCollection.UserInfo.Value {
		t.Fatalf("DataCollection.UserInfo = %+v, want explicitly disabled", dataCollection.UserInfo)
	}
	if dataCollection.Cookies == nil || dataCollection.Cookies.Mode != sentry.CollectionOff {
		t.Fatalf("DataCollection.Cookies = %+v, want CollectionOff", dataCollection.Cookies)
	}
	if dataCollection.HTTPHeaders == nil ||
		dataCollection.HTTPHeaders.Request == nil ||
		dataCollection.HTTPHeaders.Request.Mode != sentry.CollectionOff ||
		dataCollection.HTTPHeaders.Response == nil ||
		dataCollection.HTTPHeaders.Response.Mode != sentry.CollectionOff {
		t.Fatalf("DataCollection.HTTPHeaders = %+v, want request and response CollectionOff", dataCollection.HTTPHeaders)
	}
	if dataCollection.HTTPBodies == nil || len(dataCollection.HTTPBodies) != 0 {
		t.Fatalf("DataCollection.HTTPBodies = %v, want empty", dataCollection.HTTPBodies)
	}
	if dataCollection.QueryParams == nil || dataCollection.QueryParams.Mode != sentry.CollectionOff {
		t.Fatalf("DataCollection.QueryParams = %+v, want CollectionOff", dataCollection.QueryParams)
	}
}

func TestSanitizeEventRemovesSensitiveRequestData(t *testing.T) {
	event := &sentry.Event{
		Request: &sentry.Request{
			URL:         "https://example.invalid/api?token=secret",
			QueryString: "token=secret",
			Data:        `{"password":"secret"}`,
			Headers:     map[string]string{"Authorization": "Bearer secret"},
		},
		User:        sentry.User{Email: "person@example.invalid"},
		Attachments: []*sentry.Attachment{{Filename: "payload.txt"}},
		Contexts: map[string]sentry.Context{
			"request":      {"url": "https://example.invalid/?token=secret"},
			"http":         {"body": "secret"},
			"http.request": {"query": "secret"},
			"runtime":      {"name": "go"},
		},
	}

	if got := sanitizeEvent(event, nil); got != event {
		t.Fatal("sanitizeEvent() returned a different event")
	}
	if event.Request != nil || !reflect.DeepEqual(event.User, sentry.User{}) || event.Attachments != nil {
		t.Fatal("sanitizeEvent() did not remove sensitive event fields")
	}
	if _, ok := event.Contexts["request"]; ok {
		t.Fatal("request context was not removed")
	}
	if _, ok := event.Contexts["http"]; ok {
		t.Fatal("http context was not removed")
	}
	if _, ok := event.Contexts["http.request"]; ok {
		t.Fatal("http.request context was not removed")
	}
	if _, ok := event.Contexts["runtime"]; !ok {
		t.Fatal("safe runtime context was removed")
	}
}

func TestSanitizeEventHandlesNilFields(t *testing.T) {
	if sanitizeEvent(nil, nil) != nil {
		t.Fatal("sanitizeEvent(nil) should return nil")
	}

	event := &sentry.Event{
		Breadcrumbs: []*sentry.Breadcrumb{nil},
		Exception:   []sentry.Exception{{Mechanism: nil}},
	}
	if sanitizeEvent(event, nil) != event {
		t.Fatal("sanitizeEvent() returned a different event")
	}
}

func TestSanitizeEventClearsSDKExtraWhenPresentAndPreservesTagsAndFingerprint(t *testing.T) {
	event := &sentry.Event{
		Tags:        map[string]string{"kind": "worker"},
		Fingerprint: []string{"worker-failure"},
	}
	extra := reflect.ValueOf(event).Elem().FieldByName("Extra")
	if extra.IsValid() && extra.CanSet() {
		extra.Set(reflect.MakeMap(extra.Type()))
		extra.SetMapIndex(reflect.ValueOf("payload"), reflect.ValueOf("private"))
	}

	sanitizeEvent(event, nil)

	if extra.IsValid() && !extra.IsNil() {
		t.Fatal("sanitizeEvent() did not remove arbitrary extra data")
	}
	if event.Tags["kind"] != "worker" {
		t.Fatal("sanitizeEvent() removed a useful tag")
	}
	if !reflect.DeepEqual(event.Fingerprint, []string{"worker-failure"}) {
		t.Fatalf("sanitizeEvent() changed fingerprint: %v", event.Fingerprint)
	}
}

func TestRedactURLRemovesUserinfoQueryAndFragment(t *testing.T) {
	got := redactURL("https://alice:secret@example.invalid/path?token=private#fragment")
	want := "https://example.invalid/path"
	if got != want {
		t.Fatalf("redactURL() = %q, want %q", got, want)
	}
}

func TestSanitizeEventRedactsTextBreadcrumbsAndContexts(t *testing.T) {
	event := &sentry.Event{
		Message:     `panic: password="hunter2" email=person@example.invalid https://example.invalid/callback?code=oauth-secret#fragment`,
		Transaction: "GET /api/v2/tasks?filter=private#details",
		Tags:        map[string]string{"request_id": "safe-request-id", "email": "person@example.invalid"},
		Exception: []sentry.Exception{{
			Type:  "panic",
			Value: "oauth code=abc123 authorization=Bearer top-secret",
			Mechanism: &sentry.Mechanism{
				Type:        "panic",
				Description: "failed for user@example.invalid",
				Data:        map[string]interface{}{"safe": "value", "password": "secret"},
			},
		}},
		Breadcrumbs: []*sentry.Breadcrumb{
			nil,
			{Message: "request https://example.invalid/?access_token=secret#oauth", Data: map[string]interface{}{
				"safe":   "hello@example.invalid",
				"nested": map[string]interface{}{"cookie": "secret", "count": 2},
			}},
		},
		Contexts: map[string]sentry.Context{
			"runtime": {"name": "go", "version": "1.23"},
			"request": {"url": "https://example.invalid/?secret=value"},
			"custom":  {"password": "secret", "message": "email=person@example.invalid", "count": 3},
		},
	}

	if got := sanitizeEvent(event, nil); got != event {
		t.Fatal("sanitizeEvent() returned a different event")
	}

	for _, text := range []string{
		event.Message,
		event.Transaction,
		event.Exception[0].Value,
		event.Exception[0].Mechanism.Description,
		event.Breadcrumbs[1].Message,
		event.Tags["email"],
	} {
		if strings.Contains(text, "hunter2") || strings.Contains(text, "oauth-secret") || strings.Contains(text, "top-secret") || strings.Contains(text, "person@example.invalid") {
			t.Errorf("sanitized text still contains sensitive value: %q", text)
		}
	}
	if strings.Contains(event.Transaction, "?") || strings.Contains(event.Transaction, "#") {
		t.Errorf("relative URL query/hash was not removed: %q", event.Transaction)
	}
	if event.Exception[0].Type != "panic" || event.Tags["request_id"] != "safe-request-id" {
		t.Fatal("useful exception type or tag was not preserved")
	}
	if _, ok := event.Exception[0].Mechanism.Data["password"]; ok {
		t.Fatal("mechanism secret data was not removed")
	}
	if _, ok := event.Breadcrumbs[1].Data["nested"]; !ok {
		t.Fatal("safe nested breadcrumb data was removed")
	}
	if _, ok := event.Breadcrumbs[1].Data["safe"]; !ok {
		t.Fatal("safe breadcrumb data was removed")
	}
	if _, ok := event.Contexts["custom"]; !ok {
		t.Fatal("safe custom context was removed")
	}
	if _, ok := event.Contexts["request"]; ok {
		t.Fatal("request context was not removed")
	}
	if _, ok := event.Contexts["custom"]["password"]; ok {
		t.Fatal("context secret was not removed")
	}
}

func TestSanitizeLogRemovesAttributesAndRedactsBody(t *testing.T) {
	if sanitizeLog(nil) != nil {
		t.Fatal("sanitizeLog(nil) should return nil")
	}

	log := &sentry.Log{
		Body:       "request failed email=person@example.invalid token=secret",
		Attributes: map[string]attribute.Value{"password": attribute.StringValue("secret")},
	}
	if got := sanitizeLog(log); got != log {
		t.Fatal("sanitizeLog() returned a different log")
	}
	if log.Attributes != nil {
		t.Fatal("log attributes were not removed")
	}
	if strings.Contains(log.Body, "person@example.invalid") || strings.Contains(log.Body, "secret") {
		t.Fatal("log body was not redacted")
	}
}

func setSentryConfigForTest(t *testing.T, enabled bool, dsn, environment string, sampleRate float64) {
	t.Helper()
	setConfigValue(t, config.SentryEnabled, enabled)
	setConfigValue(t, config.SentryDsn, dsn)
	setConfigValue(t, config.SentryEnvironment, environment)
	setConfigValue(t, config.SentryTracesSampleRate, sampleRate)
}

func setConfigValue(t *testing.T, key config.Key, value interface{}) {
	t.Helper()
	previous := configValue(key)
	viper.Set(string(key), value)
	t.Cleanup(func() {
		if previous == nil {
			viper.Set(string(key), nil)
			return
		}
		viper.Set(string(key), previous)
	})
}

func configValue(key config.Key) interface{} {
	return viper.Get(string(key))
}

func resetStateForTest() {
	stateMu.Lock()
	initialized = false
	enabled = false
	stateMu.Unlock()
}
