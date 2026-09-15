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
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/version"

	"github.com/getsentry/sentry-go"
)

const flushTimeout = 5 * time.Second

var (
	emailPattern   = regexp.MustCompile(`(?i)\b[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+\b`)
	urlPattern     = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>]+`)
	pathURLPattern = regexp.MustCompile(`(?i)(^|[\s"'(])(/[^\s"'<>?#]+)[?#][^\s"'<>]*`)
	secretPattern  = regexp.MustCompile(`(?i)(\b(?:password|passwd|secret|token|access_token|refresh_token|id_token|client_secret|api[_-]?key|apikey|authorization|set-cookie|cookie|session[_-]?id|oauth[_-]?(?:code|state)|authorization[_-]?code|code_verifier|nonce|jwt|signature|sig|code|state)\b)(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|(?:Bearer|Basic)\s+[^\s,;}\]]+|[^\s,;}\]]+)`)

	sensitiveContextKeyPattern = regexp.MustCompile(`(?i)(?:pass(?:word)?|secret|token|authorization|set[-_]?cookie|cookie|session|api[-_]?key|client[-_]?secret|credential|body|payload|request|header|query|user|email|oauth|code|state|nonce)`)
)

var (
	stateMu     sync.RWMutex
	initialized bool
	enabled     bool
	noopHub     = sentry.NewHub(nil, sentry.NewScope())
)

// Init configures the process-wide Sentry client. It is safe to call more than
// once; the first successful call owns the SDK for the lifetime of the process.
func Init() error {
	stateMu.Lock()
	defer stateMu.Unlock()

	if initialized {
		return nil
	}

	dsn := strings.TrimSpace(config.SentryDsn.GetString())
	if !config.SentryEnabled.GetBool() || dsn == "" {
		initialized = true
		return nil
	}

	options := clientOptions()
	options.Dsn = dsn
	if err := sentry.Init(options); err != nil {
		return fmt.Errorf("initialize sentry: %w", err)
	}

	initialized = true
	enabled = true
	return nil
}

// Close flushes pending events for at most a bounded duration and then closes
// the transport. It is intended for process shutdown.
func Close() bool {
	stateMu.Lock()
	if !initialized || !enabled {
		stateMu.Unlock()
		return true
	}
	enabled = false
	hub := sentry.CurrentHub()
	client := hub.Client()
	stateMu.Unlock()

	if client == nil {
		return true
	}

	flushed := hub.Flush(flushTimeout)
	client.Close()
	return flushed
}

// Enabled reports whether the configured Sentry client is active.
func Enabled() bool {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return enabled
}

// HubFromContext returns the request-scoped hub when one is present. It falls
// back to a no-op hub while Sentry is disabled.
func HubFromContext(ctx context.Context) *sentry.Hub {
	if !Enabled() {
		return noopHub
	}
	if ctx != nil {
		if hub := sentry.GetHubFromContext(ctx); hub != nil {
			return hub
		}
	}
	return sentry.CurrentHub()
}

// CaptureException reports an error through the configured process hub.
func CaptureException(err error) *sentry.EventID {
	if !Enabled() || err == nil {
		return nil
	}
	return sentry.CurrentHub().CaptureException(err)
}

// CaptureExceptionContext reports an error using the hub attached to ctx.
func CaptureExceptionContext(ctx context.Context, err error) *sentry.EventID {
	if !Enabled() || err == nil {
		return nil
	}
	return HubFromContext(ctx).CaptureException(err)
}

// CaptureMessage reports a diagnostic message through the configured process
// hub. Messages are subject to the same event sanitization as exceptions.
func CaptureMessage(message string) *sentry.EventID {
	if !Enabled() || message == "" {
		return nil
	}
	return sentry.CurrentHub().CaptureMessage(message)
}

func clientOptions() sentry.ClientOptions {
	return sentry.ClientOptions{
		AttachStacktrace: true,
		BeforeSend:       sanitizeEvent,
		BeforeSendLog:    sanitizeLog,
		EnableTracing:    traceSampleRate() > 0,
		Environment:      strings.TrimSpace(config.SentryEnvironment.GetString()),
		Release:          version.Version,
		SampleRate:       1,
		DataCollection: &sentry.DataCollection{
			UserInfo: sentry.Set(false),
			Cookies:  &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			HTTPHeaders: &sentry.HeaderCollectionConfig{
				Request:  &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
				Response: &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff},
			},
			HTTPBodies: []sentry.BodyType{},
			QueryParams: &sentry.KeyValueCollectionBehavior{
				Mode: sentry.CollectionOff,
			},
		},
		TracesSampleRate: traceSampleRate(),
	}
}

func traceSampleRate() float64 {
	rate := config.SentryTracesSampleRate.Get()
	var value float64
	switch rate := rate.(type) {
	case float64:
		value = rate
	case float32:
		value = float64(rate)
	case int:
		value = float64(rate)
	case int64:
		value = float64(rate)
	case string:
		value, _ = strconv.ParseFloat(strings.TrimSpace(rate), 64)
	default:
		value = 0
	}

	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func sanitizeEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}

	// Request data can contain credentials, tokens, query strings, headers, or
	// bodies. Do not rely on SDK defaults because callers may add request data
	// to a scope themselves.
	event.Request = nil
	event.User = sentry.User{}
	event.Attachments = nil
	clearEventExtra(event)
	event.Message = redactText(event.Message)
	event.Transaction = redactText(event.Transaction)
	event.Logger = redactText(event.Logger)

	for index := range event.Exception {
		event.Exception[index].Value = redactText(event.Exception[index].Value)
		if mechanism := event.Exception[index].Mechanism; mechanism != nil {
			mechanism.Description = redactText(mechanism.Description)
			mechanism.HelpLink = redactText(mechanism.HelpLink)
			mechanism.Source = redactText(mechanism.Source)
			mechanism.Data = sanitizeContextMap(mechanism.Data)
		}
	}

	for key, value := range event.Tags {
		event.Tags[key] = redactText(value)
	}

	for _, breadcrumb := range event.Breadcrumbs {
		if breadcrumb == nil {
			continue
		}
		breadcrumb.Category = redactText(breadcrumb.Category)
		breadcrumb.Message = redactText(breadcrumb.Message)
		breadcrumb.Data = sanitizeContextMap(breadcrumb.Data)
	}

	if event.Contexts != nil {
		delete(event.Contexts, "request")
		delete(event.Contexts, "http")
		delete(event.Contexts, "http.request")
		for key, value := range event.Contexts {
			if isSensitiveContextKey(key) {
				delete(event.Contexts, key)
				continue
			}
			event.Contexts[key] = sanitizeContextMap(value)
		}
	}

	return event
}

func clearEventExtra(event *sentry.Event) {
	field := reflect.ValueOf(event).Elem().FieldByName("Extra")
	if field.IsValid() && field.CanSet() {
		field.Set(reflect.Zero(field.Type()))
	}
}

func sanitizeLog(log *sentry.Log) *sentry.Log {
	if log == nil {
		return nil
	}
	log.Body = redactText(log.Body)
	// Log attributes are the log equivalent of arbitrary extra data. Dropping
	// them keeps BeforeSendLog safe even when a caller adds a new attribute.
	log.Attributes = nil
	return log
}

func sanitizeContextMap(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		if isSensitiveContextKey(key) {
			continue
		}
		if sanitized, ok := sanitizeContextValue(value, 0); ok {
			result[key] = sanitized
		}
	}
	return result
}

func sanitizeContextValue(value interface{}, depth int) (interface{}, bool) {
	if value == nil || depth > 8 {
		return nil, false
	}

	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return nil, false
	}

	switch reflected.Kind() {
	case reflect.Interface, reflect.Pointer:
		if reflected.IsNil() {
			return nil, false
		}
		return sanitizeContextValue(reflected.Elem().Interface(), depth+1)
	case reflect.String:
		return redactText(reflected.String()), true
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return value, true
	case reflect.Map:
		if reflected.Type().Key().Kind() != reflect.String {
			return nil, false
		}
		result := make(map[string]interface{}, reflected.Len())
		for _, key := range reflected.MapKeys() {
			keyString := key.String()
			if isSensitiveContextKey(keyString) {
				continue
			}
			entry := reflected.MapIndex(key)
			if !entry.IsValid() {
				continue
			}
			if sanitized, ok := sanitizeContextValue(entry.Interface(), depth+1); ok {
				result[keyString] = sanitized
			}
		}
		return result, true
	case reflect.Slice, reflect.Array:
		if reflected.Type().Elem().Kind() == reflect.Uint8 {
			return "[REDACTED]", true
		}
		result := make([]interface{}, 0, reflected.Len())
		for index := 0; index < reflected.Len(); index++ {
			if sanitized, ok := sanitizeContextValue(reflected.Index(index).Interface(), depth+1); ok {
				result = append(result, sanitized)
			}
		}
		return result, true
	case reflect.Invalid, reflect.Chan, reflect.Complex64, reflect.Complex128,
		reflect.Func, reflect.Struct, reflect.UnsafePointer:
		return nil, false
	default:
		return nil, false
	}
}

func isSensitiveContextKey(key string) bool {
	return sensitiveContextKeyPattern.MatchString(key)
}

func redactText(value string) string {
	if value == "" {
		return value
	}

	value = secretPattern.ReplaceAllString(value, "$1$2[REDACTED]")
	value = urlPattern.ReplaceAllStringFunc(value, redactURL)
	value = pathURLPattern.ReplaceAllString(value, "$1$2")
	return emailPattern.ReplaceAllString(value, "[REDACTED_EMAIL]")
}

func redactURL(value string) string {
	trailing := ""
	for len(value) > 0 && strings.ContainsRune(".,;:!?)]}", rune(value[len(value)-1])) {
		trailing = string(value[len(value)-1]) + trailing
		value = value[:len(value)-1]
	}

	parsed, err := url.Parse(value)
	if err != nil {
		if index := strings.IndexAny(value, "?#"); index >= 0 {
			value = value[:index]
		}
		return value + trailing
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String() + trailing
}
