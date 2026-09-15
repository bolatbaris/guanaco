import type {App} from 'vue'
import type {Router} from 'vue-router'
import type {Event as SentryEvent} from '@sentry/vue'
import {shouldDropEvent} from './helpers/sentryFilters'
import {VERSION} from './version.json'

declare global {
	interface Window {
		SENTRY_DSN?: string;
		SENTRY_ENVIRONMENT?: string;
		SENTRY_FRONTEND_TRACES_SAMPLE_RATE?: number | string;
		SENTRY_FRONTEND_REPLAY_SESSION_SAMPLE_RATE?: number | string;
		SENTRY_FRONTEND_REPLAY_ON_ERROR_SAMPLE_RATE?: number | string;
	}
}

const SENSITIVE_QUERY_PARAMETER = /^(?:access[_-]?token|api[_-]?key|authorization|code|email|password|refresh[_-]?token|secret|session|sig(?:nature)?|token|key)$/i
const EMAIL_PATTERN = /\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/gi
const QUERY_PARAMETER_PATTERN = /([?&](?:access[_-]?token|api[_-]?key|authorization|code|email|password|refresh[_-]?token|secret|session|sig(?:nature)?|token|key)=)[^&#\s]*/gi

function sampleRate(value: number | string | undefined, fallback: number): number {
	const parsed = typeof value === 'number' ? value : Number.parseFloat(value ?? '')
	return Number.isFinite(parsed) ? Math.min(1, Math.max(0, parsed)) : fallback
}

function runtimeDsn(): string {
	if (typeof window.SENTRY_DSN === 'string') {
		return window.SENTRY_DSN.trim()
	}

	return import.meta.env.VITE_SENTRY_DSN?.trim() ?? ''
}

function runtimeEnvironment(): string | undefined {
	return window.SENTRY_ENVIRONMENT ?? import.meta.env.VITE_SENTRY_ENVIRONMENT
}

function runtimeTraceSampleRate(): number | string | undefined {
	return window.SENTRY_FRONTEND_TRACES_SAMPLE_RATE ?? import.meta.env.VITE_SENTRY_TRACES_SAMPLE_RATE
}

function scrubText(value: string): string {
	return value.replace(EMAIL_PATTERN, '[email redacted]').replace(QUERY_PARAMETER_PATTERN, '$1[redacted]')
}

function scrubUrl(value: string): string {
	try {
		const url = new URL(value, window.location.origin)
		url.username = ''
		url.password = ''
		for (const key of [...url.searchParams.keys()]) {
			if (SENSITIVE_QUERY_PARAMETER.test(key)) {
				url.searchParams.set(key, '[redacted]')
			}
		}
		url.hash = ''
		return url.toString()
	} catch {
		return scrubText(value)
	}
}

function sanitizeEvent(event: SentryEvent): void {
	if (event.message) {
		event.message = scrubText(event.message)
	}

	if (event.user) {
		delete event.user.email
		delete event.user.username
		delete event.user.ip_address
	}

	if (event.request) {
		if (event.request.url) {
			event.request.url = scrubUrl(event.request.url)
		}
		delete event.request.query_string
		delete event.request.headers
		delete event.request.cookies
		delete event.request.data
	}

	for (const value of event.exception?.values ?? []) {
		if (value.value) {
			value.value = scrubText(value.value)
		}
	}

	for (const breadcrumb of event.breadcrumbs ?? []) {
		if (breadcrumb.message) {
			breadcrumb.message = scrubText(breadcrumb.message)
		}
		const url = breadcrumb.data?.url
		if (typeof url === 'string' && breadcrumb.data) {
			breadcrumb.data.url = scrubUrl(url)
		}
		if (breadcrumb.data) {
			delete breadcrumb.data.body
			delete breadcrumb.data.data
			delete breadcrumb.data.headers
		}
	}
}

export default async function setupSentry(app: App, router: Router) {
	const dsn = runtimeDsn()
	if (!dsn) {
		return
	}

	try {
		const Sentry = await import('@sentry/vue')
		const replaysSessionSampleRate = sampleRate(window.SENTRY_FRONTEND_REPLAY_SESSION_SAMPLE_RATE, 0)
		const replaysOnErrorSampleRate = sampleRate(window.SENTRY_FRONTEND_REPLAY_ON_ERROR_SAMPLE_RATE, 0)
		// @sentry/vue 10 does not expose autoSessionTracking; remove its default
		// BrowserSession integration until release-health tracking is explicitly opted in.
		const defaultIntegrations = Sentry
			.getDefaultIntegrations({})
			.filter(integration => integration.name !== 'BrowserSession')
		const integrations = [
			Sentry.browserTracingIntegration({router}),
			...(replaysSessionSampleRate > 0 || replaysOnErrorSampleRate > 0
				? [Sentry.replayIntegration({slowClickTimeout: 0})]
				: []),
		]

		Sentry.init({
			app,
			dsn,
			environment: runtimeEnvironment(),
			release: `vikunja-frontend@${VERSION}`,

			// Keep delivery non-blocking when the browser is temporarily offline.
			transport: Sentry.makeBrowserOfflineTransport(Sentry.makeFetchTransport),
			defaultIntegrations: [...defaultIntegrations, Sentry.vueIntegration()],
			integrations,
			tracesSampleRate: sampleRate(runtimeTraceSampleRate(), 0.05),
			tracePropagationTargets: ['localhost', /^\//],
			replaysSessionSampleRate,
			replaysOnErrorSampleRate,

			denyUrls: [
				/^chrome-extension:\/\//i,
				/^moz-extension:\/\//i,
				/^safari-web-extension:\/\//i,
				/^safari-extension:\/\//i,
				/^ms-browser-extension:\/\//i,
			],

			beforeSend(event, hint) {
				sanitizeEvent(event)
				if (shouldDropEvent(hint.originalException, event)) {
					return null
				}

				return event
			},
		})

		// Resource failures are useful for detecting broken deployments, but their
		// URLs can contain user-controlled query strings.
		document.body.addEventListener(
			'error',
			(event) => {
				const target = event.target

				if (target instanceof HTMLImageElement) {
					const src = target.getAttribute('src')
					if (!src || src === '#') return

					Sentry.captureMessage(`Failed to load image: ${scrubUrl(target.src)}`, 'warning')
				} else if (target instanceof HTMLLinkElement) {
					Sentry.captureMessage(`Failed to load css: ${scrubUrl(target.href)}`, 'warning')
				}
			},
			true,
		)
	} catch (error) {
		// Observability must never prevent the application from starting.
		console.error('Could not initialize Sentry', error)
	}
}
